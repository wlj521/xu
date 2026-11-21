package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *App) syncInputCount() int {
	count := 0
	for _, on := range a.syncInputIDs {
		if on {
			count++
		}
	}
	return count
}

func (a *App) syncInputSelected(id int64) bool {
	return a.syncInputIDs[id]
}

func (a *App) setSyncInputEnabled(on bool) {
	if !on {
		if a.syncInputEnabled {
			a.status = "同步输入已关闭"
		}
		a.syncInputEnabled = false
		return
	}
	ts := a.activeTerminal()
	if ts == nil || ts.local || ts.exited {
		a.status = "请先打开一个已连接的 SSH 标签"
		return
	}
	a.syncInputIDs[ts.ID] = true
	if a.syncInputCount() < 2 {
		a.status = "同步输入至少需要选择两个 SSH 标签"
		return
	}
	a.syncInputEnabled = true
	a.status = fmt.Sprintf("同步输入已启用 · %d 个 SSH 标签", a.syncInputCount())
}

func (a *App) toggleSyncInputSession(id int64) {
	if id == 0 {
		return
	}
	if a.syncInputIDs[id] {
		if a.syncInputEnabled {
			if active := a.activeTerminal(); active != nil && active.ID == id {
				a.status = "当前标签是同步输入来源，不能移出同步组"
				return
			}
		}
		delete(a.syncInputIDs, id)
	} else {
		ts := a.termSessionByID(id)
		if ts == nil || ts.local || ts.exited {
			return
		}
		a.syncInputIDs[id] = true
	}
	a.normalizeSyncInput()
}

func (a *App) removeSyncInputSession(id int64) {
	if !a.syncInputIDs[id] {
		return
	}
	delete(a.syncInputIDs, id)
	a.normalizeSyncInput()
}

func (a *App) pruneSyncInputSessions() {
	changed := false
	for id := range a.syncInputIDs {
		ts := a.termSessionByID(id)
		if ts == nil || ts.local || ts.exited {
			delete(a.syncInputIDs, id)
			changed = true
		}
	}
	if changed {
		a.normalizeSyncInput()
	}
}

func (a *App) normalizeSyncInput() {
	if !a.syncInputEnabled {
		return
	}
	if a.syncInputCount() < 2 {
		a.syncInputEnabled = false
		a.status = "同步输入已自动关闭：可用 SSH 标签不足两个"
	}
}

func (a *App) syncInputActiveChanged() {
	if !a.syncInputEnabled {
		return
	}
	ts := a.activeTerminal()
	if ts == nil || ts.local || ts.exited {
		a.normalizeSyncInput()
		return
	}
	a.syncInputIDs[ts.ID] = true
}

func (a *App) syncInputMenu(m *ui.Menu) {
	m.MaxWidth(390)
	m.Item("高风险：相同的键盘输入会发送到多个服务器").Disabled(true)
	enable := m.Item(fmt.Sprintf("启用同步输入（已选 %d 个）", a.syncInputCount())).
		Checked(a.syncInputEnabled)
	if a.syncInputCount() < 2 {
		enable.Disabled(true)
	}
	if enable.Chosen() {
		a.setSyncInputEnabled(!a.syncInputEnabled)
	}
	m.Separator()

	candidates := 0
	for i, tab := range a.tabs {
		if tab.kind != tabTerminal || tab.term == nil || tab.term.local {
			continue
		}
		candidates++
		ts := tab.term
		label := fmt.Sprintf("%d  %s", i+1, a.tabDisplayTitle(tab))
		item := m.Item(label).Checked(a.syncInputSelected(ts.ID))
		if ts.exited {
			item.Disabled(true)
		}
		if item.Chosen() {
			a.toggleSyncInputSession(ts.ID)
		}
	}
	if candidates == 0 {
		m.Item("没有可选的 SSH 标签").Disabled(true)
	}

	m.Separator()
	if m.Item("全选 SSH 标签").Chosen() {
		for _, tab := range a.tabs {
			if tab.kind == tabTerminal && tab.term != nil && !tab.term.local && !tab.term.exited {
				a.syncInputIDs[tab.term.ID] = true
			}
		}
		a.normalizeSyncInput()
	}
	if m.Item("清空选择").Chosen() {
		a.syncInputEnabled = false
		a.syncInputIDs = make(map[int64]bool)
		a.status = "同步输入选择已清空"
	}
}

func (a *App) onTerminalInput(source *termSession, ev ui.InputEvent) {
	if !a.syncInputEnabled || source == nil || source.local || source.exited ||
		!a.syncInputSelected(source.ID) {
		return
	}
	active := a.activeTerminal()
	if active == nil || active.ID != source.ID {
		return
	}
	data := syncInputBytes(ev)
	if len(data) == 0 {
		return
	}
	for id, on := range a.syncInputIDs {
		if !on || id == source.ID {
			continue
		}
		target := a.termSessionByID(id)
		if target == nil || target.local || target.exited || target.writeInput == nil {
			continue
		}
		target.writeInput(data)
	}
}

func syncInputBytes(ev ui.InputEvent) []byte {
	switch ev.Kind {
	case ui.InputText:
		if ev.Text == "" {
			return nil
		}
		return []byte(ev.Text)
	case ui.InputCommand:
		if strings.EqualFold(ev.Text, "paste") {
			return syncInputPasteBytes()
		}
	case ui.InputKeyDown:
		return syncInputKeyBytes(ev.Mods, ev.Key)
	}
	return nil
}

func syncInputKeyBytes(mods ui.Modifiers, key ui.Key) []byte {
	if mods&ui.Super != 0 {
		return nil
	}
	if mods&ui.Alt == 0 && mods&ui.Ctrl != 0 && mods&ui.Shift != 0 {
		switch key {
		case ui.KeyC, ui.KeyA:
			return nil
		case ui.KeyV:
			return syncInputPasteBytes()
		}
	}
	if mods&ui.Alt == 0 && mods == ui.Ctrl && key == ui.KeyV {
		return syncInputPasteBytes()
	}
	if mods&ui.Alt == 0 && mods == ui.Ctrl && key == ui.KeyInsert {
		return nil
	}
	if mods == ui.Shift && key == ui.KeyInsert {
		return syncInputPasteBytes()
	}
	if mods&ui.Ctrl != 0 && mods&ui.Alt == 0 {
		if b, ok := ctrlKeyByte(key); ok {
			return []byte{b}
		}
	}

	seq := plainKeySequence(mods, key)
	if len(seq) == 0 {
		return nil
	}
	if mods&ui.Alt != 0 {
		return append([]byte{0x1b}, seq...)
	}
	return seq
}

func syncInputPasteBytes() []byte {
	text := mygo.Clipboard.ReadText()
	if text == "" {
		return nil
	}
	return []byte(text)
}

func ctrlKeyByte(key ui.Key) (byte, bool) {
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		return byte(key-ui.KeyA) + 1, true
	case key == ui.KeySpace:
		return 0, true
	case key == ui.KeyBracketLeft:
		return 0x1b, true
	case key == ui.KeyBackslash:
		return 0x1c, true
	case key == ui.KeyBracketRight:
		return 0x1d, true
	case key == ui.Key6:
		return 0x1e, true
	case key == ui.KeyMinus:
		return 0x1f, true
	}
	return 0, false
}

func plainKeySequence(mods ui.Modifiers, key ui.Key) []byte {
	switch key {
	case ui.KeyEnter:
		return []byte("\r")
	case ui.KeyBackspace:
		return []byte{0x7f}
	case ui.KeyEscape:
		return []byte{0x1b}
	case ui.KeyTab:
		if mods&ui.Shift != 0 {
			return []byte("\x1b[Z")
		}
		return []byte("\t")
	case ui.KeySpace:
		if mods&ui.Ctrl != 0 {
			return []byte{0}
		}
		return nil
	case ui.KeyDelete:
		return []byte("\x1b[3~")
	case ui.KeyInsert:
		return []byte("\x1b[2~")
	case ui.KeyHome:
		return []byte("\x1b[H")
	case ui.KeyEnd:
		return []byte("\x1b[F")
	case ui.KeyPageUp:
		return []byte("\x1b[5~")
	case ui.KeyPageDown:
		return []byte("\x1b[6~")
	case ui.KeyUp:
		return []byte("\x1b[A")
	case ui.KeyDown:
		return []byte("\x1b[B")
	case ui.KeyRight:
		return []byte("\x1b[C")
	case ui.KeyLeft:
		return []byte("\x1b[D")
	case ui.KeyF1:
		return []byte("\x1bOP")
	case ui.KeyF2:
		return []byte("\x1bOQ")
	case ui.KeyF3:
		return []byte("\x1bOR")
	case ui.KeyF4:
		return []byte("\x1bOS")
	case ui.KeyF5:
		return []byte("\x1b[15~")
	case ui.KeyF6:
		return []byte("\x1b[17~")
	case ui.KeyF7:
		return []byte("\x1b[18~")
	case ui.KeyF8:
		return []byte("\x1b[19~")
	case ui.KeyF9:
		return []byte("\x1b[20~")
	case ui.KeyF10:
		return []byte("\x1b[21~")
	case ui.KeyF11:
		return []byte("\x1b[23~")
	case ui.KeyF12:
		return []byte("\x1b[24~")
	}
	return nil
}

func (a *App) syncInputBanner(c *ui.Context, t *ui.Theme) {
	if !a.syncInputEnabled {
		return
	}
	count := a.syncInputCount()
	message := fmt.Sprintf("同步输入已启用：键盘、粘贴和特殊键会发送到选中的 %d 个 SSH 标签", count)
	if active := a.activeTerminal(); active == nil || active.local {
		message = fmt.Sprintf("同步输入已启用：切换到同步组内的 %d 个 SSH 标签后生效", count)
	}
	ui.Row(c).Height(30).Shrink(0).Padding(0, 12).Gap(7).AlignItems(ui.Center).
		Background(colAmber.Alpha(0.1)).BorderWidth(0, 0, 1, 0).
		BorderColor(colAmber.Alpha(0.24)).Children(func() {
		iconEl(c, iconSync, 12, colAmber)
		ui.Text(c, message).FontSize(9).TextColor(colAmber).NoWrap()
		ui.Text(c, "").Grow(1)
		ui.Text(c, "请确保所有标签处于相同交互状态").FontSize(8.5).TextColor(colMuted2).NoWrap()
		if ghostAction(c, t, "sync-input-disable", "关闭同步", nil).Clicked() {
			a.setSyncInputEnabled(false)
		}
	})
}

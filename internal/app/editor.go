package app

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type editorSession struct {
	id       int64
	termID   int64
	browser  *fileBrowser
	name     string
	remote   string
	local    string
	buffer   *editor.Buffer
	state    *editor.State
	baseSize int64
	baseMod  time.Time
	saving   bool
	err      string
}

const maxEditorSize = 5 << 20 // 5 MB

func (a *App) activeEditor() *editorSession {
	if a.editorIndex < 0 || a.editorIndex >= len(a.editorSessions) {
		return nil
	}
	return a.editorSessions[a.editorIndex]
}

func (a *App) findEditor(termID int64, remote string) int {
	for i, es := range a.editorSessions {
		if es.termID == termID && es.remote == remote {
			return i
		}
	}
	return -1
}

func (a *App) openRemoteEditor(b *fileBrowser, remote string) {
	if idx := a.findEditor(b.termID, remote); idx >= 0 {
		a.editorIndex = idx
		a.fileErr = ""
		return
	}

	st, err := b.client.Stat(remote)
	if err != nil {
		a.fileErr = "读取远端文件失败: " + err.Error()
		return
	}
	if st.Size() > maxEditorSize {
		a.fileErr = "文件超过 5 MB，暂不支持内置编辑"
		return
	}

	dataDir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		a.fileErr = "读取数据目录失败: " + err.Error()
		return
	}
	tmpDir := filepath.Join(dataDir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		a.fileErr = "创建临时目录失败: " + err.Error()
		return
	}

	name := path.Base(remote)
	local := filepath.Join(tmpDir, fmt.Sprintf("edit-%d-%s", time.Now().UnixNano(), safeLocalName(name)))
	if err := downloadFile(a.ctx, b.client, remote, local, nil); err != nil {
		a.fileErr = "下载文件失败: " + err.Error()
		return
	}

	data, err := os.ReadFile(local)
	if err != nil {
		os.Remove(local)
		a.fileErr = "读取临时文件失败: " + err.Error()
		return
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		os.Remove(local)
		a.fileErr = "这看起来是二进制文件，暂不支持内置编辑"
		return
	}

	buf := editor.NewBuffer(string(data))
	if lang := editor.LanguageFor(name); lang != nil {
		buf.SetLanguage(lang)
	}

	a.nextEditorID++
	a.editorSessions = append(a.editorSessions, &editorSession{
		id:       a.nextEditorID,
		termID:   b.termID,
		browser:  b,
		name:     name,
		remote:   remote,
		local:    local,
		buffer:   buf,
		state:    &editor.State{Buffer: buf},
		baseSize: st.Size(),
		baseMod:  st.ModTime(),
	})
	a.editorIndex = len(a.editorSessions) - 1
	a.fileErr = ""
	a.status = "已打开 " + name
}

// editorWindowRect keeps the floating editor over the terminal pane and
// leaves the file dock exposed, so another file can be opened without
// closing the editor.
func (a *App) editorWindowRect(c *ui.Context) (left, top, w, h float32) {
	cw, ch := c.Size()
	rail := float32(44)
	if !a.monitorCollapsed {
		rail = monitorWidth(c)
	}

	top = 86
	areaLeft := rail
	areaW := cw - rail
	areaH := ch - top - 12
	if areaW < 360 {
		areaW = 360
	}
	if areaH < 300 {
		areaH = 300
	}

	w = areaW - 40
	if w > 1000 {
		w = 1000
	}
	if w < 420 {
		w = areaW - 24
	}
	if w < 280 {
		w = 280
	}
	left = areaLeft + (areaW-w)/2

	h = areaH
	if h > 640 {
		h = 640
	}
	if a.bottomFiles {
		if ts := a.activeTerminal(); ts != nil && !ts.local {
			dockH := areaH * (1 - a.splitPercent/100)
			if candidate := areaH - dockH - 14; candidate >= 340 {
				h = candidate
			}
		}
	}
	if h > areaH {
		h = areaH
	}
	return left, top, w, h
}

func (a *App) editorWindow(c *ui.Context, t *ui.Theme) {
	es := a.activeEditor()
	if es == nil {
		return
	}
	left, top, w, h := a.editorWindowRect(c)

	ui.Overlay(c, func() {
		ui.Box(c).Absolute().Left(left).Top(top).Width(w).Height(h).
			Background(colPanel).Border(1, colLine2).Radius(12).Clip().
			Shadow(0, 24, 60, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Column(c).Fill().Children(func() {
				a.editorHeader(c, t, es)
				if len(a.editorSessions) > 1 {
					a.editorTabStrip(c, t)
				}
				es = a.activeEditor()
				if es == nil {
					return
				}
				if es.err != "" {
					ui.Box(c).FillWidth().Padding(8, 12).Margin(0, 14, 10, 14).Radius(7).
						Background(colRed.Alpha(0.07)).Border(1, colRed.Alpha(0.18)).Children(func() {
						ui.Text(c, es.err).FontSize(10).TextColor(colRed)
					})
				}

				if c.Shortcut(ui.Cmd, ui.KeyS) {
					a.saveEditor(es)
				}

				editor.View(c, es.state).Grow(1).FillWidth().AutoFocus()
			})
		})
	})
}

func (a *App) editorHeader(c *ui.Context, t *ui.Theme, es *editorSession) {
	ui.Row(c).Height(54).Shrink(0).Padding(0, 15).Gap(10).AlignItems(ui.Center).
		Background(colPanelTop).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Box(c).Size(32, 32).Radius(8).Shrink(0).
			Background(colCyan.Alpha(0.08)).Border(1, colCyan.Alpha(0.2)).Center().Children(func() {
			iconEl(c, iconFile, 15, colCyan)
		})

		title := es.name
		if es.buffer.Modified() {
			title += " *"
		}
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(fsTitle).FontWeight(650).NoWrap()
				if es.saving {
					ui.Text(c, "保存中…").FontSize(8.8).TextColor(colAmber).NoWrap()
				}
			})
			ui.Text(c, es.remote).FontSize(9.2).Font("Cascadia Code").
				TextColor(colMuted).NoWrap()
		})

		if len(a.editorSessions) > 1 {
			ui.Text(c, fmt.Sprintf("%d 个文件", len(a.editorSessions))).FontSize(9).
				Font("Cascadia Code").TextColor(colMuted2).NoWrap()
		}

		if primaryAction(c, "editor-save", "保存", iconSave).Disabled(es.saving).Clicked() {
			a.saveEditor(es)
		}

		close := ui.ButtonBase(c.Key("editor-window-close")).Size(28, 28).Radius(7).
			Center().Tooltip("关闭编辑器").Label("关闭编辑器")
		if close.Hovered() {
			close.Background(t.SurfaceHover)
		}
		close.Children(func() { iconEl(c, iconClose, 13, colMuted) })
		if close.Clicked() {
			a.closeEditorWindow()
		}
	})
}

func (a *App) editorTabStrip(c *ui.Context, t *ui.Theme) {
	ui.ScrollHorizontal(c).Height(35).Shrink(0).FillWidth().Scrollbars(false).
		Background(colPanel).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Row(c).Height(35).Padding(0, 10).Gap(4).AlignItems(ui.Center).Children(func() {
			for i, es := range a.editorSessions {
				i, es := i, es
				active := i == a.editorIndex
				dirty := es.buffer.Modified()
				var closed bool

				tab := ui.ButtonBase(c.Key(fmt.Sprintf("editor-file-%d", es.id))).
					Height(27).Padding(0, 8).Gap(6).Radius(6).AlignItems(ui.Center).
					Label("编辑文件 " + es.name).Tooltip(es.remote)
				tab = styleTabPill(tab, active)
				tab.Children(func() {
					statusDot(c, condColor(dirty, colAmber, colMuted2), 6)
					ui.Text(c, shortLabel(es.name, 20)).FontSize(10.5).
						FontWeight(condWeight(active, 600, 500)).
						TextColor(condColor(active, colText, colMuted)).NoWrap()
					if dirty {
						ui.Text(c, "*").FontSize(10).TextColor(colAmber)
					}

					close := ui.ButtonBase(c.Key(fmt.Sprintf("editor-file-close-%d", es.id))).
						Size(17, 17).Radius(4).Center().Tooltip("关闭 " + es.name).
						Label("关闭编辑文件 " + es.name)
					if close.Hovered() {
						close.Background(colLine2)
					}
					close.Children(func() {
						iconEl(c, iconClose, 9.5, condColor(active, colMuted, colMuted2))
					})
					if close.Clicked() {
						closed = true
						a.closeEditorTab(i)
					}
				})
				if tab.Clicked() && !closed {
					a.editorIndex = i
				}
			}
		})
	})
}

func (a *App) saveEditor(es *editorSession) {
	if es == nil || es.saving {
		return
	}

	if st, err := es.browser.client.Stat(es.remote); err == nil {
		if st.Size() != es.baseSize || !st.ModTime().Equal(es.baseMod) {
			res, _ := mygo.Dialog.Message(mygo.MessageOptions{
				Type:               mygo.MessageWarning,
				Message:            "远端文件已被修改，仍要覆盖？",
				Buttons:            []string{"取消", "覆盖"},
				DestructiveButtons: []int{1},
			})
			if res.Button != 1 {
				return
			}
		}
	}

	f, err := os.Create(es.local)
	if err != nil {
		es.err = "写入临时文件失败: " + err.Error()
		return
	}
	if _, err := es.buffer.WriteTo(f); err != nil {
		f.Close()
		es.err = "写入临时文件失败: " + err.Error()
		return
	}
	if err := f.Close(); err != nil {
		es.err = "写入临时文件失败: " + err.Error()
		return
	}

	es.saving = true
	es.err = ""
	go func() {
		err := uploadFile(a.ctx, es.browser.client, es.local, es.remote, nil)
		a.post(func() {
			es.saving = false
			if err != nil {
				es.err = "保存失败: " + err.Error()
				return
			}

			es.buffer.MarkSaved()
			if st, err := es.browser.client.Stat(es.remote); err == nil {
				es.baseSize = st.Size()
				es.baseMod = st.ModTime()
			}
			es.browser.loadRemote()
			a.status = "已保存 " + es.name
		})
	}()
}

func (a *App) closeEditorWindow() {
	if len(a.editorSessions) == 0 {
		return
	}
	for _, es := range a.editorSessions {
		if es.saving {
			a.status = "正在保存，请稍后"
			return
		}
	}

	dirty := 0
	for _, es := range a.editorSessions {
		if es.buffer.Modified() {
			dirty++
		}
	}
	if dirty > 0 {
		message := "有未保存的修改，放弃并关闭编辑器？"
		if dirty > 1 {
			message = fmt.Sprintf("有 %d 个文件未保存，放弃并关闭编辑器？", dirty)
		}
		res, _ := mygo.Dialog.Message(mygo.MessageOptions{
			Type:               mygo.MessageWarning,
			Message:            message,
			Buttons:            []string{"取消", "放弃并关闭"},
			DestructiveButtons: []int{1},
		})
		if res.Button != 1 {
			return
		}
	}

	a.discardAllEditors()
	a.status = ""
}

func (a *App) closeEditorTab(i int) {
	if i < 0 || i >= len(a.editorSessions) {
		return
	}
	es := a.editorSessions[i]
	if es.saving {
		a.status = "正在保存，请稍后"
		return
	}
	if es.buffer.Modified() {
		res, _ := mygo.Dialog.Message(mygo.MessageOptions{
			Type:               mygo.MessageWarning,
			Message:            es.name + " 有未保存的修改，放弃并关闭？",
			Buttons:            []string{"取消", "放弃并关闭"},
			DestructiveButtons: []int{1},
		})
		if res.Button != 1 {
			return
		}
	}
	a.dropEditor(i)
}

func (a *App) dropEditor(i int) {
	if i < 0 || i >= len(a.editorSessions) {
		return
	}
	es := a.editorSessions[i]
	os.Remove(es.local)
	a.editorSessions = append(a.editorSessions[:i], a.editorSessions[i+1:]...)
	if len(a.editorSessions) == 0 {
		a.editorIndex = 0
		return
	}
	switch {
	case i < a.editorIndex:
		a.editorIndex--
	case a.editorIndex >= len(a.editorSessions):
		a.editorIndex = len(a.editorSessions) - 1
	}
}

func (a *App) discardEditorsForTerm(termID int64) {
	for i := len(a.editorSessions) - 1; i >= 0; i-- {
		if a.editorSessions[i].termID == termID {
			a.dropEditor(i)
		}
	}
}

func (a *App) discardAllEditors() {
	for _, es := range a.editorSessions {
		os.Remove(es.local)
	}
	a.editorSessions = nil
	a.editorIndex = 0
}

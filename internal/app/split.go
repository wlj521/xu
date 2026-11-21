package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

type splitLayout int

const (
	splitLayoutColumns splitLayout = iota
	splitLayoutRows
)

func (l splitLayout) label() string {
	if l == splitLayoutRows {
		return "上下分屏"
	}
	return "左右分屏"
}

func (l splitLayout) workspaceName() string {
	if l == splitLayoutRows {
		return "rows"
	}
	return "columns"
}

func splitLayoutFromWorkspace(name string) (splitLayout, bool) {
	switch name {
	case "", "columns":
		return splitLayoutColumns, true
	case "rows":
		return splitLayoutRows, true
	}
	return splitLayoutColumns, false
}

// terminalSplit refers to app tabs rather than sessions so a tab can finish
// connecting or reconnect in place without losing its place in the layout.
type terminalSplit struct {
	firstID  int64
	secondID int64
	layout   splitLayout
	ratio    float32
}

func (a *App) splitTab(tabID int64) *appTab {
	if idx := a.tabIndexByID(tabID); idx >= 0 {
		return a.tabs[idx]
	}
	return nil
}

func splitTabReady(tab *appTab) bool {
	return tab != nil && tab.kind == tabTerminal && tab.term != nil
}

// splitTabs returns the pair only once both tabs hold live terminal views.
// While a restored split is still connecting, the shell keeps showing the
// active tab normally.
func (a *App) splitTabs() (*appTab, *appTab) {
	s := a.terminalSplit
	if s == nil || s.firstID == s.secondID {
		return nil, nil
	}
	first, second := a.splitTab(s.firstID), a.splitTab(s.secondID)
	if !splitTabReady(first) || !splitTabReady(second) {
		return nil, nil
	}
	return first, second
}

func (a *App) splitActive() bool {
	s := a.terminalSplit
	if s == nil {
		return false
	}
	tab := a.activeTabRef()
	return tab != nil && (tab.id == s.firstID || tab.id == s.secondID)
}

func (a *App) splitContains(tabID int64) bool {
	s := a.terminalSplit
	return s != nil && (tabID == s.firstID || tabID == s.secondID)
}

func (a *App) splitReady() bool {
	if !a.splitActive() {
		return false
	}
	first, second := a.splitTabs()
	return first != nil && second != nil
}

func (a *App) pruneTerminalSplit() {
	s := a.terminalSplit
	if s == nil {
		return
	}
	first, second := a.splitTab(s.firstID), a.splitTab(s.secondID)
	if first == nil || second == nil || first == second ||
		first.kind == tabQuickConnect || second.kind == tabQuickConnect ||
		!a.splitActive() {
		a.clearTerminalSplit()
	}
}

func (a *App) clearTerminalSplit() {
	if a.terminalSplit == nil {
		return
	}
	a.terminalSplit = nil
	a.splitRatioDirty = false
}

func (a *App) startTerminalSplit(otherID int64, layout splitLayout) {
	active := a.activeTabRef()
	other := a.splitTab(otherID)
	if !splitTabReady(active) || !splitTabReady(other) || active.id == other.id {
		a.status = "至少需要两个已打开的终端标签"
		return
	}

	a.terminalSplit = &terminalSplit{
		firstID:  active.id,
		secondID: other.id,
		layout:   layout,
		ratio:    50,
	}
	active.term.focusRequest = true
	a.splitRatioDirty = false
	a.status = fmt.Sprintf("%s · %s | %s", layout.label(),
		a.tabDisplayTitle(active), a.tabDisplayTitle(other))
	a.saveWorkspace()
}

func (a *App) setTerminalSplitLayout(layout splitLayout) {
	if a.terminalSplit == nil {
		return
	}
	a.terminalSplit.layout = layout
	a.status = "已切换为" + layout.label()
	a.saveWorkspace()
}

func (a *App) replaceTerminalSplitPane(tabID int64) {
	s := a.terminalSplit
	active := a.activeTabRef()
	replacement := a.splitTab(tabID)
	if s == nil || active == nil || !splitTabReady(replacement) {
		return
	}
	if active.id != s.firstID && active.id != s.secondID {
		return
	}

	if active.id == s.firstID {
		if replacement.id == s.firstID || replacement.id == s.secondID {
			return
		}
		s.secondID = replacement.id
	} else {
		if replacement.id == s.firstID || replacement.id == s.secondID {
			return
		}
		s.firstID = replacement.id
	}
	active.term.focusRequest = true
	a.status = "分屏另一侧已切换为 " + a.tabDisplayTitle(replacement)
	a.saveWorkspace()
}

func (a *App) focusSplitTab(tabID int64) {
	tab := a.splitTab(tabID)
	if tab == nil || tab.kind != tabTerminal || tab.term == nil {
		return
	}
	if idx := a.tabIndexByID(tabID); idx >= 0 && a.activeTab != idx {
		a.activateTabIndex(idx)
	}
	tab.term.focusRequest = true
}

func (a *App) addSplitCandidate(m *ui.Menu, tab *appTab, idx int, layout splitLayout, replace bool) {
	if tab == nil || !splitTabReady(tab) {
		return
	}
	label := fmt.Sprintf("%d  %s", idx+1, a.tabDisplayTitle(tab))
	if m.Item(label).Chosen() {
		if replace {
			a.replaceTerminalSplitPane(tab.id)
		} else {
			a.startTerminalSplit(tab.id, layout)
		}
	}
}

func (a *App) terminalSplitMenu(m *ui.Menu) {
	m.MaxWidth(420)
	active := a.activeTabRef()
	if !splitTabReady(active) {
		m.Item("当前标签不是可用的终端").Disabled(true)
		return
	}

	if s := a.terminalSplit; s != nil && a.splitActive() {
		m.Item("当前布局：" + s.layout.label()).Disabled(true)
		m.Item("拖动中间分隔条可调整比例").Disabled(true)
		m.Separator()
		other := splitLayoutColumns
		if s.layout == splitLayoutColumns {
			other = splitLayoutRows
		}
		if m.Item("切换为" + other.label()).Chosen() {
			a.setTerminalSplitLayout(other)
		}

		replacement := m.Submenu("更换另一侧会话", func(sub *ui.Menu) {
			n := 0
			for idx, tab := range a.tabs {
				if tab.id == active.id || !splitTabReady(tab) {
					continue
				}
				n++
				a.addSplitCandidate(sub, tab, idx, s.layout, true)
			}
			if n == 0 {
				sub.Item("没有其他终端标签").Disabled(true)
			}
		})
		replacement.Disabled(a.openTerminalTabCount() < 2)

		if m.Item("关闭分屏").Chosen() {
			a.clearTerminalSplit()
			if active.term != nil {
				active.term.focusRequest = true
			}
			a.status = "已退出终端分屏"
			a.saveWorkspace()
		}
		return
	}

	m.Item("选择另一个终端标签进行分屏").Disabled(true)
	m.Separator()
	for _, layout := range []splitLayout{splitLayoutColumns, splitLayoutRows} {
		layout := layout
		n := 0
		item := m.Submenu(layout.label(), func(sub *ui.Menu) {
			for idx, tab := range a.tabs {
				if tab.id == active.id || !splitTabReady(tab) {
					continue
				}
				n++
				a.addSplitCandidate(sub, tab, idx, layout, false)
			}
			if n == 0 {
				sub.Item("没有其他终端标签").Disabled(true)
			}
		})
		item.Disabled(a.openTerminalTabCount() < 2)
	}
	if a.openTerminalTabCount() < 2 {
		m.Separator()
		m.Item("至少需要两个终端标签").Disabled(true)
	}
}

func (a *App) openTerminalTabCount() int {
	n := 0
	for _, tab := range a.tabs {
		if splitTabReady(tab) {
			n++
		}
	}
	return n
}

func (a *App) terminalSplitView(c *ui.Context, first, second *appTab) {
	active := a.activeTabRef()
	var activeID int64
	if active != nil {
		activeID = active.id
	}

	if ts := a.activeTerminal(); ts != nil {
		if !a.finding && a.lastSearch != "" {
			if ts.Term != nil {
				ts.Term.EndSearch()
			}
			a.resetFind()
		}
		if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF) {
			a.openFind()
		}
	}

	firstPane := func() {
		a.terminalSplitPane(c, first, first.id == activeID)
	}
	secondPane := func() {
		a.terminalSplitPane(c, second, second.id == activeID)
	}

	var split ui.Element
	if a.terminalSplit.layout == splitLayoutRows {
		split = ui.SplitVerticalPercent(c, &a.terminalSplit.ratio, firstPane, secondPane)
	} else {
		split = ui.SplitPercent(c, &a.terminalSplit.ratio, firstPane, secondPane)
	}
	split = split.Grow(1).FillWidth()
	split.ObserveInput(func(ev ui.InputEvent) {
		if !a.splitRatioDirty {
			return
		}
		if ev.Kind == ui.InputPointerUp || ev.Kind == ui.InputKeyUp {
			a.saveWorkspace()
			a.splitRatioDirty = false
		}
	})
	if split.Changed() {
		a.splitRatioDirty = true
	}
}

func (a *App) terminalSplitPane(c *ui.Context, tab *appTab, focused bool) {
	if !splitTabReady(tab) {
		return
	}
	ts := tab.term
	t := c.Theme()
	tabID := tab.id

	pane := ui.Column(c).FillHeight().Background(colTermBg)
	pane.ObserveInput(func(ev ui.InputEvent) {
		if ev.Kind == ui.InputPointerDown && ev.Button == 0 && !focused {
			a.focusSplitTab(tabID)
		}
	})
	pane.Children(func() {
		tabIdx := a.tabIndexByID(tabID)
		title := a.tabDisplayTitle(tab)
		headLabel := "聚焦分屏面板 " + title
		head := ui.ButtonBase(c.Key(fmt.Sprintf("split-focus-%d", tabID))).
			FillWidth().Height(28).Shrink(0).Padding(0, 10).Gap(6).
			AlignItems(ui.Center).Label(headLabel).Tooltip(headLabel)
		head = head.Background(condColor(focused, colPanelTop, colPanelDeep)).
			BorderWidth(0, 0, 1, 0).BorderColor(condColor(focused, colAccentLine, colLine))
		head.Children(func() {
			statusDot(c, a.tabDotColor(tab), 6)
			if tabIdx >= 0 {
				ui.Text(c, fmt.Sprintf("%d", tabIdx+1)).FontSize(8.4).
					Font("Cascadia Code").TextColor(colMuted2).NoWrap()
			}
			ui.Text(c, shortLabel(title, 30)).FontSize(9.5).
				FontWeight(condWeight(focused, 650, 500)).
				TextColor(condColor(focused, colText, colMuted)).NoWrap()
			if focused {
				ui.Box(c).Height(16).Padding(0, 5).Radius(3).
					Background(colCyan.Alpha(0.8)).Center().Children(func() {
					ui.Text(c, "活动").FontSize(7.8).TextColor(colPanelDeep).NoWrap()
				})
			}
			ui.Text(c, "").Grow(1)
			if ts.exited {
				ui.Text(c, "已断开").FontSize(8.2).TextColor(colRed).NoWrap()
			}
		})
		if head.Clicked() {
			a.focusSplitTab(tabID)
		}

		if focused && a.finding && ts.Term != nil {
			a.findBar(c, ts)
		}
		if ts.local && ts.exited {
			ui.Row(c).Padding(5, 10).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "本地终端已退出").FontSize(9).TextColor(colAmber)
				if ui.Button(c, "新开本地终端").Clicked() {
					a.OpenLocalTerminal()
				}
			})
		}
		if !ts.local && ts.exited {
			ui.Row(c).Padding(5, 10).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "连接已断开").FontSize(9).TextColor(colRed)
				if ui.Button(c, "重连").Disabled(ts.reconnecting).Clicked() {
					a.reconnect(ts)
				}
				if ts.reconnecting {
					ui.Text(c, "重连中…").FontSize(8.6).TextColor(t.TextMuted)
				}
				if ts.reconnectErr != "" {
					ui.Text(c, shortLabel(ts.reconnectErr, 36)).FontSize(8.4).TextColor(colRed)
				}
			})
		}
		terminalView(c, ts, focused)
	})
}

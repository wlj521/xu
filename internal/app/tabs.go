package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// tabKind tells what a top tab shows: a terminal session or the
// quick-connect page the user opens with "+".
type tabKind int

const (
	tabTerminal tabKind = iota
	tabQuickConnect
	tabConnecting
)

// appTab is one entry of the top tab strip. Terminal tabs point at their
// session, quick-connect tabs show the saved connections of the profile
// store, and connecting tabs hold a dial that becomes a terminal.
type appTab struct {
	id   int64
	kind tabKind
	term *termSession
	conn *connectingTab
}

func (a *App) newTabID() int64 {
	a.nextTabID++
	return a.nextTabID
}

func (a *App) activeTabRef() *appTab {
	if a.activeTab < 0 || a.activeTab >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.activeTab]
}

// quickTabActive reports whether the tab in front is a quick-connect page.
func (a *App) quickTabActive() bool {
	tab := a.activeTabRef()
	return tab != nil && tab.kind == tabQuickConnect
}

func (a *App) termTabIndex(term *termSession) int {
	for i, tab := range a.tabs {
		if tab.kind == tabTerminal && tab.term == term {
			return i
		}
	}
	return -1
}

func (a *App) termIndex(term *termSession) int {
	for i, ts := range a.terms {
		if ts == term {
			return i
		}
	}
	return -1
}

// openTermTab gives a freshly opened session its tab at the end and focuses it.
func (a *App) openTermTab(term *termSession) {
	if term == nil {
		return
	}
	if idx := a.termTabIndex(term); idx >= 0 {
		a.activateTabIndex(idx)
		return
	}
	a.tabs = append(a.tabs, &appTab{id: a.newTabID(), kind: tabTerminal, term: term})
	a.activeTab = len(a.tabs) - 1
	if idx := a.termIndex(term); idx >= 0 {
		a.activeTerm = idx
	}
}

// openQuickTab appends a quick-connect page and puts it in front.
func (a *App) openQuickTab() {
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	if a.finding {
		a.closeFind()
	}
	a.tabs = append(a.tabs, &appTab{id: a.newTabID(), kind: tabQuickConnect})
	a.activeTab = len(a.tabs) - 1
	a.activeTerm = -1
	a.formOpen = false
	a.pickerOpen = false
	a.filesOpen = false
	a.monitorOpen = false
	a.forwardOpen = false
	a.batchOpen = false
	a.settingsOpen = false
	a.managerErr = ""
	a.connectErr = ""
}

// activateTabIndex focuses the tab at index i, closing the find bar of the
// session that leaves the front.
func (a *App) activateTabIndex(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	if a.finding {
		a.closeFind()
	}
	tab := a.tabs[i]
	a.activeTab = i
	a.formOpen = false
	a.pickerOpen = false
	a.filesOpen = false
	a.monitorOpen = false
	a.forwardOpen = false
	a.batchOpen = false
	a.settingsOpen = false
	if tab.kind == tabQuickConnect || tab.term == nil {
		a.activeTerm = -1
		return
	}
	a.activeTerm = a.termIndex(tab.term)
}

// activateTerm keeps the old session-index API: it focuses the tab that
// holds the i-th session of a.terms.
func (a *App) activateTerm(i int) {
	if i < 0 || i >= len(a.terms) {
		return
	}
	a.syncTabs()
	if idx := a.termTabIndex(a.terms[i]); idx >= 0 {
		a.activateTabIndex(idx)
		return
	}
	a.activeTerm = i
}

// syncTabs keeps the tab strip in step with a.terms: sessions opened behind
// the model's back (tests, legacy paths) get a tab, closed ones lose theirs.
func (a *App) syncTabs() {
	inTerms := make(map[*termSession]bool, len(a.terms))
	for _, ts := range a.terms {
		inTerms[ts] = true
	}

	kept := make([]*appTab, 0, len(a.tabs)+len(a.terms))
	seen := make(map[*termSession]bool, len(a.terms))
	for _, tab := range a.tabs {
		if tab.kind == tabTerminal {
			if tab.term == nil || !inTerms[tab.term] || seen[tab.term] {
				continue
			}
			seen[tab.term] = true
		}
		kept = append(kept, tab)
	}
	for _, ts := range a.terms {
		if !seen[ts] {
			kept = append(kept, &appTab{id: a.newTabID(), kind: tabTerminal, term: ts})
			seen[ts] = true
		}
	}
	a.tabs = kept

	if len(a.tabs) == 0 {
		a.activeTab = 0
		a.activeTerm = -1
		return
	}
	if a.activeTab < 0 || a.activeTab >= len(a.tabs) {
		a.activeTab = len(a.tabs) - 1
	}
	if tab := a.activeTabRef(); tab != nil && tab.kind == tabTerminal && tab.term != nil {
		if idx := a.termIndex(tab.term); idx >= 0 {
			a.activeTerm = idx
		}
	}
	if front := a.activeTabRef(); front != nil && (front.kind == tabQuickConnect || front.kind == tabConnecting) {
		a.activeTerm = -1
	}
}

// tabTitle is the short name shown on the tab: the saved profile name where
// there is one, the shell title otherwise.
func (a *App) tabTitle(tab *appTab) string {
	if tab == nil {
		return ""
	}
	if tab.kind == tabConnecting {
		if tab.conn != nil {
			return profileDisplayName(tab.conn.profile)
		}
		return "新标签页"
	}
	if tab.kind == tabQuickConnect || tab.term == nil {
		return "新标签页"
	}
	term := tab.term
	if term.local {
		if term.Title != "" {
			return term.Title
		}
		return "本地终端"
	}
	if p := a.profileForTerm(term); p != nil && p.Name != "" {
		return p.Name
	}
	if term.Title != "" {
		return term.Title
	}
	return term.profile.Username + "@" + term.profile.Host
}

// attachSessionTab gives a session that just finished connecting its tab:
// the quick-connect page it was started from becomes the session tab, else a
// tab is appended.
func (a *App) attachSessionTab(term *termSession) {
	if term == nil {
		return
	}
	if a.pendingQuickTab != 0 {
		for i, tab := range a.tabs {
			if tab.kind == tabQuickConnect && tab.id == a.pendingQuickTab {
				a.tabs[i] = &appTab{id: a.newTabID(), kind: tabTerminal, term: term}
				a.activeTab = i
				if idx := a.termIndex(term); idx >= 0 {
					a.activeTerm = idx
				}
				a.pendingQuickTab = 0
				return
			}
		}
		a.pendingQuickTab = 0
	}
	a.openTermTab(term)
}

// tabTitleCount counts the tabs that would show the same name, so duplicate
// sessions of one server get distinct labels for screen readers and tests.
func (a *App) tabTitleCount(title string) int {
	n := 0
	for _, tab := range a.tabs {
		if a.tabTitle(tab) == title {
			n++
		}
	}
	return n
}

// tabDisplayTitle is the name a tab shows. Tabs of the same server (or
// several quick-connect pages) are numbered 1, 2, 3… so they can be told
// apart; a unique tab keeps its plain name.
func (a *App) tabDisplayTitle(tab *appTab) string {
	base := a.tabTitle(tab)
	if a.tabTitleCount(base) <= 1 {
		return base
	}
	n := 0
	for _, other := range a.tabs {
		if a.tabTitle(other) != base {
			continue
		}
		n++
		if other == tab {
			return fmt.Sprintf("%s %d", base, n)
		}
	}
	return base
}

func (a *App) tabDotColor(tab *appTab) ui.Color {
	if tab != nil && tab.kind == tabConnecting {
		if tab.conn != nil && tab.conn.err != "" {
			return colRed
		}
		return colCyan
	}
	if tab == nil || tab.kind == tabQuickConnect {
		return colCyan
	}
	if tab.term == nil {
		return colMuted2
	}
	if tab.term.exited {
		return colRed
	}
	if tab.term.local {
		return colViolet
	}
	return colGreen
}

// closeTabIndex closes the tab at i. Closing a terminal tab releases its
// monitor, forwards and SFTP session, like the old session close did.
func (a *App) closeTabIndex(i int) {
	if i < 0 || i >= len(a.tabs) {
		return
	}
	closingActive := i == a.activeTab
	tab := a.tabs[i]
	a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)

	switch tab.kind {
	case tabTerminal:
		if tab.term != nil {
			a.closeTermSession(tab.term)
		}
	case tabConnecting:
		// Closing the tab is how the user cancels a dial: cancel it, and
		// clear the progress state when this tab owns the in-flight connect.
		if tab.conn != nil && tab.conn.cancel != nil {
			tab.conn.cancel()
		}
		if a.connTabID == tab.id {
			a.stopConnectTimer()
			a.connecting = false
			a.connectingProfileID = 0
			a.pendingQuickTab = 0
			a.connTabID = 0
			a.status = ""
		}
	}

	if len(a.tabs) == 0 {
		a.activeTab = 0
		a.activeTerm = -1
		a.status = "没有打开的会话，点 + 快速连接"
		return
	}
	if a.activeTab > i || a.activeTab >= len(a.tabs) {
		a.activeTab--
	}
	if a.activeTab < 0 {
		a.activeTab = 0
	}
	if closingActive {
		a.activateTabIndex(a.activeTab)
		return
	}
	// A background tab closed: keep the front tab, its find bar and the
	// overlays exactly as they were, only re-pointing the session index.
	if front := a.activeTabRef(); front != nil && front.kind == tabTerminal && front.term != nil {
		a.activeTerm = a.termIndex(front.term)
	}
}

func (a *App) tabIndexByID(id int64) int {
	for i, tab := range a.tabs {
		if tab.id == id {
			return i
		}
	}
	return -1
}

func (a *App) tabContextMenu(m *ui.Menu, tab *appTab) {
	if tab == nil {
		return
	}
	if m.Item("复制标签").Chosen() {
		a.duplicateTab(tab)
	}
	m.Separator()
	if m.Item("连接").Chosen() {
		a.connectTab(tab)
	}
	if m.Item("连接全部").Chosen() {
		a.connectAllTabs()
	}
	if m.Item("断开").Chosen() {
		a.disconnectTab(tab)
	}
	m.Separator()
	if m.Item("关闭").Chosen() {
		a.closeTabByID(tab.id)
	}
	if m.Item("关闭其他").Chosen() {
		a.closeOtherTabs(tab.id)
	}
	if m.Item("关闭全部").Chosen() {
		a.closeAllTabs()
	}
}

func (a *App) duplicateTab(tab *appTab) {
	if tab == nil || a.tabIndexByID(tab.id) < 0 {
		return
	}
	if tab.kind == tabQuickConnect {
		a.openQuickTab()
		a.status = "已复制快速连接标签"
		return
	}
	if tab.kind == tabConnecting {
		a.status = "标签正在连接，请稍候"
		return
	}
	ts := tab.term
	if ts == nil {
		return
	}
	if ts.local {
		a.duplicateLocalTab()
		return
	}

	p, profileID := ts.profile, ts.profileID
	title := a.tabDisplayTitle(tab)
	a.status = "正在复制标签 " + title + "…"
	go func() {
		client, err := a.dialWithPrompt(p)
		if err != nil {
			a.post(func() { a.status = "复制标签失败: " + err.Error() })
			return
		}
		newTS, err := newTermSession(client, p, 100, 30, a.post, a.onTermSearch, a.termDisplay())
		if err != nil {
			client.Close()
			a.post(func() { a.status = "复制标签失败: " + err.Error() })
			return
		}
		newTS.profileID = profileID
		a.post(func() {
			if a.ctx.Err() != nil {
				newTS.Close()
				return
			}
			a.terms = append(a.terms, newTS)
			a.openTermTab(newTS)
			a.finding = false
			a.resetFind()
			a.filesOpen = false
			a.monitorOpen = false
			a.forwardOpen = false
			a.batchOpen = false
			a.settingsOpen = false
			a.autoStartForwards(newTS)
			a.startMonitorBackground(newTS)
			a.status = "已复制标签 · " + title
		})
	}()
}

func (a *App) duplicateLocalTab() {
	ts, err := newLocalTerminal(a.post, a.onTermSearch, a.termDisplay())
	if err != nil {
		a.status = "复制本地标签失败: " + err.Error()
		return
	}
	a.terms = append(a.terms, ts)
	a.openTermTab(ts)
	a.status = "已复制本地标签"
}

func (a *App) connectTab(tab *appTab) {
	if tab == nil {
		return
	}
	if idx := a.tabIndexByID(tab.id); idx >= 0 {
		a.activateTabIndex(idx)
	}
	if tab.kind == tabConnecting {
		if tab.conn != nil && tab.conn.err != "" {
			a.retryConnectingTab(tab)
			return
		}
		a.status = "标签正在连接，请稍候"
		return
	}
	if tab.kind != tabTerminal || tab.term == nil {
		a.status = "快速连接页无需连接"
		return
	}
	ts := tab.term
	if ts.local {
		if ts.exited {
			a.status = "本地终端已退出，请新开标签"
		} else {
			a.status = "本地终端无需连接"
		}
		return
	}
	if ts.reconnecting {
		a.status = "标签正在连接，请稍候"
		return
	}
	if !ts.exited && ts.client != nil {
		a.status = "标签已连接"
		return
	}
	a.status = "正在连接 " + a.tabDisplayTitle(tab) + "…"
	a.reconnect(ts)
}

func (a *App) connectAllTabs() {
	count := 0
	for _, tab := range a.tabs {
		if tab.kind != tabTerminal || tab.term == nil {
			continue
		}
		ts := tab.term
		if ts.local || ts.reconnecting || (!ts.exited && ts.client != nil) {
			continue
		}
		a.reconnect(ts)
		count++
	}
	if count == 0 {
		a.status = "没有需要连接的标签"
		return
	}
	a.status = fmt.Sprintf("正在连接 %d 个标签…", count)
}

func (a *App) disconnectTab(tab *appTab) {
	if tab != nil && tab.kind == tabConnecting {
		a.status = "连接尚未建立"
		return
	}
	if tab == nil || tab.kind != tabTerminal || tab.term == nil {
		a.status = "当前标签无法断开"
		return
	}
	a.disconnectSession(tab.term)
}

func (a *App) disconnectSession(ts *termSession) {
	if ts == nil {
		return
	}
	if ts.reconnecting {
		a.status = "标签正在连接，请稍候"
		return
	}
	if ts.exited {
		a.status = "标签已断开"
		return
	}
	if a.editor != nil && a.editor.termID == ts.ID {
		a.discardEditor()
	}
	a.stopMonitor(ts.ID)
	a.stopForwards(ts.ID)
	if b := a.browsers[ts.ID]; b != nil {
		b.Close()
		delete(a.browsers, ts.ID)
	}
	if ts == a.activeTerminal() {
		a.filesOpen = false
		a.monitorOpen = false
		a.forwardOpen = false
		a.finding = false
		a.resetFind()
	}

	ts.exited = true
	ts.reconnectErr = ""
	if ts.local {
		if ts.Term != nil {
			ts.Term.Close()
		}
	} else {
		if ts.conn != nil {
			_ = ts.conn.Close()
		}
		if ts.client != nil {
			_ = ts.client.Close()
		}
	}
	a.status = "已断开 " + shortLabel(ts.Title, 28)
}

func (a *App) closeTabByID(id int64) {
	if idx := a.tabIndexByID(id); idx >= 0 {
		a.closeTabIndex(idx)
	}
}

func (a *App) closeOtherTabs(keepID int64) {
	if a.tabIndexByID(keepID) < 0 {
		return
	}
	for len(a.tabs) > 1 {
		idx := -1
		for i, tab := range a.tabs {
			if tab.id != keepID {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		a.closeTabIndex(idx)
	}
	if idx := a.tabIndexByID(keepID); idx >= 0 {
		a.activateTabIndex(idx)
	}
}

func (a *App) closeAllTabs() {
	for len(a.tabs) > 0 {
		a.closeTabIndex(len(a.tabs) - 1)
	}
}

// tabWidth estimates one tab's width from its measured texts, so the tab
// scroller can be sized to its content.
func (a *App) tabWidth(c *ui.Context, i int, tab *appTab) float32 {
	numW, _ := c.MeasureText(0, ui.Span{Text: fmt.Sprintf("%d", i+1), Size: 8.8, Font: "Cascadia Code"})
	nameW, _ := c.MeasureText(0, ui.Span{Text: shortLabel(a.tabDisplayTitle(tab), 12), Size: 11.2, Weight: 600})
	w := float32(65) + numW + nameW
	if tab.kind == tabQuickConnect {
		chipW, _ := c.MeasureText(0, ui.Span{Text: "快速连接", Size: 8})
		// Chip padding plus the gap before it; the old estimate clipped the
		// chip's right edge in the scroll viewport.
		w += 14 + chipW
	}
	if tab.kind == tabConnecting {
		chipW, _ := c.MeasureText(0, ui.Span{Text: "连接中", Size: 8})
		w += 14 + chipW
	}
	return w
}

// tabsStripScrollWidth is the tab scroller's width: the tabs' own width
// while they fit, the room left between the manager button and "+" when they
// do not, so "+" never scrolls out of the strip.
func (a *App) tabsStripScrollWidth(c *ui.Context, quick bool) float32 {
	content := float32(0)
	for i, tab := range a.tabs {
		content += a.tabWidth(c, i, tab) + 4
	}
	w, _ := c.Size()
	fixed := float32(200) // manager, divider, "+", gaps, padding and actions
	if quick {
		if w-monitorWidth(c) > 620 {
			fixed += 150 // the quick-connect hint
		}
	}
	avail := w - monitorWidth(c) - fixed
	if avail < 120 {
		avail = 120
	}
	if content < avail {
		return content
	}
	return avail
}

// tabStrip is the FinalShell-like tab bar under the title bar: the
// connection manager (folder), the open tabs in a horizontal scroller,
// "+", and the active session's actions.
func (a *App) tabStrip(c *ui.Context, t *ui.Theme) {
	a.syncTabs()
	ts := a.activeTerminal()
	quick := a.quickTabActive()

	ui.Row(c).Height(tabStripHeight).Shrink(0).Padding(0, 10).Gap(4).AlignItems(ui.Center).
		Background(colPanelTop).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		manager := ui.ButtonBase(c.Key("tab-manager")).Size(32, tabPillHeight).Radius(tabPillRadius).Center().
			Tooltip("连接管理器").Label("连接管理器")
		manager.Border(1, ui.Transparent)
		if manager.Hovered() || a.managerOpen {
			manager.Background(colPanel)
			manager.Border(1, colLine2)
		}
		manager.Children(func() {
			iconEl(c, iconManager, 20, colCyan)
		})
		if manager.Clicked() {
			a.openConnectionManager()
		}

		ui.Box(c).Width(1).Height(20).Background(colLine).MarginX(2)

		// Tabs scroll sideways when they outgrow the strip; the scroller is
		// only as wide as the tabs while they fit, so "+" sits right after
		// the last tab, and stays pinned when they do not.
		ui.ScrollHorizontal(c).Width(a.tabsStripScrollWidth(c, quick)).Height(tabStripHeight).
			Scrollbars(false).Children(func() {
			ui.Row(c).Height(tabStripHeight).AlignItems(ui.Center).Gap(4).Children(func() {
				for i, tab := range a.tabs {
					i, tab := i, tab
					active := i == a.activeTab
					title := a.tabDisplayTitle(tab)
					var closed bool
					tabEl := ui.ButtonBase(c.Key(fmt.Sprintf("tab-%d", tab.id))).
						Height(tabPillHeight).Padding(0, 10).Gap(6).Radius(tabPillRadius).AlignItems(ui.Center).
						Label(fmt.Sprintf("标签页 %d：%s", i+1, title)).
						Tooltip(fmt.Sprintf("标签页 %d · %s", i+1, title))
					tabEl = styleTabPill(tabEl, active)
					tabEl.ContextMenu(func(m *ui.Menu) {
						a.tabContextMenu(m, tab)
					})
					tabEl.Children(func() {
						idxColor := colMuted2
						nameColor := colMuted
						closeColor := colMuted2
						if active {
							idxColor = colCyan.Alpha(0.8)
							nameColor = colText
							closeColor = colMuted
						}
						ui.Text(c, fmt.Sprintf("%d", i+1)).FontSize(8.8).
							Font("Cascadia Code").TextColor(idxColor).NoWrap()
						statusDot(c, a.tabDotColor(tab), 7)
						ui.Text(c, shortLabel(title, 12)).FontSize(11.2).
							FontWeight(condWeight(active, 650, 500)).TextColor(nameColor).NoWrap()
						if tab.kind == tabQuickConnect {
							ui.Box(c).Height(16).Padding(0, 4).Radius(3).
								Background(colCyan.Alpha(0.07)).Center().Children(func() {
								ui.Text(c, "快速连接").FontSize(8).TextColor(colCyan.Alpha(0.9)).NoWrap()
							})
						}
						if tab.kind == tabConnecting {
							chip, tint := "连接中", colCyan
							if tab.conn != nil && tab.conn.err != "" {
								chip, tint = "失败", colRed
							}
							ui.Box(c).Height(16).Padding(0, 4).Radius(3).
								Background(tint.Alpha(0.07)).Center().Children(func() {
								ui.Text(c, chip).FontSize(8).TextColor(tint.Alpha(0.9)).NoWrap()
							})
						}
						closeLabel := "关闭 " + title
						close := ui.ButtonBase(c.Key(fmt.Sprintf("tab-close-%d", tab.id))).
							Size(18, 18).Radius(4).Center().Tooltip(closeLabel).
							Label(closeLabel)
						if close.Hovered() {
							close.Background(colLine2)
						}
						close.Children(func() {
							iconEl(c, iconClose, 10.5, closeColor)
						})
						if close.Clicked() {
							closed = true
							a.closeTabIndex(i)
						}
					})
					if tabEl.Clicked() && !closed {
						a.activateTabIndex(i)
					}
				}
			})
		})

		add := ui.ButtonBase(c.Key("tab-new")).Size(30, tabPillHeight).Radius(tabPillRadius).Center().
			Tooltip("新建标签页（快速连接）").Label("新建标签页")
		add.Background(ui.Transparent)
		add.Border(1, ui.Transparent)
		if add.Hovered() {
			add.Background(colPanel2.Alpha(0.72))
		}
		addColor := colText2
		if add.Hovered() {
			addColor = colCyan
		}
		add.Children(func() {
			iconEl(c, iconPlus, 16, addColor)
		})
		if add.Clicked() {
			a.openQuickTab()
			a.status = "从快速连接中选择服务器"
		}

		ui.Text(c, "").Grow(1)

		if quick {
			if w, _ := c.Size(); w-monitorWidth(c) > 620 {
				iconEl(c, iconHelp, 12, colMuted2)
				ui.Text(c, "点击卡片新开标签页").FontSize(9).
					Font("Cascadia Code").TextColor(colMuted2).NoWrap()
			}
			return
		}
		if ts != nil {
			ui.Box(c).Width(1).Height(14).Background(colLine).MarginX(4)
			if a.finding {
				if a.sessionAction(c, t, "ts-search", iconSearch, "查找（Ctrl Shift F）", true, false).Clicked() {
					a.closeFind()
				}
			} else if a.sessionAction(c, t, "ts-search", iconSearch, "查找（Ctrl Shift F）", false, false).Clicked() {
				a.openFind()
			}
			if a.sessionAction(c, t, "ts-clear", iconEraser, "清屏", false, false).Clicked() {
				a.clearTerminal(ts)
			}
		}
	})
}

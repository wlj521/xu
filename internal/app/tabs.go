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
)

// appTab is one entry of the top tab strip. Terminal tabs point at their
// session, quick-connect tabs show the saved connections of the profile store.
type appTab struct {
	id   int64
	kind tabKind
	term *termSession
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
	if a.activeTabRef() != nil && a.activeTabRef().kind == tabQuickConnect {
		a.activeTerm = -1
	}
}

// tabTitle is the short name shown on the tab: the saved profile name where
// there is one, the shell title otherwise.
func (a *App) tabTitle(tab *appTab) string {
	if tab == nil {
		return ""
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

	if tab.kind == tabTerminal && tab.term != nil {
		a.closeTermSession(tab.term)
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

// tabWidth estimates one tab's width from its measured texts, so the tab
// scroller can be sized to its content.
func (a *App) tabWidth(c *ui.Context, i int, tab *appTab) float32 {
	numW, _ := c.MeasureText(0, ui.Span{Text: fmt.Sprintf("%d", i+1), Size: 8.8, Font: "Cascadia Code"})
	nameW, _ := c.MeasureText(0, ui.Span{Text: shortLabel(a.tabDisplayTitle(tab), 12), Size: 11.2, Weight: 600})
	w := float32(63) + numW + nameW
	if tab.kind == tabQuickConnect {
		chipW, _ := c.MeasureText(0, ui.Span{Text: "快速连接", Size: 8})
		w += 6 + chipW
	}
	return w
}

// tabsStripScrollWidth is the tab scroller's width: the tabs' own width
// while they fit, the room left between the manager button and "+" when they
// do not, so "+" never scrolls out of the strip.
func (a *App) tabsStripScrollWidth(c *ui.Context, quick bool) float32 {
	content := float32(0)
	for i, tab := range a.tabs {
		content += a.tabWidth(c, i, tab) + 2
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

	ui.Row(c).Height(34).Shrink(0).Padding(0, 10).Gap(4).AlignItems(ui.Center).
		Background(colPanelTop).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		manager := ui.ButtonBase(c.Key("tab-manager")).Size(40, 34).Radius(4).Center().
			Tooltip("连接管理器").Label("连接管理器")
		if manager.Hovered() || a.managerOpen {
			manager.Background(colPanel)
			manager.Border(1, colLine2)
		}
		manager.Children(func() {
			iconEl(c, iconManager, 23, colCyan)
		})
		if manager.Clicked() {
			a.openConnectionManager()
		}

		ui.Box(c).Width(1).Height(20).Background(colLine).MarginX(2)

		// Tabs scroll sideways when they outgrow the strip; the scroller is
		// only as wide as the tabs while they fit, so "+" sits right after
		// the last tab, and stays pinned when they do not.
		ui.ScrollHorizontal(c).Width(a.tabsStripScrollWidth(c, quick)).Height(34).
			Scrollbars(false).Children(func() {
			ui.Row(c).Height(34).AlignItems(ui.Center).Gap(2).Children(func() {
				for i, tab := range a.tabs {
					i, tab := i, tab
					active := i == a.activeTab
					title := a.tabDisplayTitle(tab)
					var closed bool
					tabEl := ui.ButtonBase(c.Key(fmt.Sprintf("tab-%d", tab.id))).
						Height(34).Padding(0, 9).Gap(6).Radius(0).AlignItems(ui.Center).
						Label(fmt.Sprintf("标签页 %d：%s", i+1, title)).
						Tooltip(fmt.Sprintf("标签页 %d · %s", i+1, title))
					if active {
						tabEl.Background(colPanel)
						tabEl.Border(1, colLine2)
						tabEl.DrawOver(func(p *ui.Painter, r ui.Rect) {
							p.Fill(ui.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: 2.5}, colCyan, 0)
						})
					} else if tabEl.Hovered() {
						tabEl.Background(colPanel2.Alpha(0.7))
					}
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

		add := ui.ButtonBase(c.Key("tab-new")).Size(34, 34).Radius(4).Center().
			Tooltip("新建标签页（快速连接）").Label("新建标签页")
		add.Background(colPanel)
		add.Border(1, colLine2)
		if add.Hovered() {
			add.Background(colPanel2)
			add.BorderColor(colAccentLine)
		}
		addColor := colMuted
		if add.Hovered() {
			addColor = colCyan
		}
		add.Children(func() {
			iconEl(c, iconPlus, 15, addColor)
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

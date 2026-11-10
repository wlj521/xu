package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

// qcGroup is one folder of the quick-connect page and of the connection
// manager: the saved connections that belong to a group, in the order the
// groups were created. A group with id 0 collects the "未分组" connections.
type qcGroup struct {
	id    int64
	name  string
	items []store.Profile
}

// ---------------------------------------------------------------------------
// Quick connect page — the body of a "新标签页" tab.
// ---------------------------------------------------------------------------

// quickConnectView is the FinalShell-style launcher the "+" tab shows: the
// saved connections, grouped, with a search box. Clicking a card opens that
// server in the current tab.
func (a *App) quickConnectView(c *ui.Context, t *ui.Theme) {
	visible := a.quickVisibleProfiles()

	ui.Column(c).Grow(1).FillHeight().Background(colPanelDeep).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(24, 30).Gap(15).Children(func() {
				pageHeader(c, t, iconConnections, "快速连接", a.quickSubtitle(), func() {
					if primaryAction(c, "qc-new", "新建连接", iconPlus).Clicked() {
						a.openConnectForm()
					}
				})

				a.quickToolbar(c, t)
				a.quickStatus(c, t)

				switch {
				case len(a.profiles) == 0:
					a.quickEmptyState(c, t)
				case len(visible) == 0:
					a.quickNoMatch(c, t)
				default:
					for _, g := range a.qcSections(visible, false) {
						a.quickGroupBlock(c, t, g)
					}
					a.quickHint(c)
				}
			})
		})
	})
}

// quickSubtitle is the one-line summary under the page title.
func (a *App) quickSubtitle() string {
	if a.connecting {
		return "正在建立连接，成功后当前标签页会切换成终端"
	}
	if len(a.profiles) == 0 {
		return "还没有保存的服务器，先新建一个 SSH 连接"
	}
	online := 0
	for _, p := range a.profiles {
		if a.profileConnected(p.ID) {
			online++
		}
	}
	if online > 0 {
		return fmt.Sprintf("共 %d 个连接 · %d 个在线 · 点击卡片新开标签页", len(a.profiles), online)
	}
	return fmt.Sprintf("共 %d 个连接 · 点击卡片新开标签页", len(a.profiles))
}

// quickToolbar holds the search box and the page actions.
func (a *App) quickToolbar(c *ui.Context, t *ui.Theme) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		qcSearchField(c, "qc-search", &a.quickFilter, "搜索名称、主机、用户名或分组").Grow(1)
		if ghostAction(c, t, "qc-local", "本机终端", iconTerm).Clicked() {
			a.OpenLocalTerminal()
		}
		if ghostAction(c, t, "qc-manage", "连接管理", iconConnections).Clicked() {
			a.openConnectionManager()
		}
	})
}

// quickStatus reports the in-flight connection and the last failure, so the
// page stays informative while the dial runs in the background.
func (a *App) quickStatus(c *ui.Context, t *ui.Theme) {
	if a.connecting {
		name := "服务器"
		sub := "正在协商算法与认证"
		if p := a.qcProfileByID(a.connectingProfileID); p != nil {
			if p.Name != "" {
				name = p.Name
			}
			sub = fmt.Sprintf("%s@%s:%d · 正在协商算法与认证", p.Username, p.Host, p.Port)
		}
		ui.Box(c).FillWidth().Padding(11, 13).Radius(8).
			Background(colCyan.Alpha(0.07)).Border(1, colCyan.Alpha(0.22)).Children(func() {
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				statusDot(c, colCyan, 7)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, "正在连接 "+name).FontSize(10.8).FontWeight(600).
						TextColor(colCyan).NoWrap()
					ui.Text(c, shortLabel(sub, 64)).FontSize(9.2).Font("Cascadia Code").
						TextColor(colMuted).NoWrap()
				})
				ui.Text(c, "连接中…").FontSize(9).Font("Cascadia Code").
					TextColor(colCyan.Alpha(0.9)).NoWrap()
			})
		})
		return
	}

	if a.connectErr == "" {
		return
	}
	ui.Box(c).FillWidth().Padding(11, 13).Radius(8).
		Background(colRed.Alpha(0.055)).Border(1, colRed.Alpha(0.2)).Children(func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			statusDot(c, colRed, 7)
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "连接失败").FontSize(10.8).FontWeight(600).TextColor(colRed)
				ui.Text(c, shortLabel(a.connectErr, 120)).FontSize(9.4).
					TextColor(colMuted).LineHeight(1.45)
			})
			if a.profileID != 0 {
				if ghostAction(c, t, "qc-err-edit", "编辑连接", iconEdit).Clicked() {
					if p := a.qcProfileByID(a.profileID); p != nil {
						a.editProfile(*p)
					}
				}
			}
			if ghostAction(c, t, "qc-err-close", "关闭提示", iconClose).Clicked() {
				a.connectErr = ""
			}
		})
	})
}

// quickGroupBlock is one group of the launcher: a section label and the cards,
// two per row on wide windows and one per row when the workspace is narrow.
func (a *App) quickGroupBlock(c *ui.Context, t *ui.Theme, g qcGroup) {
	perRow := 2
	if w, _ := c.Size(); w-monitorWidth(c) < 620 {
		perRow = 1
	}
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Row(c).FillWidth().Gap(7).AlignItems(ui.Center).Children(func() {
			iconEl(c, iconFolder, 12, colMuted2)
			ui.Text(c, g.name).FontSize(10).FontWeight(650).LetterSpacing(0.6).
				TextColor(colMuted).NoWrap()
			ui.Text(c, fmt.Sprintf("%d", len(g.items))).FontSize(8.4).
				Font("Cascadia Code").TextColor(colMuted2)
			ui.Box(c).Height(1).Grow(1).Background(colLine)
		})

		for i := 0; i < len(g.items); i += perRow {
			i := i
			ui.Row(c).FillWidth().Gap(10).Children(func() {
				for j := 0; j < perRow && i+j < len(g.items); j++ {
					a.quickCard(c, t, g.items[i+j])
				}
				for j := len(g.items) - i; j < perRow; j++ {
					ui.Text(c, "").Grow(1)
				}
			})
		}
	})
}

// quickCard is one saved connection. The whole card is the click target.
func (a *App) quickCard(c *ui.Context, t *ui.Theme, p store.Profile) {
	connected := a.profileConnected(p.ID)
	connecting := a.connecting && a.connectingProfileID == p.ID

	card := ui.ButtonBase(c.Key(fmt.Sprintf("qc-conn-%d", p.ID))).
		Grow(1).Height(58).Padding(0, 12).Gap(10).Radius(9).
		AlignItems(ui.Center).Label(p.Name)
	card.Background(colPanel)
	card.Border(1, colLine)
	hovered := card.Hovered()
	switch {
	case connecting:
		card.Background(colCyan.Alpha(0.05))
		card.BorderColor(colCyan.Alpha(0.3))
	case connected:
		card.Background(colGreen.Alpha(0.05))
		card.BorderColor(colGreen.Alpha(0.26))
	case hovered:
		card.Background(colSurfaceHover)
		card.BorderColor(colAccentLine)
	}

	card.Children(func() {
		if connected {
			ui.Box(c).Width(2).Height(26).Radius(1).Background(colGreen)
		}
		avatarTint := colMuted
		if connecting {
			avatarTint = colCyan
		} else if connected {
			avatarTint = colGreen
		}
		ui.Box(c).Size(32, 32).Radius(8).Background(colPanel2.Alpha(0.75)).
			Border(1, colLine).Center().Children(func() {
			iconEl(c, iconServer, 15, avatarTint)
		})
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, shortLabel(p.Name, 20)).FontSize(11.5).FontWeight(600).
					TextColor(colText).NoWrap()
				if open := a.openSessionsForProfile(p.ID); open > 1 {
					miniTag(c, fmt.Sprintf("%d 个标签", open), colGreen)
				} else if open == 1 {
					miniTag(c, "在线", colGreen)
				}
			})
			ui.Text(c, shortLabel(fmt.Sprintf("%s@%s:%d", p.Username, p.Host, p.Port), 34)).
				FontSize(9.2).Font("Cascadia Code").TextColor(colMuted).NoWrap()
		})
		if connecting {
			miniTag(c, "连接中", colCyan)
		} else {
			miniTag(c, authLabel(p.AuthType), colMuted2)
			chevron := colMuted2
			if hovered && !connected {
				chevron = colCyan
			}
			iconEl(c, iconChevronRight, 12, chevron)
		}
	})

	card.ContextMenu(func(m *ui.Menu) {
		if m.Item("连接").Chosen() {
			a.connectSavedProfile(p)
		}
		if m.Item("编辑连接").Chosen() {
			a.editProfile(p)
		}
		m.Separator()
		if m.Item("连接管理器").Chosen() {
			a.openConnectionManager()
		}
	})
	if card.Clicked() {
		a.connectSavedProfile(p)
	}
}

// quickEmptyState stands in for the card list when nothing is saved yet.
func (a *App) quickEmptyState(c *ui.Context, t *ui.Theme) {
	ui.Column(c).FillWidth().Center().Padding(46, 30).Gap(9).Children(func() {
		ui.Box(c).Size(54, 54).Radius(14).
			Background(colCyan.Alpha(0.07)).Border(1, colCyan.Alpha(0.2)).Center().Children(func() {
			iconEl(c, iconServer, 24, colCyan.Alpha(0.85))
		})
		ui.Text(c, "还没有保存的连接").FontSize(15).FontWeight(650).TextColor(colText)
		ui.Text(c, "新建一个 SSH 连接，保存后它会出现在这里，点一下就能打开终端").
			FontSize(10.2).TextColor(colMuted).TextAlign(ui.Center)
		ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
			if primaryAction(c, "qc-empty-new", "新建连接", iconPlus).Clicked() {
				a.openConnectForm()
			}
			if ghostAction(c, t, "qc-empty-local", "本机终端", iconTerm).Clicked() {
				a.OpenLocalTerminal()
			}
		})
	})
}

// quickNoMatch reports a search that matched nothing, with a way out.
func (a *App) quickNoMatch(c *ui.Context, t *ui.Theme) {
	ui.Column(c).FillWidth().Center().Padding(40, 30).Gap(7).Children(func() {
		iconEl(c, iconSearch, 20, colMuted2)
		ui.Text(c, fmt.Sprintf("没有匹配「%s」的连接", shortLabel(a.quickFilter, 18))).
			FontSize(fsBody).TextColor(colMuted)
		if ghostAction(c, t, "qc-clear-filter", "清除筛选", iconClose).Clicked() {
			a.quickFilter = ""
		}
	})
}

// quickHint is a quiet line of guidance at the end of the launcher.
func (a *App) quickHint(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Margin(0, 4, 0, 0).Children(func() {
		iconEl(c, iconHelp, 11, colMuted2)
		ui.Text(c, "点击卡片新开标签页（同一服务器可开多个） · 右键可编辑 · 连接管理器整理分组").
			FontSize(9).TextColor(colMuted2).NoWrap()
	})
}

// quickVisibleProfiles applies the launcher's own search box.
func (a *App) quickVisibleProfiles() []store.Profile {
	out := make([]store.Profile, 0, len(a.profiles))
	for _, p := range a.profiles {
		if qcMatches(p, a.groupNameByID(p.GroupID), a.quickFilter) {
			out = append(out, p)
		}
	}
	return out
}

// qcProfileByID looks a saved connection up by id, or reports nil.
func (a *App) qcProfileByID(id int64) *store.Profile {
	if id == 0 {
		return nil
	}
	for i := range a.profiles {
		if a.profiles[i].ID == id {
			return &a.profiles[i]
		}
	}
	return nil
}

// qcSections buckets the connections by group, keeping the order of a.groups
// and putting the ungrouped ones last. With includeEmpty, groups without a
// connection are listed too, so they can be renamed or removed.
func (a *App) qcSections(list []store.Profile, includeEmpty bool) []qcGroup {
	known := make(map[int64]bool, len(a.groups))
	for _, g := range a.groups {
		known[g.ID] = true
	}
	buckets := make(map[int64][]store.Profile, len(a.groups)+1)
	for _, p := range list {
		id := p.GroupID
		if id != 0 && !known[id] {
			// A group that is gone: show the connection under 未分组.
			id = 0
		}
		buckets[id] = append(buckets[id], p)
	}

	out := make([]qcGroup, 0, len(a.groups)+1)
	for _, g := range a.groups {
		items := buckets[g.ID]
		if len(items) == 0 && !includeEmpty {
			continue
		}
		out = append(out, qcGroup{id: g.ID, name: g.Name, items: items})
	}
	if items := buckets[0]; len(items) > 0 {
		out = append(out, qcGroup{id: 0, name: ungroupedName, items: items})
	}
	return out
}

// qcMatches reports whether a saved connection passes a search box.
func qcMatches(p store.Profile, groupName, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	haystack := strings.ToLower(p.Name + " " + p.Host + " " + p.Username + " " + groupName)
	return strings.Contains(haystack, query)
}

// qcSearchField is the search box shared by the page and the manager: an icon,
// the input, and a clear button once something is typed.
func qcSearchField(c *ui.Context, key string, value *string, placeholder string) ui.Element {
	field := ui.Row(c.Key(key)).Height(34).Padding(0, 10).Gap(7).Radius(8).
		Background(colPanel).Border(1, colLine).AlignItems(ui.Center)
	if field.Hovered() {
		field.BorderColor(colLine2)
	}
	field.Children(func() {
		iconEl(c, iconSearch, 13, colMuted2)
		ui.TextInput(c, value).Grow(1).Height(30).FontSize(11).
			Placeholder(placeholder).Background(ui.Transparent).Border(0, ui.Transparent)
		if strings.TrimSpace(*value) != "" {
			if qcIconAction(c, key+"-clear", iconClose, "清除搜索", "清除搜索", false).Clicked() {
				*value = ""
			}
		}
	})
	return field
}

// qcIconAction is the small icon button used inside rows and toolbars.
func qcIconAction(c *ui.Context, key string, icon *ui.SVG, tip, label string, danger bool) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(24, 24).Radius(5).Center().
		Tooltip(tip).Label(label)
	color := colMuted
	if b.Hovered() {
		if danger {
			b.Background(colRed.Alpha(0.1))
			color = colRed
		} else {
			b.Background(colSurfaceHover)
			color = colCyan
		}
	}
	b.Children(func() {
		iconEl(c, icon, 12, color)
	})
	return b
}

// ---------------------------------------------------------------------------
// Connection manager — the "连接管理器" dialog.
// ---------------------------------------------------------------------------

// connectionManagerModal lists every saved connection by group, with the
// inline actions FinalShell's connection manager offers.
func (a *App) connectionManagerModal(c *ui.Context, t *ui.Theme) {
	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}

	dlgW, dlgH := fitDialog(c, 880, 580)
	ui.DialogBase(c, &a.managerOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(dlgW).Height(dlgH).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			a.managerHeader(c, t)
			a.managerToolbar(c, t)
			if a.groupFormOpen {
				a.managerGroupForm(c, t)
			}
			ui.Scroll(c).Grow(1).Padding(8, 12).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					a.managerBody(c, t)
				})
			})
			a.managerFooter(c, t)
		})
	})
	// Escape and a click on the backdrop close the dialog behind the button's
	// back, so a half-typed group name goes with it.
	if !a.managerOpen && a.groupFormOpen {
		a.cancelGroupForm()
	}
}

func (a *App) managerHeader(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(64).Shrink(0).Padding(0, 18).Gap(11).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Box(c).Size(30, 30).Radius(8).
			Background(colCyan.Alpha(0.08)).Border(1, colCyan.Alpha(0.22)).Center().Children(func() {
			iconEl(c, iconConnections, 15, colCyan)
		})
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, "连接管理器").FontSize(14.5).FontWeight(650)
			ui.Text(c, fmt.Sprintf("%d 个连接 · %d 个分组 · 分组决定快速连接与连接管理器的显示顺序",
				len(a.profiles), len(a.groups))).
				FontSize(9.5).TextColor(colMuted).NoWrap()
		})
		if iconButton(c, t, "mgr-close-icon", iconClose, "关闭连接管理器").Clicked() {
			a.closeManager()
		}
	})
}

func (a *App) managerToolbar(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(54).Shrink(0).Padding(0, 16).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		qcSearchField(c, "mgr-search", &a.managerFilter, "搜索名称、主机、用户名或分组").Grow(1)
		if ghostAction(c, t, "mgr-new-group", "新建分组", iconNewFolder).Clicked() {
			a.openGroupForm(0, "")
		}
		if primaryAction(c, "mgr-new-conn", "新建连接", iconPlus).Clicked() {
			a.managerOpen = false
			a.openConnectForm()
		}
	})
}

// managerGroupForm is the inline name field for creating or renaming a group.
func (a *App) managerGroupForm(c *ui.Context, t *ui.Theme) {
	title := "新建分组"
	if a.groupEditID != 0 {
		title = "重命名分组"
	}
	ui.Row(c).Height(48).Shrink(0).Padding(0, 16).Gap(7).AlignItems(ui.Center).
		Background(colCyan.Alpha(0.035)).BorderWidth(0, 0, 1, 0).BorderColor(colLine).
		Children(func() {
			iconEl(c, iconFolder, 12, colCyan)
			ui.Text(c, title).FontSize(9.8).FontWeight(600).
				TextColor(colMuted).NoWrap()
			input := ui.TextInput(c, &a.groupName).Grow(1).Height(28).FontSize(10.5).
				Placeholder("分组名称，例如：生产环境").AutoFocus()
			if input.Submitted() {
				a.submitGroupForm()
			}
			if ghostAction(c, t, "mgr-group-save", "保存", iconCheck).Clicked() {
				a.submitGroupForm()
			}
			if ghostAction(c, t, "mgr-group-cancel", "取消", iconClose).Clicked() {
				a.cancelGroupForm()
			}
		})
}

func (a *App) managerBody(c *ui.Context, t *ui.Theme) {
	visible := a.managerVisibleProfiles()
	filtering := strings.TrimSpace(a.managerFilter) != ""

	if len(a.profiles) == 0 && len(a.groups) == 0 {
		a.managerEmpty(c, t)
		return
	}

	sections := a.qcSections(visible, !filtering)
	if len(sections) == 0 {
		a.managerNoMatch(c, t)
		return
	}

	for _, g := range sections {
		collapsed := !filtering && a.collapsedGroups[g.id]
		a.managerGroupHead(c, t, g, collapsed)
		if collapsed {
			continue
		}
		if len(g.items) == 0 {
			ui.Text(c, "暂无连接").FontSize(9).TextColor(colMuted2).Padding(2, 14, 8, 14)
			continue
		}
		for _, p := range g.items {
			p := p
			a.managerRow(c, t, p)
		}
	}
}

func (a *App) managerEmpty(c *ui.Context, t *ui.Theme) {
	ui.Column(c).FillWidth().Center().Padding(46, 30).Gap(9).Children(func() {
		ui.Box(c).Size(52, 52).Radius(13).
			Background(colPanel2.Alpha(0.8)).Border(1, colLine).Center().Children(func() {
			iconEl(c, iconConnections, 22, colMuted2)
		})
		ui.Text(c, "还没有保存的连接").FontSize(14).FontWeight(650).TextColor(colText)
		ui.Text(c, "新建一个 SSH 连接，或先建分组来整理服务器").
			FontSize(10).TextColor(colMuted).TextAlign(ui.Center)
		ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
			if primaryAction(c, "mgr-empty-new", "新建连接", iconPlus).Clicked() {
				a.managerOpen = false
				a.openConnectForm()
			}
			if ghostAction(c, t, "mgr-empty-group", "新建分组", iconNewFolder).Clicked() {
				a.openGroupForm(0, "")
			}
		})
	})
}

func (a *App) managerNoMatch(c *ui.Context, t *ui.Theme) {
	ui.Column(c).FillWidth().Center().Padding(40, 30).Gap(7).Children(func() {
		iconEl(c, iconSearch, 20, colMuted2)
		ui.Text(c, fmt.Sprintf("没有匹配「%s」的连接", shortLabel(a.managerFilter, 18))).
			FontSize(fsBody).TextColor(colMuted)
		if ghostAction(c, t, "mgr-clear-filter", "清除筛选", iconClose).Clicked() {
			a.managerFilter = ""
		}
	})
}

// managerGroupHead is a folder header: it folds, and carries the group's own
// actions.
func (a *App) managerGroupHead(c *ui.Context, t *ui.Theme, g qcGroup, collapsed bool) {
	online := 0
	for _, p := range g.items {
		if a.profileConnected(p.ID) {
			online++
		}
	}

	ui.Row(c).FillWidth().Height(34).Gap(2).AlignItems(ui.Center).Children(func() {
		head := ui.ButtonBase(c.Key(fmt.Sprintf("mgr-group-%d", g.id))).
			Grow(1).Height(32).Padding(0, 6).Radius(6).Gap(7).AlignItems(ui.Center).
			Label("分组 " + g.name)
		if head.Hovered() {
			head.Background(colPanel2)
		}
		head.Children(func() {
			chevron := iconChevron
			if collapsed {
				chevron = iconChevronRight
			}
			iconEl(c, chevron, 12, colMuted2)
			folderTint := colMuted2
			if g.id != 0 {
				folderTint = colMuted
			}
			iconEl(c, iconFolder, 13, folderTint)
			ui.Text(c, shortLabel(g.name, 22)).FontSize(10.2).FontWeight(650).
				TextColor(colText2).NoWrap()
			ui.Text(c, fmt.Sprintf("%d", len(g.items))).FontSize(8.2).
				Font("Cascadia Code").TextColor(colMuted2)
			ui.Text(c, "").Grow(1)
			if online > 0 {
				miniTag(c, fmt.Sprintf("%d 在线", online), colGreen)
			}
		})
		if head.Clicked() {
			a.collapsedGroups[g.id] = !collapsed
		}

		if g.id == 0 {
			return
		}
		if qcIconAction(c, fmt.Sprintf("mgr-group-add-%d", g.id), iconPlus,
			"新建连接", "在分组 "+g.name+" 中新建连接", false).Clicked() {
			a.managerOpen = false
			a.openConnectForm()
			if a.formOpen {
				a.groupChoice = g.name
			}
		}
		if qcIconAction(c, fmt.Sprintf("mgr-group-edit-%d", g.id), iconEdit,
			"重命名分组", "重命名分组 "+g.name, false).Clicked() {
			a.openGroupForm(g.id, g.name)
		}
		if qcIconAction(c, fmt.Sprintf("mgr-group-del-%d", g.id), iconTrash,
			"删除分组", "删除分组 "+g.name, true).Clicked() {
			a.deleteGroup(store.Group{ID: g.id, Name: g.name})
		}
	})
}

// managerRow is one saved connection with its inline actions.
func (a *App) managerRow(c *ui.Context, t *ui.Theme, p store.Profile) {
	connected := a.profileConnected(p.ID)
	connecting := a.connecting && a.connectingProfileID == p.ID
	selected := a.managerSel == p.ID

	ui.Row(c).FillWidth().Height(48).Gap(2).Padding(0, 4).AlignItems(ui.Center).Children(func() {
		main := ui.ButtonBase(c.Key(fmt.Sprintf("mgr-row-%d", p.ID))).
			Grow(1).Height(42).Padding(0, 10).Radius(6).Gap(10).
			AlignItems(ui.Center).Label(p.Name)
		if selected {
			main.Background(colCyan.Alpha(0.06))
			main.BorderWidth(0, 0, 0, 2)
			main.BorderColor(colCyan)
		} else if main.Hovered() {
			main.Background(colPanel2)
		}
		main.Children(func() {
			avatarTint := colMuted
			if connecting {
				avatarTint = colCyan
			} else if connected {
				avatarTint = colGreen
			}
			ui.Box(c).Size(28, 28).Radius(7).
				Background(colPanel2.Alpha(0.75)).Border(1, colLine).Center().Children(func() {
				iconEl(c, iconServer, 13, avatarTint)
			})
			ui.Column(c).Grow(1).Gap(1).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, shortLabel(p.Name, 26)).FontSize(11.2).FontWeight(600).
						TextColor(colText).NoWrap()
					if connecting {
						statusDot(c, colCyan, 5)
					} else if connected {
						statusDot(c, colGreen, 5)
					}
				})
				ui.Text(c, fmt.Sprintf("%s@%s:%d", p.Username, p.Host, p.Port)).
					FontSize(9.2).Font("Cascadia Code").TextColor(colMuted).NoWrap()
			})
			if connecting {
				miniTag(c, "连接中", colCyan)
			} else if open := a.openSessionsForProfile(p.ID); open > 1 {
				miniTag(c, fmt.Sprintf("%d 个标签", open), colGreen)
			} else if connected {
				miniTag(c, "在线", colGreen)
			}
			miniTag(c, authLabel(p.AuthType), colMuted2)
		})
		main.ContextMenu(func(m *ui.Menu) {
			if m.Item("连接").Chosen() {
				a.connectSavedProfile(p)
			}
			if m.Item("编辑连接").Chosen() {
				a.managerOpen = false
				a.editProfile(p)
			}
			m.Separator()
			if m.Item("删除连接").Chosen() {
				a.removeProfile(p)
			}
		})
		if main.Clicked() {
			a.managerSel = p.ID
		}
		if main.DoubleClicked() {
			a.connectSavedProfile(p)
		}

		if qcIconAction(c, fmt.Sprintf("mgr-connect-%d", p.ID), iconLink,
			"连接", "连接 "+p.Name, false).Clicked() {
			a.connectSavedProfile(p)
		}
		if qcIconAction(c, fmt.Sprintf("mgr-edit-%d", p.ID), iconEdit,
			"编辑连接", "编辑连接 "+p.Name, false).Clicked() {
			a.managerOpen = false
			a.editProfile(p)
		}
		if qcIconAction(c, fmt.Sprintf("mgr-delete-%d", p.ID), iconTrash,
			"删除连接", "删除连接 "+p.Name, true).Clicked() {
			a.removeProfile(p)
		}
	})
}

func (a *App) managerFooter(c *ui.Context, t *ui.Theme) {
	online := 0
	for _, p := range a.profiles {
		if a.profileConnected(p.ID) {
			online++
		}
	}
	ui.Row(c).Height(54).Shrink(0).Padding(0, 16).Gap(9).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		if a.managerErr != "" {
			statusDot(c, colRed, 6)
			ui.Text(c, shortLabel(a.managerErr, 72)).FontSize(9.4).
				TextColor(colRed).NoWrap()
		} else {
			dot := colMuted2
			if online > 0 {
				dot = colGreen
			}
			statusDot(c, dot, 6)
			ui.Text(c, fmt.Sprintf("%d 个连接 · %d 个分组 · %d 个在线 · 凭据保存在系统钥匙串",
				len(a.profiles), len(a.groups), online)).
				FontSize(9.4).TextColor(colMuted2).NoWrap()
		}
		ui.Text(c, "").Grow(1)
		if ghostAction(c, t, "mgr-close", "关闭", iconClose).Clicked() {
			a.closeManager()
		}
	})
}

// closeManager dismisses the dialog, dropping a group form the user started
// but never saved.
func (a *App) closeManager() {
	a.managerOpen = false
	if a.groupFormOpen {
		a.cancelGroupForm()
	}
}

// managerVisibleProfiles applies the manager's own search box.
func (a *App) managerVisibleProfiles() []store.Profile {
	out := make([]store.Profile, 0, len(a.profiles))
	for _, p := range a.profiles {
		if qcMatches(p, a.groupNameByID(p.GroupID), a.managerFilter) {
			out = append(out, p)
		}
	}
	return out
}

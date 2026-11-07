package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

type navView int

const (
	navWorkspace navView = iota
	navConnections
	navFiles
	navSettings
)

func (a *App) goNav(v navView) {
	a.nav = v
	a.formOpen = false
	a.pickerOpen = false
	a.batchOpen = false
	a.monitorOpen = false
	a.forwardOpen = false
	a.filesOpen = v == navFiles
	a.settingsOpen = v == navSettings
	if v == navConnections {
		a.refreshProfiles()
		if a.connPageSel == 0 && len(a.profiles) > 0 {
			a.connPageSel = a.profiles[0].ID
		}
	}
}

func (a *App) setTabsVertical(v bool) {
	if a.settings.TabsVertical == v {
		return
	}
	a.settings.TabsVertical = v
	if a.store != nil {
		value := "0"
		if v {
			value = "1"
		}
		_ = a.store.SetSetting(a.ctx, "ui.tabs.vertical", value)
	}
	if v {
		a.status = "标签栏已移到侧边"
	} else {
		a.status = "标签栏已回到顶部"
	}
}

func (a *App) shellRail(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Width(60).FillHeight().Background(colPanelTop).
		BorderWidth(0, 1, 0, 0).BorderColor(colLine).Padding(8, 6).Children(func() {
		a.railButton(c, t, "rail-workspace", iconTerm, "终端", a.nav == navWorkspace, func() {
			a.goNav(navWorkspace)
		})
		a.railButton(c, t, "rail-connections", iconConnections, "连接", a.nav == navConnections, func() {
			a.openConnectionManager()
		})
		a.railButton(c, t, "rail-files", iconFolder, "文件", a.nav == navFiles, func() {
			a.openFiles()
		})
		a.railButton(c, t, "rail-settings", iconGear, "设置", a.nav == navSettings, func() {
			a.openSettings()
		})
		ui.Text(c, "").Grow(1)
		ui.Text(c, "v0.1").FontSize(8.5).Font("Cascadia Code").
			TextColor(colMuted2).TextAlign(ui.Center)
	})
}

func (a *App) railButton(c *ui.Context, t *ui.Theme, key string, icon *ui.SVG, label string, active bool, onClick func()) {
	b := ui.ButtonBase(c.Key(key)).FillWidth().Height(52).Radius(6).Center()
	if active {
		b.Background(colPanel2)
		b.BorderWidth(0, 0, 0, 2).BorderColor(colCyan)
	} else if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	color := colMuted
	iconColor := colMuted2
	if active {
		color = colText
		iconColor = colCyan
	}
	b.Children(func() {
		ui.Column(c).Gap(4).AlignItems(ui.Center).Children(func() {
			iconEl(c, icon, 17, iconColor)
			ui.Text(c, label).FontSize(9).TextColor(color).NoWrap()
		})
	})
	if b.Clicked() {
		onClick()
	}
}

func (a *App) shellSidebar(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Width(264).FillHeight().Background(colPanel).
		BorderWidth(0, 1, 0, 0).BorderColor(colLine).Children(func() {
		a.sidebarWorkspace(c, t)
	})
}

func (a *App) sidebarHead(c *ui.Context, t *ui.Theme, key, title string, count int, onNew func()) {
	ui.Row(c).Height(44).Shrink(0).Padding(0, 10, 0, 12).Gap(7).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(11.5).FontWeight(600).TextColor(colMuted)
		if count >= 0 {
			ui.Text(c, fmt.Sprintf("%d", count)).FontSize(9.5).Font("Cascadia Code").
				TextColor(colMuted2).NoWrap()
		}
		ui.Text(c, "").Grow(1)
		if onNew != nil {
			if iconButton(c, t, key+"-new", iconPlus, "新建连接").Clicked() {
				onNew()
			}
		}
	})
}

func (a *App) sidebarWorkspace(c *ui.Context, t *ui.Theme) {
	a.sidebarConnectionsHead(c, t)
	a.sidebarLocalRow(c, t)
	a.groupFormRow(c, t)

	ui.Column(c).Grow(1).Padding(2, 8, 10, 8).Children(func() {
		ui.Scroll(c).Grow(1).Gap(1).Children(func() {
			if len(a.profiles) == 0 && len(a.groups) == 0 {
				a.sidebarConnectionsEmpty(c)
				return
			}
			if len(a.groups) > 0 {
				for _, g := range a.groups {
					g := g
					profiles := a.profilesInGroup(g.ID)
					a.groupHeaderRow(c, t, g.ID, g.Name, len(profiles), true)
					a.groupBodyRow(c, t, g.ID, profiles)
				}
				ungrouped := a.profilesInGroup(0)
				if len(ungrouped) > 0 {
					a.groupHeaderRow(c, t, 0, ungroupedName, len(ungrouped), false)
					a.groupBodyRow(c, t, 0, ungrouped)
				}
				return
			}
			for _, p := range a.profiles {
				p := p
				a.profileRow(c, t, p, a.profileConnected(p.ID))
			}
		})
	})

	a.sidebarConnectionsFooter(c)
}

func (a *App) sidebarConnectionsHead(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(44).Shrink(0).Padding(0, 12, 0, 14).Gap(7).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "连接").FontSize(11).FontWeight(650).LetterSpacing(0.55).
			TextColor(colMuted).NoWrap()
		ui.Box(c).Height(15).Padding(0, 4).Radius(4).Border(1, colLine).Center().Children(func() {
			ui.Text(c, fmt.Sprintf("%d", len(a.profiles))).FontSize(8.5).
				Font("Cascadia Code").TextColor(colMuted2)
		})
		ui.Text(c, "").Grow(1)
		if sidebarIconButton(c, t, "side-new-group", iconNewFolder, "新建分组").Clicked() {
			a.openGroupForm(0, "")
		}
		if sidebarIconButton(c, t, "side-new-connection", iconPlus, "新建连接").Clicked() {
			a.openConnectForm()
		}
	})
}

func (a *App) sidebarLocalRow(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	active := ts != nil && ts.local
	a.sidebarSectionLabel(c, "本机")
	ui.Column(c).Shrink(0).Padding(0, 8, 3, 8).Children(func() {
		row := ui.ButtonBase(c.Key("side-local-terminal")).FillWidth().Height(38).
			Padding(0, 8).Radius(7).Gap(8).AlignItems(ui.Center)
		if active {
			row.Background(colAccentSoft)
			row.Border(1, colAccentLine)
		} else if row.Hovered() {
			row.Background(colPanel2)
		}
		row.Children(func() {
			if active {
				ui.Box(c).Width(2).Height(18).Radius(1).Background(colCyan)
			}
			ui.Box(c).Size(25, 25).Radius(7).Background(colPanel2).
				Border(1, colLine).Center().Children(func() {
				iconEl(c, iconTerm, 13, colViolet)
			})
			ui.Column(c).Grow(1).Gap(0).Children(func() {
				ui.Text(c, "本地终端").FontSize(11.6).FontWeight(550).
					TextColor(colText).NoWrap()
				ui.Text(c, "PowerShell · 本机").FontSize(8.8).Font("Cascadia Code").
					TextColor(colMuted).NoWrap()
			})
			ui.Text(c, "LOCAL").FontSize(8).Font("Cascadia Code").TextColor(colMuted2)
		})
		if row.Clicked() {
			a.OpenLocalTerminal()
		}
	})
}

func (a *App) sidebarSectionLabel(c *ui.Context, label string) {
	ui.Row(c).Shrink(0).Padding(10, 8, 4, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(8.8).FontWeight(650).LetterSpacing(1.1).
			TextColor(colMuted2).NoWrap()
		ui.Box(c).Height(1).Grow(1).Background(colLine)
	})
}

func sidebarIconButton(c *ui.Context, t *ui.Theme, key string, icon *ui.SVG, tip string) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(26, 26).Radius(7).Center().Tooltip(tip).Label(tip)
	b.Background(colPanel).Border(1, colLine)
	color := colText2
	if b.Hovered() {
		b.Background(colAccentSoft)
		b.BorderColor(colAccentLine)
		color = colCyan
	}
	b.Children(func() {
		iconEl(c, icon, 14, color)
	})
	return b
}

func (a *App) sidebarConnectionsEmpty(c *ui.Context) {
	ui.Column(c).Padding(18, 12, 20, 12).Center().Gap(8).Children(func() {
		ui.Box(c).Size(44, 44).Radius(10).
			Background(colPanel2).Border(1, colLine).Center().Children(func() {
			iconEl(c, iconConnections, 19, colMuted2)
		})
		ui.Text(c, "还没有保存的连接").FontSize(fsBody).FontWeight(600).TextColor(colText)
		ui.Text(c, "点右上角 + 新建连接，也可以先建一个分组").FontSize(10).TextColor(colMuted2).
			TextAlign(ui.Center)
		if primaryAction(c, "side-empty-new", "新建连接", iconPlus).Clicked() {
			a.openConnectForm()
		}
	})
}

func (a *App) sidebarConnectionsFooter(c *ui.Context) {
	ui.Row(c).Shrink(0).Padding(9, 12).Gap(7).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		iconEl(c, iconShield, 12, colGreen)
		ui.Text(c, "凭据加密 · 系统钥匙串").FontSize(8.6).TextColor(colMuted2).NoWrap()
		ui.Text(c, "").Grow(1)
		ui.Text(c, fmt.Sprintf("%d 分组 · %d 连接", len(a.groups), len(a.profiles))).FontSize(8.6).
			Font("Cascadia Code").TextColor(colMuted2).NoWrap()
	})
}

func (a *App) sidebarOpenSessions(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(colLine).
		Padding(0, 0, 7, 0).Children(func() {
		a.sidebarHead(c, t, "side-open", "打开的会话", len(a.terms), func() {
			a.openConnectForm()
		})
		ui.Column(c).Padding(0, 7).Children(func() {
			ui.Scroll(c).MaxHeight(220).Gap(1).Children(func() {
				if len(a.terms) == 0 {
					ui.Text(c, "还没有打开的会话").FontSize(10).TextColor(colMuted2).Padding(6, 6)
					return
				}
				for i, ts := range a.terms {
					a.sidebarTermRow(c, t, i, ts)
				}
			})
		})
	})
}

func (a *App) sidebarTermRow(c *ui.Context, t *ui.Theme, i int, ts *termSession) {
	active := i == a.activeTerm && a.nav == navWorkspace
	row := ui.Row(c).FillWidth().Height(46).Gap(2).AlignItems(ui.Center)
	row.Children(func() {
		main := ui.ButtonBase(c.Key(fmt.Sprintf("vtab-%d", ts.ID))).Grow(1).Height(44).
			Padding(0, 6).Radius(6).Gap(7).AlignItems(ui.Center)
		if active {
			main.Background(colAccentSoft)
			main.Border(1, colAccentLine)
		} else if main.Hovered() {
			main.Background(t.SurfaceHover)
		}
		main.Children(func() {
			dot := colGreen
			if ts.local {
				dot = colViolet
			}
			if ts.exited {
				dot = colRed
			}
			idxColor := colMuted2
			nameColor := colText
			subColor := colMuted
			if active {
				idxColor = colCyan.Alpha(0.7)
				nameColor = colText
				subColor = colCyan
			}
			ui.Text(c, fmt.Sprintf("%02d", i+1)).FontSize(8.5).Font("Cascadia Code").
				TextColor(idxColor).NoWrap()
			statusDot(c, dot, 6)
			ui.Column(c).Grow(1).Gap(1).Children(func() {
				ui.Text(c, shortLabel(ts.Title, 20)).FontSize(11).
					FontWeight(condWeight(active, 600, 500)).TextColor(nameColor).NoWrap()
				sub := "本地终端"
				if !ts.local {
					sub = ts.profile.Username + "@" + ts.profile.Host
				}
				if ts.exited {
					sub += " · 已断开"
				}
				ui.Text(c, shortLabel(sub, 26)).FontSize(8.5).Font("Cascadia Code").
					TextColor(subColor).NoWrap()
			})
		})
		if main.Clicked() {
			a.activateTerm(i)
		}

		closeBtn := ui.ButtonBase(c.Key(fmt.Sprintf("vclose-%d", ts.ID))).Size(18, 18).
			Center().Radius(4)
		if closeBtn.Hovered() {
			closeBtn.Background(colLine2)
		}
		color := colMuted
		closeBtn.Children(func() {
			ui.Text(c, "×").FontSize(12).TextColor(color)
		})
		if closeBtn.Clicked() {
			a.closeTerm(i)
		}
	})
}

func (a *App) sidebarConnections(c *ui.Context, t *ui.Theme) {
	a.sidebarHead(c, t, "side-conn", "筛选", -1, nil)
	counts := map[string]int{"": len(a.profiles)}
	for _, p := range a.profiles {
		counts[authGroup(p.AuthType)]++
	}
	labels := []struct {
		key   string
		label string
	}{
		{"", "全部连接"},
		{"password", "密码登录"},
		{"key", "私钥登录"},
		{"agent", "Agent"},
	}
	ui.Column(c).Grow(1).Padding(0, 7, 6, 7).Children(func() {
		ui.Scroll(c).Grow(1).Gap(1).Children(func() {
			for _, item := range labels {
				item := item
				a.sidebarFilterRow(c, t, "auth-"+item.key, item.label, counts[item.key], a.authFilter == item.key, func() {
					a.authFilter = item.key
					a.connPageSel = 0
				})
			}
		})
	})
	a.sidebarNote(c, "密码保存在系统钥匙串。首次连接会核对主机密钥，密钥变化会阻断连接。")
}

func (a *App) sidebarFiles(c *ui.Context, t *ui.Theme) {
	a.sidebarHead(c, t, "side-files", "文件主机", len(a.terms), func() {
		a.openConnectForm()
	})
	ui.Column(c).Grow(1).Padding(0, 7, 6, 7).Children(func() {
		ui.Scroll(c).Grow(1).Gap(1).Children(func() {
			if len(a.terms) == 0 {
				ui.Column(c).Padding(20, 10).Center().Gap(6).Children(func() {
					ui.Text(c, "还没有打开的会话").FontSize(10.5).TextColor(colMuted2)
					if primaryAction(c, "side-files-new", "新建连接", iconPlus).Clicked() {
						a.openConnectForm()
					}
				})
				return
			}
			for i, ts := range a.terms {
				i, ts := i, ts
				active := i == a.activeTerm
				row := ui.ButtonBase(c.Key(fmt.Sprintf("files-host-%d", ts.ID))).
					FillWidth().Height(44).Padding(0, 8).Radius(5).Gap(8).AlignItems(ui.Center)
				if active {
					row.Background(colPanel2).Border(1, colLine2)
				} else if row.Hovered() {
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					statusDot(c, condColor(ts.local, colViolet, condColor(ts.exited, colRed, colGreen)), 6)
					ui.Column(c).Grow(1).Gap(1).Children(func() {
						ui.Text(c, shortLabel(ts.Title, 21)).FontSize(11).FontWeight(condWeight(active, 600, 500)).
							TextColor(colText).NoWrap()
						sub := "本地终端 · 不支持 SFTP"
						if !ts.local {
							sub = ts.profile.Username + "@" + ts.profile.Host + " · SFTP"
						}
						ui.Text(c, shortLabel(sub, 30)).FontSize(8.5).Font("Cascadia Code").
							TextColor(colMuted2).NoWrap()
					})
				})
				if row.Clicked() {
					a.activeTerm = i
					a.openFiles()
				}
			}
		})
	})
	a.sidebarNote(c, "拖拽文件到远端目录即可上传；同名文件会询问覆盖、跳过或重命名。")
}

func (a *App) sidebarSettings(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(44).Shrink(0).Padding(0, 12).Gap(7).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "设置").FontSize(11.5).FontWeight(600).TextColor(colMuted)
		ui.Text(c, "").Grow(1)
		ui.Text(c, "本地配置").FontSize(9).Font("Cascadia Code").TextColor(colMuted2)
	})
	ui.Column(c).Grow(1).Padding(0, 7).Children(func() {
		ui.Scroll(c).Grow(1).Gap(1).Children(func() {
			items := []struct {
				label string
				icon  *ui.SVG
			}{
				{"外观", iconSun},
				{"终端", iconTerm},
				{"连接", iconLink},
				{"关于 Xu", iconHelp},
			}
			for i, item := range items {
				i, item := i, item
				active := a.settingsSection == i
				row := ui.ButtonBase(c.Key(fmt.Sprintf("set-side-%d", i))).FillWidth().Height(34).
					Padding(0, 9).Radius(5).Gap(8).AlignItems(ui.Center)
				if active {
					row.Background(colPanel2).Border(1, colLine)
				} else if row.Hovered() {
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					iconColor := colMuted2
					textColor := colMuted
					if active {
						iconColor = colCyan
						textColor = colText
					}
					iconEl(c, item.icon, 14, iconColor)
					ui.Text(c, item.label).FontSize(11).TextColor(textColor).NoWrap()
				})
				if row.Clicked() {
					a.settingsSection = i
				}
			}
		})
	})
	a.sidebarNote(c, "配置写入 ~/.xu/settings.db。主题和标签方向也可以从工具栏快速切换。")
}

func (a *App) sidebarFilterRow(c *ui.Context, t *ui.Theme, key, label string, count int, active bool, onClick func()) {
	row := ui.ButtonBase(c.Key("side-filter-"+key)).FillWidth().Height(32).
		Padding(0, 8).Radius(5).Gap(8).AlignItems(ui.Center)
	if active {
		row.Background(colPanel2).Border(1, colLine)
	} else if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		iconColor := colMuted2
		textColor := colMuted
		if active {
			iconColor = colCyan
			textColor = colText
		}
		iconEl(c, iconFolder, 13, iconColor)
		ui.Text(c, label).FontSize(11).TextColor(textColor).NoWrap()
		ui.Text(c, "").Grow(1)
		ui.Text(c, fmt.Sprintf("%d", count)).FontSize(9).Font("Cascadia Code").
			TextColor(colMuted2).NoWrap()
	})
	if row.Clicked() {
		onClick()
	}
}

func (a *App) sidebarNote(c *ui.Context, text string) {
	ui.Column(c).Shrink(0).Padding(10, 12).BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		ui.Text(c, text).FontSize(9).TextColor(colMuted2).LineHeight(1.55)
	})
}

func (a *App) tabLayoutToggle(c *ui.Context, t *ui.Theme, key string) {
	ui.Row(c).Height(26).Radius(5).Border(1, colLine).Background(colPanel2).
		Padding(2, 2).Gap(1).Children(func() {
		bH := ui.ButtonBase(c.Key(key+"-h")).Size(26, 22).Radius(4).Center().Tooltip("横向标签")
		if !a.settings.TabsVertical {
			bH.Background(colPanel)
		}
		bH.Children(func() {
			iconEl(c, iconLayoutH, 13, condColor(!a.settings.TabsVertical, colText, colMuted2))
		})
		if bH.Clicked() {
			a.setTabsVertical(false)
		}

		bV := ui.ButtonBase(c.Key(key+"-v")).Size(26, 22).Radius(4).Center().Tooltip("竖向标签")
		if a.settings.TabsVertical {
			bV.Background(colPanel)
		}
		bV.Children(func() {
			iconEl(c, iconLayoutV, 13, condColor(a.settings.TabsVertical, colText, colMuted2))
		})
		if bV.Clicked() {
			a.setTabsVertical(true)
		}
	})
}

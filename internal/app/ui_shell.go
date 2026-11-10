package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"xu/internal/monitorx"
)

func (a *App) openConnectForm() {
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	if a.connecting {
		a.status = "正在连接，请稍候"
		return
	}
	if a.finding {
		a.closeFind()
	}
	a.resetForm()
	a.saveProfile = true
	a.formOpen = true
	a.managerOpen = false
	a.pickerOpen = false
	a.directConnect = false
	a.batchOpen = false
	a.settingsOpen = false
	a.connectErr = ""
}

func (a *App) openConnectionManager() {
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	if a.finding {
		a.closeFind()
	}
	a.formOpen = false
	a.pickerOpen = false
	a.settingsOpen = false
	a.searchOpen = false
	a.managerOpen = true
	a.managerErr = ""
	if a.managerSel == 0 && len(a.profiles) > 0 {
		a.managerSel = a.profiles[0].ID
	}
}

func (a *App) shellMain(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).FillHeight().Background(colPanelDeep).Children(func() {
		a.tabStrip(c, t)
		switch {
		case a.quickTabActive():
			a.quickConnectView(c, t)
		case a.editorOpen:
			a.editorView(c, t)
		case len(a.terms) == 0:
			a.shellWelcome(c, t)
		default:
			termPane := func() {
				ui.Column(c).FillHeight().Background(colTermBg).Children(func() {
					a.terminal(c)
				})
			}
			if a.bottomFiles {
				// The divider between the terminal and the file dock can be
				// dragged up and down; double-click resets it to 64%.
				ui.SplitVerticalPercent(c, &a.splitPercent, termPane, func() {
					a.dockShellExpanded(c, t)
				}).Grow(1).SplitReset(64)
			} else {
				termPane()
				a.dockShell(c, t)
			}
		}
	})
}

func (a *App) sessionAction(c *ui.Context, t *ui.Theme, key string, icon *ui.SVG, tip string, active, danger bool) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(26, 26).Radius(6).Center().Tooltip(tip).Label(tip)
	if active {
		b.Background(colAccentSoft)
		b.Border(1, colAccentLine)
	} else if b.Hovered() {
		if danger {
			b.Background(colRed.Alpha(0.1))
		} else {
			b.Background(colPanel2)
		}
	}
	color := colMuted
	if active {
		color = colCyan
	}
	if danger && b.Hovered() {
		color = colRed
	}
	b.Children(func() {
		iconEl(c, icon, 13, color)
	})
	return b
}

func condColor(cond bool, yes, no ui.Color) ui.Color {
	if cond {
		return yes
	}
	return no
}

func condWeight(cond bool, yes, no int) int {
	if cond {
		return yes
	}
	return no
}

func (a *App) clearTerminal(ts *termSession) {
	if ts == nil || ts.Term == nil {
		return
	}
	ts.Term.ClearScreen()
}

func (a *App) shellStatusBar(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()

	dot := colMuted2
	label := "未连接"
	switch {
	case a.quickTabActive():
		label = "快速连接 · 选择服务器打开新会话"
		dot = colCyan
	case ts != nil:
		dot = colGreen
		if ts.exited {
			dot = colRed
		}
		if ts.local {
			label = "本地终端 · " + shortLabel(ts.Title, 28)
		} else {
			label = "已连接 · " + ts.profile.Username + "@" + ts.profile.Host
		}
	case len(a.terms) == 0:
		label = "未连接"
	}

	var s *monitorx.Sample
	if ts != nil && !ts.local {
		if ms := a.monitors[ts.ID]; ms != nil && len(ms.samples) > 0 {
			latest := ms.samples[len(ms.samples)-1]
			s = &latest
		}
	}

	ui.Row(c).Height(27).Shrink(0).Padding(0, 12).Gap(9).AlignItems(ui.Center).
		Background(colPanelTop).BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		statusDot(c, dot, 5)
		ui.Text(c, shortLabel(label, 42)).FontSize(8.8).TextColor(colMuted).NoWrap()
		if ts != nil && !a.quickTabActive() {
			path := terminalDir(ts)
			if b := a.browsers[ts.ID]; b != nil {
				if ts.local {
					path = b.localDir
				} else if b.remoteDir != "" {
					path = b.remoteDir
				}
			}
			a.statusSep(c)
			ui.Text(c, shortLabel(path, 30)).FontSize(8.4).Font("Cascadia Code").
				TextColor(colMuted2).NoWrap()
		}
		a.statusSep(c)

		if ts != nil && !ts.local {
			if s != nil {
				ui.Text(c, "CPU").FontSize(8.8).TextColor(colMuted2)
				ui.Text(c, fmt.Sprintf("%.0f%%", s.CPU)).FontSize(8.2).Font("Cascadia Code").TextColor(colMuted)
				ui.Text(c, "内存").FontSize(8.8).TextColor(colMuted2)
				ui.Text(c, fmt.Sprintf("%s / %s", humanSize(int64(s.MemUsed)), humanSize(int64(s.MemTotal)))).
					FontSize(8.2).Font("Cascadia Code").TextColor(colMuted)
				ui.Text(c, "↓ "+humanSize(int64(s.RxRate))+"/s").FontSize(8.2).Font("Cascadia Code").TextColor(colGreen)
				ui.Text(c, "↑ "+humanSize(int64(s.TxRate))+"/s").FontSize(8.2).Font("Cascadia Code").TextColor(colCyan)
			} else {
				ui.Text(c, "CPU —    内存 —    ↓ —    ↑ —").FontSize(8.8).TextColor(colMuted2)
			}
		}

		if a.status != "" {
			a.statusSep(c)
			ui.Text(c, shortLabel(a.status, 70)).FontSize(8.8).TextColor(colMuted2).NoWrap()
		}

		ui.Text(c, "").Grow(1)
		ui.Text(c, fmt.Sprintf("%d 个标签页", len(a.tabs))).FontSize(8.2).
			Font("Cascadia Code").TextColor(colMuted2)
		a.statusSep(c)
		ui.Text(c, "UTF-8").FontSize(8.2).Font("Cascadia Code").TextColor(colMuted2)
		a.statusSep(c)
		ui.Text(c, "Xu 0.1.0").FontSize(8.2).Font("Cascadia Code").TextColor(colMuted2)
	})
}

func (a *App) statusSep(c *ui.Context) {
	ui.Box(c).Width(1).Height(11).Background(colLine)
}

func (a *App) shellSection(c *ui.Context, t *ui.Theme, label string) {
	ui.Text(c, label).FontSize(9.5).FontWeight(650).LetterSpacing(0.8).
		TextColor(colMuted2).Padding(9, 3, 4, 3)
}

func (a *App) profileConnected(id int64) bool {
	return a.openSessionsForProfile(id) > 0
}

// openSessionsForProfile counts the live terminal tabs of one saved server,
// so the launcher can show that the same host is open more than once.
func (a *App) openSessionsForProfile(id int64) int {
	n := 0
	for _, ts := range a.terms {
		if !ts.local && !ts.exited && ts.profileID == id {
			n++
		}
	}
	return n
}

func shortLabel(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func (a *App) shellWelcome(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Fill().Center().Padding(32).Gap(9).Children(func() {
		ui.Box(c).Size(46, 46).Radius(14).
			Background(colCyan.Alpha(0.08)).Border(1, colCyan.Alpha(0.22)).Center().Children(func() {
			iconEl(c, iconServer, 22, colCyan)
		})
		ui.Text(c, "还没有打开的会话").FontSize(15).FontWeight(650)
		ui.Text(c, "点顶部 + 打开快速连接选择服务器，或直接打开本地终端").FontSize(11).TextColor(colMuted)
		ui.Row(c).Gap(8).Margin(6, 0, 0, 0).Children(func() {
			if primaryAction(c, "welcome-quick", "快速连接", iconPlus).Clicked() {
				a.openQuickTab()
				a.status = "从快速连接中选择服务器"
			}
			if ghostAction(c, t, "welcome-new", "新建连接", nil).Clicked() {
				a.openConnectForm()
			}
			if ghostAction(c, t, "welcome-local", "本地终端", iconTerm).Clicked() {
				a.OpenLocalTerminal()
			}
		})
	})
}

func (a *App) connectModal(c *ui.Context, t *ui.Theme) {
	editing := a.profileID != 0
	title := "新建 SSH 连接"
	sub := "填写服务器信息，保存后会出现在快速连接与连接管理器"
	if editing {
		title = "编辑连接"
		sub = "修改连接信息；保存后列表会立即更新"
	}

	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}

	ui.DialogBase(c, &a.formOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(fitDialogWidth(c, 600)).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Row(c).Height(64).Shrink(0).Padding(0, 18).Gap(10).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
				ui.Box(c).Size(30, 30).Radius(8).
					Background(colCyan.Alpha(0.08)).Border(1, colCyan.Alpha(0.22)).Center().Children(func() {
					iconEl(c, iconServer, 15, colCyan)
				})
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, title).FontSize(14.5).FontWeight(650)
					ui.Text(c, sub).FontSize(9.5).TextColor(colMuted).NoWrap()
				})
				if iconButton(c, t, "connect-close", iconClose, "关闭").Clicked() {
					a.formOpen = false
				}
			})

			bodyMax := float32(520)
			if _, ch := c.Size(); ch < 720 {
				bodyMax = ch - 210
			}
			ui.Scroll(c).Shrink(0).MaxHeight(bodyMax).Children(func() {
				ui.Column(c).FillWidth().Padding(16, 18).Gap(10).Children(func() {
					fieldColumn(c, "名称", func() {
						ui.TextInput(c, &a.profileName).Label("名称").
							Placeholder("可选，留空则用 用户名@主机")
					})
					ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
						fieldColumn(c, "主机", func() {
							ui.TextInput(c, &a.host).Label("主机").
								Placeholder("192.168.1.10").AutoFocus()
						}).Grow(1)
						fieldColumn(c, "端口", func() {
							ui.TextInput(c, &a.port).Label("端口").Placeholder("22")
						}).Width(96)
					})
					ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
						fieldColumn(c, "用户名", func() {
							ui.TextInput(c, &a.username).Label("用户名").Placeholder("root")
						}).Grow(1)
						fieldColumn(c, "分组", func() {
							ui.Select(c, &a.groupChoice, a.groupOptions()).FillWidth()
						}).Grow(1)
					})
					passwordHint := "留空则尝试 ~/.xu/keys 私钥"
					if editing {
						passwordHint = "留空则使用已保存密码或私钥"
					}
					fieldColumn(c, "密码", func() {
						ui.TextInput(c, &a.password).Label("密码（可留空）").
							Placeholder(passwordHint).Password()
					})

					if editing {
						ui.Text(c, "修改会立即保存到连接列表").
							FontSize(9.5).TextColor(colMuted2).NoWrap()
					} else {
						ui.Checkbox(c, &a.saveProfile, "保存到连接列表")
					}

					ui.Box(c).FillWidth().Padding(9, 10).Radius(6).Background(colPanelTop).
						Border(1, colLine).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
							iconEl(c, iconShield, 12, colGreen)
							ui.Text(c, "首次连接会核对主机密钥指纹；密钥变化会立即阻断连接。").
								FontSize(9).TextColor(colMuted2).LineHeight(1.45)
						})
					})

					if a.connectErr != "" {
						ui.Box(c).FillWidth().Padding(8, 10).Radius(5).
							Background(colRed.Alpha(0.07)).Border(1, colRed.Alpha(0.18)).Children(func() {
							ui.Text(c, a.connectErr).FontSize(10).TextColor(colRed).LineHeight(1.45)
						})
					}
				})
			})

			ui.Row(c).Height(54).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
				if editing {
					if dangerAction(c, "connect-delete", "删除", iconTrash).Clicked() {
						a.deleteProfile()
					}
				} else {
					iconEl(c, iconLink, 11, colMuted2)
					ui.Text(c, "SFTP · 监控 · 端口转发").FontSize(9).TextColor(colMuted2).NoWrap()
				}
				ui.Text(c, "").Grow(1)
				if ghostAction(c, t, "connect-cancel", "取消", nil).Clicked() {
					a.formOpen = false
				}
				if ghostAction(c, t, "connect-save", "保存", iconSave).Disabled(a.connecting).Clicked() {
					a.saveProfileForm()
				}
				label := "保存并连接"
				if editing {
					label = "连接"
				}
				if a.connecting {
					label = "连接中…"
				}
				if primaryAction(c, "connect-submit", label, iconLink).Disabled(a.connecting).Clicked() {
					a.directConnect = false
					a.connectingProfileID = 0
					a.connect()
				}
			})
		})
	})
}

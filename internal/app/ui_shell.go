package app

import (
	"fmt"

	"github.com/egoist/mygo"
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
	a.managerNotice = ""
	if a.managerSel == 0 && len(a.profiles) > 0 {
		a.managerSel = a.profiles[0].ID
	}
}

func (a *App) shellMain(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).FillHeight().Background(colPanelDeep).Children(func() {
		a.tabStrip(c, t)
		switch {
		case a.connectingTabActive():
			a.connectingTabView(c, t, a.activeTabRef())
		case a.quickTabActive():
			a.quickConnectView(c, t)
		case a.editorOpen:
			a.editorView(c, t)
		case len(a.terms) == 0:
			a.shellWelcome(c, t)
		default:
			local := a.activeTerminal() != nil && a.activeTerminal().local
			termPane := func() {
				ui.Column(c).FillHeight().Background(colTermBg).Children(func() {
					a.terminal(c)
				})
			}
			if !local && a.bottomFiles {
				// The divider between the terminal and the file dock can be
				// dragged up and down; double-click resets it to 64%.
				ui.SplitVerticalPercent(c, &a.splitPercent, termPane, func() {
					a.dockShellExpanded(c, t)
				}).Grow(1).SplitReset(64)
			} else {
				termPane()
				if !local {
					a.dockShell(c, t)
				}
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
		ui.Text(c, "Xu "+mygo.App.Version()).FontSize(8.2).Font("Cascadia Code").TextColor(colMuted2)
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
	w, ch := c.Size()
	wide := w-monitorWidth(c) >= 820
	minHeight := ch - 100
	if minHeight < 480 {
		minHeight = 480
	}

	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().MinHeight(minHeight).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Grow(1).MinHeight(28)
			ui.Column(c).FillWidth().MaxWidth(900).Gap(16).Children(func() {
				a.welcomeHero(c, t, wide)
				a.welcomeFeatures(c, t, wide)
				a.welcomeRecent(c, t, wide)
				a.welcomeShortcuts(c, wide)
			})
			ui.Box(c).Grow(1).MinHeight(28)
		})
	})
}

func (a *App) welcomeHero(c *ui.Context, t *ui.Theme, wide bool) {
	ui.Column(c).FillWidth().Padding(26).Radius(16).
		Gradient(colPanelTop, colPanelDeep, 135).Border(1, colLine).
		Shadow(0, 18, 46, 0, ui.Hex("#000000").Alpha(0.16)).Children(func() {
		if wide {
			ui.Row(c).FillWidth().Gap(30).AlignItems(ui.Center).Children(func() {
				a.welcomeIntro(c, t, true)
				a.welcomeTerminalPreview(c, t, true)
			})
			return
		}
		ui.Column(c).FillWidth().Gap(22).Children(func() {
			a.welcomeIntro(c, t, false)
			a.welcomeTerminalPreview(c, t, false)
		})
	})
}

func (a *App) welcomeIntro(c *ui.Context, t *ui.Theme, grow bool) {
	intro := ui.Column(c).Gap(14)
	if grow {
		intro = intro.Grow(1)
	} else {
		intro = intro.FillWidth()
	}
	intro.Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			statusDot(c, colAmber, 6)
			ui.Text(c, "还没有打开的会话").FontSize(9.2).FontWeight(600).TextColor(colAmber)
			ui.Text(c, "·").FontSize(9).TextColor(colMuted2)
			ui.Text(c, "XU SSH WORKBENCH").FontSize(8.4).FontWeight(650).
				LetterSpacing(1.3).TextColor(colMuted2)
		})
		ui.Text(c, "连接服务器，\n开始今天的工作。").FontSize(26).FontWeight(650).
			TextColor(colText).LineHeight(1.18)
		ui.Text(c, "多标签终端、SFTP 文件、实时监控和端口转发，都在一个原生窗口里。").
			FontSize(11.2).TextColor(colMuted).LineHeight(1.65).MaxWidth(500)
		ui.Row(c).Gap(9).Margin(2, 0, 0, 0).Children(func() {
			if primaryAction(c, "welcome-quick", "快速连接", iconPlus).Clicked() {
				a.openQuickTab()
				a.status = "从快速连接中选择服务器"
			}
			if ghostAction(c, t, "welcome-manager", "连接管理", iconManager).Clicked() {
				a.openConnectionManager()
			}
			if ghostAction(c, t, "welcome-local", "本地终端", iconTerm).Clicked() {
				a.OpenLocalTerminal()
			}
		})
		ui.Row(c).Gap(6).Margin(1, 0, 0, 0).Children(func() {
			miniTag(c, "原生 UI", colCyan)
			miniTag(c, "Ghostty VT", colViolet)
			miniTag(c, "Ed25519 更新", colGreen)
		})
	})
}

func (a *App) welcomeTerminalPreview(c *ui.Context, t *ui.Theme, fixedWidth bool) {
	card := ui.Column(c).Padding(14).Radius(12).Background(colTermBg).
		Border(1, colTermLine).Gap(9)
	if fixedWidth {
		card = card.Width(320)
	} else {
		card = card.FillWidth()
	}
	card.Children(func() {
		ui.Row(c).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
			statusDot(c, colRed, 7)
			statusDot(c, colAmber, 7)
			statusDot(c, colGreen, 7)
			ui.Text(c, "xu · terminal").Font("Cascadia Code").FontSize(8.4).
				TextColor(colMuted2).Margin(4, 0, 0, 0)
			ui.Text(c, "").Grow(1)
			miniTag(c, "示例", colMuted2)
		})
		hairline(c)
		ui.Column(c).Gap(4).Children(func() {
			welcomeTermLine(c, "$", "ssh root@prod-01", colText, colCyan)
			welcomeTermLine(c, "✓", "connected · 28 ms", colGreen, colGreen)
			welcomeTermLine(c, "›", "tail -f access.log", colMuted, colMuted2)
		})
		ui.Box(c).FillWidth().Height(1).Background(colTermLine)
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			welcomeMetric(c, "CPU", 0.12, "12%", colCyan)
			welcomeMetric(c, "MEM", 0.38, "38%", colAmber)
			welcomeMetric(c, "NET", 0.66, "↓ 1.2M", colGreen)
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			statusDot(c, colGreen, 5)
			ui.Text(c, "3 个会话 · 28 ms").FontSize(8.4).TextColor(colMuted2)
		})
	})
}

func welcomeTermLine(c *ui.Context, mark, text string, color, markColor ui.Color) {
	ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
		ui.Text(c, mark).Font("Cascadia Code").FontSize(9.4).TextColor(markColor)
		ui.Text(c, text).Font("Cascadia Code").FontSize(9.4).TextColor(color).NoWrap()
	})
}

func welcomeMetric(c *ui.Context, label string, value float64, text string, color ui.Color) {
	ui.Column(c).Grow(1).Gap(4).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).Font("Cascadia Code").FontSize(7.6).TextColor(colMuted2)
			ui.Text(c, "").Grow(1)
			ui.Text(c, text).Font("Cascadia Code").FontSize(8.2).TextColor(color)
		})
		ui.Progress(c, value).FillWidth().Height(3)
	})
}

func (a *App) welcomeFeatures(c *ui.Context, t *ui.Theme, wide bool) {
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "工作台能力").FontSize(9.5).FontWeight(650).LetterSpacing(0.8).
				TextColor(colMuted2)
			ui.Box(c).Height(1).Grow(1).Background(colLine)
		})
		if wide {
			ui.Row(c).FillWidth().Gap(10).Children(func() {
				a.welcomeFeature(c, iconTerm, "多标签终端", "SSH 与本地 Shell", colCyan)
				a.welcomeFeature(c, iconFolder, "SFTP 文件", "浏览、上传、编辑", colViolet)
				a.welcomeFeature(c, iconActivity, "实时监控", "CPU、内存、网络", colGreen)
				a.welcomeFeature(c, iconLink, "端口转发", "本地、远程、SOCKS5", colAmber)
			})
			return
		}
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			a.welcomeFeature(c, iconTerm, "多标签终端", "SSH 与本地 Shell", colCyan)
			a.welcomeFeature(c, iconFolder, "SFTP 文件", "浏览、上传、编辑", colViolet)
		})
		ui.Row(c).FillWidth().Gap(10).Children(func() {
			a.welcomeFeature(c, iconActivity, "实时监控", "CPU、内存、网络", colGreen)
			a.welcomeFeature(c, iconLink, "端口转发", "本地、远程、SOCKS5", colAmber)
		})
	})
}

func (a *App) welcomeFeature(c *ui.Context, icon *ui.SVG, title, sub string, color ui.Color) {
	ui.Column(c).Grow(1).Height(96).Padding(13).Radius(12).
		Background(colPanel.Alpha(0.72)).Border(1, colLine).Gap(7).Children(func() {
		ui.Box(c).Size(30, 30).Radius(8).Background(color.Alpha(0.09)).
			Border(1, color.Alpha(0.2)).Center().Children(func() {
			iconEl(c, icon, 14, color)
		})
		ui.Text(c, title).FontSize(11).FontWeight(600).TextColor(colText)
		ui.Text(c, sub).FontSize(9).TextColor(colMuted).NoWrap()
	})
}

func (a *App) welcomeRecent(c *ui.Context, t *ui.Theme, wide bool) {
	if len(a.profiles) == 0 {
		return
	}
	slots := 3
	if !wide {
		slots = 2
	}
	limit := slots
	if limit > len(a.profiles) {
		limit = len(a.profiles)
	}

	ui.Column(c).FillWidth().Gap(9).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			iconEl(c, iconHistory, 12, colMuted2)
			ui.Text(c, "最近连接").FontSize(9.5).FontWeight(650).LetterSpacing(0.8).
				TextColor(colMuted2)
			miniTag(c, fmt.Sprintf("%d", len(a.profiles)), colMuted2)
			ui.Box(c).Height(1).Grow(1).Background(colLine)
			if len(a.profiles) > limit {
				if quietAction(c, t, "welcome-all-connections", "全部", iconChevronRight, false).Clicked() {
					a.openConnectionManager()
				}
			}
		})
		ui.Row(c).FillWidth().Height(58).Gap(10).AlignItems(ui.Start).Children(func() {
			for i := 0; i < limit; i++ {
				p := a.profiles[i]
				cell := ui.Box(c).Grow(1).Height(58)
				cell.Children(func() { a.quickCard(c, t, p) })
			}
			for i := limit; i < slots; i++ {
				ui.Box(c).Grow(1).Height(58)
			}
		})
	})
}

func (a *App) welcomeShortcuts(c *ui.Context, wide bool) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		welcomeShortcut(c, "Ctrl K", "搜索")
		welcomeShortcut(c, "Ctrl Alt T", "新标签")
		welcomeShortcut(c, "Ctrl Shift O", "文件")
		if wide {
			welcomeShortcut(c, "Ctrl J", "底部面板")
		}
		ui.Text(c, "").Grow(1)
		if wide {
			ui.Text(c, "连接成功后可直接保存会话").FontSize(8.8).TextColor(colMuted2)
		}
	})
}

func welcomeShortcut(c *ui.Context, key, label string) {
	ui.Row(c).Height(24).Padding(0, 8).Radius(6).Background(colPanel2.Alpha(0.55)).
		Border(1, colLine).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, key).Font("Cascadia Code").FontSize(8.2).TextColor(colText2)
		ui.Text(c, label).FontSize(9).TextColor(colMuted)
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
					a.connectingProfileID = 0
					a.connect()
				}
			})
		})
	})
}

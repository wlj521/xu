package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"xu/internal/monitorx"
	"xu/internal/store"
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
	a.settingsOpen = false
	a.formOpen = false
	a.pickerOpen = false
	a.status = "连接管理已合并到左侧连接树：点 + 新建，点铅笔编辑"
}

func (a *App) shellMain(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).FillHeight().Background(colPanelDeep).Children(func() {
		if a.editorOpen {
			a.sessionBar(c, t)
			a.editorView(c, t)
			a.shellStatusBar(c, t)
			return
		}
		if len(a.terms) == 0 {
			a.shellWelcome(c, t)
			a.shellStatusBar(c, t)
			return
		}

		a.sessionBar(c, t)
		ui.Column(c).Grow(1).FillHeight().Background(colTermBg).Children(func() {
			a.terminal(c)
		})
		a.dockShell(c, t)
		a.shellStatusBar(c, t)
	})
}

func (a *App) sessionBar(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	ui.Row(c).Height(39).Shrink(0).Padding(0, 13).Gap(9).AlignItems(ui.Center).
		Background(colPanel).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		if ts == nil {
			statusDot(c, colMuted2, 6)
			ui.Text(c, "未连接").FontSize(11).FontWeight(600).TextColor(colMuted)
		} else {
			color := colGreen
			if ts.local {
				color = colViolet
			}
			if ts.exited {
				color = colRed
			}
			ui.Box(c).Size(12, 12).Radius(6).Background(color.Alpha(0.13)).Center().Children(func() {
				statusDot(c, color, 6)
			})
			name := ts.Title
			if ts.local {
				name = "本地终端"
			}
			ui.Text(c, shortLabel(name, 30)).FontSize(11).FontWeight(600).
				Font("Cascadia Code").TextColor(colText).NoWrap()

			path := terminalDir(ts)
			if b := a.browsers[ts.ID]; b != nil {
				if ts.local {
					path = b.localDir
				} else if b.remoteDir != "" {
					path = b.remoteDir
				}
			}
			ui.Text(c, ":").FontSize(10).Font("Cascadia Code").TextColor(colMuted2)
			ui.Text(c, shortLabel(path, 28)).FontSize(10.5).Font("Cascadia Code").
				TextColor(colText2).NoWrap()

			if !ts.local {
				if ms := a.monitors[ts.ID]; ms != nil && len(ms.samples) > 0 {
					if osName := osTag(ms.samples[len(ms.samples)-1].OS); osName != "" {
						a.sessionChip(c, osName)
					}
				}
				a.sessionChip(c, fmt.Sprintf("SSH · %d", ts.profile.Port))
			} else {
				a.sessionChip(c, "LOCAL")
			}
			if ts.exited {
				a.sessionChipColored(c, "已断开", colRed)
			}
		}

		ui.Text(c, "").Grow(1)

		search := a.sessionAction(c, t, "sb-search", iconSearch, "查找（Ctrl Shift F）", a.finding, false)
		if search.Clicked() {
			if a.finding {
				a.closeFind()
			} else {
				a.openFind()
			}
		}
		if a.sessionAction(c, t, "sb-clear", iconEraser, "清屏", false, false).Clicked() {
			a.clearTerminal(ts)
		}
		ui.Box(c).Width(1).Height(14).Background(colLine).MarginX(4)
		if ts != nil {
			if a.sessionAction(c, t, "sb-close", iconPower, "断开连接", false, true).Clicked() {
				a.closeTerm(a.activeTerm)
			}
		}
	})
}

func (a *App) sessionChip(c *ui.Context, label string) {
	a.sessionChipColored(c, label, colText2)
}

func (a *App) sessionChipColored(c *ui.Context, label string, color ui.Color) {
	ui.Box(c).Height(19).Padding(0, 6).Radius(5).Background(colSurfaceHover).
		Border(1, colLine).Center().Children(func() {
		ui.Text(c, label).FontSize(8.8).TextColor(color).NoWrap()
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

func (a *App) shellBody(c *ui.Context, t *ui.Theme) {
	switch a.nav {
	case navConnections:
		a.connectionsPage(c, t)
		return
	case navFiles:
		a.filesView(c, t)
		return
	case navSettings:
		a.settingsView(c, t)
		return
	}

	switch {
	case a.batchOpen:
		a.batchView(c, t)
		return
	case a.settingsOpen:
		a.settingsView(c, t)
		return
	case len(a.terms) == 0:
		a.shellWelcome(c, t)
		return
	case a.editorOpen:
		a.editorView(c, t)
		return
	case a.monitorOpen:
		a.monitorView(c, t)
		return
	case a.forwardOpen:
		a.forwardView(c, t)
		return
	}

	ts := a.activeTerminal()
	if a.filesOpen {
		a.filesView(c, t)
		return
	}

	if ts != nil && !ts.local && a.bottomFiles {
		b, err := a.ensureFileBrowser(ts)
		if err != nil {
			a.bottomFiles = false
			a.status = "打开 SFTP 失败: " + err.Error()
			a.terminalSection(c, t)
			return
		}
		ui.SplitVerticalPercent(c, &a.splitPercent, func() {
			a.terminalSection(c, t)
		}, func() {
			a.bottomFilesView(c, t, ts, b)
		}).Grow(1)
		return
	}

	a.terminalSection(c, t)
}

func (a *App) shellTabbar(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(44).Shrink(0).Padding(0, 10).Gap(5).AlignItems(ui.Center).
		Background(colPanel).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		if !a.settings.TabsVertical {
			for i, ts := range a.terms {
				i, ts := i, ts
				active := i == a.activeTerm
				dot := colGreen
				if ts.local {
					dot = colViolet
				}
				if ts.exited {
					dot = colRed
				}

				var closed bool
				tab := ui.ButtonBase(c.Key(fmt.Sprintf("tab-%d", ts.ID))).Height(30).Padding(0, 8).
					Gap(7).Radius(5).AlignItems(ui.Center)
				if active {
					tab.Background(colAccentSoft).Border(1, colAccentLine)
				} else if tab.Hovered() {
					tab.Background(t.SurfaceHover)
				}
				tab.Children(func() {
					idxColor := colMuted2
					nameColor := colMuted
					closeColor := colMuted
					if active {
						idxColor = colCyan.Alpha(0.7)
						nameColor = colText
						closeColor = colCyan
					}
					ui.Text(c, fmt.Sprintf("%02d", i+1)).FontSize(8.5).Font("Cascadia Code").
						TextColor(idxColor).NoWrap()
					statusDot(c, dot, 5)
					ui.Text(c, shortLabel(ts.Title, 18)).FontSize(11).
						FontWeight(condWeight(active, 600, 500)).TextColor(nameColor).NoWrap()
					closeBtn := ui.ButtonBase(c.Key(fmt.Sprintf("close-%d", ts.ID))).Size(17, 17).Radius(4).Center()
					if closeBtn.Hovered() {
						closeBtn.Background(colLine2)
					}
					closeBtn.Children(func() {
						ui.Text(c, "×").FontSize(11.5).TextColor(closeColor)
					})
					if closeBtn.Clicked() {
						closed = true
						a.closeTerm(i)
					}
				})
				if tab.Clicked() && !closed {
					a.activateTerm(i)
				}
			}

			add := ui.ButtonBase(c.Key("tab-add")).Size(28, 28).Radius(5).Center().Tooltip("打开已保存的连接")
			if add.Hovered() || a.pickerOpen {
				add.Background(colCyan.Alpha(0.07))
			}
			add.Border(1, colLine)
			add.Children(func() {
				color := colMuted
				if add.Hovered() || a.pickerOpen {
					color = colCyan
				}
				iconEl(c, iconPlus, 13, color)
			})
			if add.Clicked() {
				a.pickerOpen = !a.pickerOpen
			}
			ui.Popover(c, add, &a.pickerOpen, func() {
				a.connectionPicker(c, t)
			})
		} else {
			iconEl(c, iconTerm, 13, colCyan)
			ui.Text(c, "工作区").FontSize(8.8).TextColor(colMuted2).NoWrap()
			if ts := a.activeTerminal(); ts != nil {
				ui.Text(c, "/").FontSize(8.8).TextColor(colMuted2)
				ui.Text(c, shortLabel(ts.Title, 30)).FontSize(10.5).Font("Cascadia Code").
					TextColor(colMuted).NoWrap()
			}
		}

		ui.Text(c, "").Grow(1)

		a.tabLayoutToggle(c, t, "tabs-layout")

		dock := iconButton(c, t, "tab-dock", iconFolder, "文件面板")
		if a.bottomFiles {
			dock.Background(colPanel2).Border(1, colLine)
		}
		if dock.Clicked() {
			a.toggleBottomFiles()
		}

		panel := iconButton(c, t, "tab-inspector", iconPanel, "连接信息")
		if a.sidebarVisible {
			panel.Background(colPanel2).Border(1, colLine)
		}
		if panel.Clicked() {
			a.sidebarVisible = !a.sidebarVisible
		}

		more := iconButton(c, t, "tab-more", iconMore, "更多")
		more.Menu(func(m *ui.Menu) {
			if m.Item("服务器监控").Chosen() {
				a.openMonitor()
			}
			if m.Item("端口转发").Chosen() {
				a.openForwards()
			}
			if m.Item("批量命令").Chosen() {
				a.openBatch()
			}
			m.Separator()
			if m.Item("本地终端").Chosen() {
				a.OpenLocalTerminal()
			}
		})
	})
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

func tabAction(c *ui.Context, t *ui.Theme, key, label string, icon *ui.SVG, active bool) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Height(30).Padding(0, 10).Radius(8).Gap(6)
	if active {
		b.Background(colPanel2).Border(1, colLine2)
	} else if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	color := t.TextMuted
	if active {
		color = colText
	}
	b.Children(func() {
		iconEl(c, icon, 14, color)
		ui.Text(c, label).FontSize(11).TextColor(color).NoWrap()
	})
	return b
}

func (a *App) terminalSection(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	ui.Column(c).Grow(1).Background(colTermBg).Children(func() {
		a.termHead(c, t, ts)
		ui.Column(c).Grow(1).Children(func() {
			a.terminal(c)
		})
		a.commandBar(c, t, ts)
	})
}

func (a *App) termHead(c *ui.Context, t *ui.Theme, ts *termSession) {
	ui.Row(c).Height(38).Shrink(0).Padding(0, 16).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colTermLine).Children(func() {
		host := "未连接"
		path := "~"
		hostIcon := iconServer
		hostColor := colMuted
		if ts != nil {
			if ts.local {
				host = "本机"
				hostIcon = iconTerm
				hostColor = colViolet
			} else {
				host = ts.profile.Username + "@" + ts.profile.Host
				hostColor = colGreen
			}
			if b := a.browsers[ts.ID]; b != nil && b.remoteDir != "" {
				path = b.remoteDir
			} else if ts.Term != nil {
				if dir := ts.Term.Dir(); dir != "" {
					path = dir
				}
			}
		}
		iconEl(c, hostIcon, 13, hostColor)
		ui.Text(c, shortLabel(host, 42)).FontSize(10.5).FontWeight(550).
			TextColor(colMuted).NoWrap()
		ui.Text(c, "/").FontSize(10.5).TextColor(colMuted2)
		ui.Text(c, shortLabel(path, 70)).FontSize(10.5).Font("Cascadia Code").
			TextColor(colCyan).NoWrap()
		if ts != nil && ts.exited {
			ui.Box(c).Height(20).Padding(0, 7).Radius(5).
				Background(colRed.Alpha(0.08)).Border(1, colRed.Alpha(0.22)).Center().Children(func() {
				ui.Text(c, "已断开").FontSize(9).TextColor(colRed).NoWrap()
			})
		}

		ui.Text(c, "").Grow(1)

		search := quietAction(c, t, "term-search", "搜索", iconSearch, a.finding)
		if search.Clicked() {
			if a.finding {
				a.closeFind()
			} else {
				a.openFind()
			}
		}
		if quietAction(c, t, "term-clear", "清屏", nil, false).Clicked() {
			a.clearTerminal(ts)
		}
		ui.Box(c).Height(22).Padding(0, 7).Radius(5).Background(colPanel2.Alpha(0.7)).Border(1, colLine).
			Center().Children(func() {
			ui.Text(c, "UTF-8").FontSize(8.5).TextColor(colMuted2).LetterSpacing(0.5)
		})
	})
}

func (a *App) clearTerminal(ts *termSession) {
	if ts == nil || ts.Term == nil {
		return
	}
	ts.Term.ClearScreen()
}

func (a *App) commandBar(c *ui.Context, t *ui.Theme, ts *termSession) {
	ui.Row(c).Height(46).Shrink(0).Padding(0, 14).Gap(9).AlignItems(ui.Center).
		Background(colPanel).BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		ui.Box(c).Width(2).Height(15).Radius(1).Background(colCyan.Alpha(0.55))
		ui.Text(c, "命令").FontSize(9).FontWeight(650).LetterSpacing(1).TextColor(colMuted2).NoWrap()

		input := ui.TextInput(c, &a.commandInput).Grow(1).Height(32).Font("Cascadia Code").
			FontSize(12).Placeholder("输入命令，回车执行…").
			Background(ui.Transparent).Border(0, ui.Transparent)
		if input.Submitted() {
			a.sendCommand(ts, a.commandInput)
		}

		history := quietAction(c, t, "cmd-history", "历史", iconHistory, false)
		history.Menu(func(m *ui.Menu) {
			if len(a.commandHistory) == 0 {
				m.Item("还没有命令历史").Disabled(true)
				return
			}
			for i := len(a.commandHistory) - 1; i >= 0 && i >= len(a.commandHistory)-12; i-- {
				cmd := a.commandHistory[i]
				if m.Item(shortLabel(cmd, 46)).Chosen() {
					a.commandInput = cmd
				}
			}
		})

		send := accentAction(c, "cmd-send", "执行", iconCommand)
		if send.Clicked() {
			a.sendCommand(ts, a.commandInput)
		}
	})
}

func (a *App) sendCommand(ts *termSession, cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" || ts == nil || ts.Term == nil || ts.exited {
		return
	}
	ts.Term.Send([]byte(cmd + "\r"))
	a.commandInput = ""
	kept := a.commandHistory[:0]
	for _, old := range a.commandHistory {
		if old != cmd {
			kept = append(kept, old)
		}
	}
	a.commandHistory = append(kept, cmd)
	if len(a.commandHistory) > 50 {
		a.commandHistory = a.commandHistory[len(a.commandHistory)-50:]
	}
}

func (a *App) shellStatusBar(c *ui.Context, t *ui.Theme) {
	switch a.nav {
	case navConnections:
		open := 0
		for _, ts := range a.terms {
			if !ts.local && !ts.exited {
				open++
			}
		}
		note := "凭据保存在系统钥匙串"
		if a.status != "" {
			note = a.status
		}
		a.statusBarSimple(c, t, colGreen, fmt.Sprintf("%d 个连接 · %d 个已打开", len(a.profiles), open), note)
		return
	case navSettings:
		note := "修改会在保存后写入 ~/.xu"
		if a.settingsStatus != "" {
			note = a.settingsStatus
		}
		a.statusBarSimple(c, t, colGreen, "设置", note)
		return
	case navFiles:
		ts := a.activeTerminal()
		label := "文件管理 · 未连接"
		dot := colMuted2
		if ts != nil {
			if ts.local {
				label = "文件管理 · 本地终端"
				dot = colAmber
			} else {
				label = "SFTP · " + ts.Title
				dot = colGreen
				if ts.exited {
					dot = colRed
				}
			}
		}
		note := "本地与远端双栏"
		if a.status != "" {
			note = a.status
		}
		a.statusBarSimple(c, t, dot, label, note)
		return
	}

	ts := a.activeTerminal()
	dot := colMuted2
	label := "未连接"
	if ts != nil {
		dot = colGreen
		if ts.exited {
			dot = colRed
		}
		if ts.local {
			label = "本地终端 · " + ts.Title
		} else {
			label = "已连接 · " + ts.profile.Username + "@" + ts.profile.Host
		}
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
		ui.Text(c, label).FontSize(8.8).TextColor(colMuted).NoWrap()
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
		ui.Text(c, "UTF-8").FontSize(8.2).Font("Cascadia Code").TextColor(colMuted2)
		a.statusSep(c)
		ui.Text(c, "Xu 0.1.0").FontSize(8.2).Font("Cascadia Code").TextColor(colMuted2)
	})
}

func (a *App) statusBarSimple(c *ui.Context, t *ui.Theme, dot ui.Color, label, note string) {
	ui.Row(c).Height(27).Shrink(0).Padding(0, 12).Gap(9).AlignItems(ui.Center).
		Background(colPanelTop).BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		statusDot(c, dot, 5)
		ui.Text(c, label).FontSize(8.8).TextColor(colMuted).NoWrap()
		if note != "" {
			a.statusSep(c)
			ui.Text(c, shortLabel(note, 80)).FontSize(8.8).TextColor(colMuted2).NoWrap()
		}
		ui.Text(c, "").Grow(1)
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
	for _, ts := range a.terms {
		if !ts.local && !ts.exited && ts.profileID == id {
			return true
		}
	}
	return false
}

func (a *App) profileRow(c *ui.Context, t *ui.Theme, p store.Profile, connected bool) {
	connecting := a.connecting && a.connectingProfileID == p.ID
	row := ui.Row(c).FillWidth().Height(38).Padding(0, 1).AlignItems(ui.Center)
	row.Children(func() {
		main := ui.ButtonBase(c.Key(fmt.Sprintf("profile-connect-%d", p.ID))).
			Grow(1).Height(36).Padding(0, 8).Radius(7).Gap(8).AlignItems(ui.Center)
		if connected {
			main.Background(colAccentSoft)
			main.Border(1, colAccentLine)
		} else if main.Hovered() {
			main.Background(colPanel2)
		}
		main.Children(func() {
			if connected {
				ui.Box(c).Width(2).Height(18).Radius(1).Background(colCyan)
			}
			dot := colMuted
			if connecting {
				dot = colAmber
			}
			if connected {
				dot = colGreen
			}
			statusDot(c, dot, 5)
			ui.Column(c).Grow(1).Gap(0).Children(func() {
				nameColor := colText2
				if connected {
					nameColor = colText
				}
				ui.Text(c, p.Name).FontSize(11.6).FontWeight(condWeight(connected, 600, 550)).
					TextColor(nameColor).NoWrap()
				auth := "自动"
				switch p.AuthType {
				case "password":
					auth = "密码"
				case "key":
					auth = "密钥"
				}
				sub := fmt.Sprintf("%s:%d · %s", p.Host, p.Port, auth)
				if connecting {
					sub = "正在连接…"
				}
				ui.Text(c, sub).FontSize(8.8).Font("Cascadia Code").
					TextColor(colMuted).NoWrap()
			})
			meta := "SSH"
			metaColor := colMuted2
			if connecting {
				meta = "连接中"
				metaColor = colAmber
			} else if connected {
				meta = "在线"
				metaColor = colCyan
			}
			ui.Text(c, meta).FontSize(8).Font("Cascadia Code").TextColor(metaColor).NoWrap()
		})
		main.ContextMenu(func(m *ui.Menu) {
			if m.Item("连接").Chosen() {
				a.connectSavedProfile(p)
			}
			m.Separator()
			if m.Item("编辑连接").Chosen() {
				a.editProfile(p)
			}
		})
		if main.Clicked() {
			a.connectSavedProfile(p)
		}
	})
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
		ui.Text(c, "点击左侧栏右上角的 + 新建连接，或打开本地终端").FontSize(11).TextColor(colMuted)
		ui.Row(c).Gap(8).Margin(6, 0, 0, 0).Children(func() {
			if ghostAction(c, t, "welcome-local", "本地终端", iconTerm).Clicked() {
				a.OpenLocalTerminal()
			}
		})
	})
}

func (a *App) connectModal(c *ui.Context, t *ui.Theme) {
	editing := a.profileID != 0
	title := "新建 SSH 连接"
	sub := "填写服务器信息，保存后会出现在左侧连接树"
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
		panel.Width(600).Padding(0).Background(colPanel).
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

			ui.Column(c).Padding(16, 18).Gap(10).Children(func() {
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
					ui.Text(c, "修改会保存到左侧连接树").
						FontSize(9.5).TextColor(colMuted2).NoWrap()
				} else {
					ui.Checkbox(c, &a.saveProfile, "保存到连接树")
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

func (a *App) previewFact(c *ui.Context, key, value string) {
	ui.Row(c).Height(22).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Text(c, key).FontSize(9).TextColor(colMuted2)
		ui.Text(c, "").Grow(1)
		ui.Text(c, shortLabel(value, 22)).FontSize(9).Font("Cascadia Code").
			TextColor(colText).NoWrap()
	})
}

func (a *App) pickerRow(c *ui.Context, t *ui.Theme, p store.Profile) {
	connected := a.profileConnected(p.ID)
	row := ui.ButtonBase(c.Key(fmt.Sprintf("picker-%d", p.ID))).
		FillWidth().Height(42).Padding(0, 8).Radius(8).Gap(9).AlignItems(ui.Center)
	if row.Hovered() {
		row.Background(colPanel2)
	}
	row.Children(func() {
		avatar := ui.Box(c).Size(28, 28).Radius(8).
			Background(colPanel2.Alpha(0.7)).Border(1, colLine).Center()
		avatar.Children(func() {
			color := colMuted
			if connected {
				color = colGreen
			}
			iconEl(c, iconServer, 13, color)
		})
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, p.Name).FontSize(11.5).FontWeight(550).NoWrap()
			ui.Text(c, fmt.Sprintf("%s:%d", p.Host, p.Port)).FontSize(9.5).
				Font("Cascadia Code").TextColor(colMuted).NoWrap()
		})
		statusDot(c, condColor(connected, colGreen, colMuted2), 5)
	})
	if row.Clicked() {
		a.pickerOpen = false
		a.connectSavedProfile(p)
	}
}

func (a *App) connectionPicker(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Width(380).Padding(6).Gap(4).Children(func() {
		ui.Row(c).Height(28).Padding(0, 6).Gap(7).AlignItems(ui.Center).Children(func() {
			iconEl(c, iconConnections, 13, colCyan)
			ui.Text(c, "打开连接").FontSize(10.5).FontWeight(650).Grow(1)
			if quietAction(c, t, "picker-manage", "管理", iconConnections, false).Clicked() {
				a.pickerOpen = false
				a.openConnectionManager()
			}
		})

		if len(a.profiles) == 0 {
			ui.Column(c).Padding(14, 8).Gap(5).Children(func() {
				ui.Text(c, "还没有保存的连接").FontSize(11).TextColor(colMuted)
				ui.Text(c, "使用左侧栏的 + 新建连接后会出现在这里").FontSize(9.5).TextColor(colMuted2)
			})
		} else {
			ui.Scroll(c).MaxHeight(300).Children(func() {
				for _, p := range a.profiles {
					a.pickerRow(c, t, p)
				}
			})
		}
	})
}

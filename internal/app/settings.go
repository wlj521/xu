package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

type appSettings struct {
	FontFamily    string
	FontSize      int
	ScrollbackMB  int
	Theme         int  // 0 follows the system, 1 dark, 2 light
	Accent        int  // 0 blue, 1 teal, 2 violet, 3 orange
	CopyOnSelect  bool // selection copies to the clipboard
	FindHighlight bool // terminal search highlights matches
	AutoReconnect bool // reconnects dropped sessions
}

func defaultSettings() appSettings {
	return appSettings{
		FontSize: 13, ScrollbackMB: 10, Theme: 1,
		Accent: 0, CopyOnSelect: true, FindHighlight: true, AutoReconnect: true,
	}
}

func (a *App) openSettings() {
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	if a.finding {
		a.closeFind()
	}
	a.settingsOpen = true
	a.formOpen = false
	a.searchOpen = false
	a.settingsErr = ""
	a.settingsStatus = ""
	a.settingsFontPick = fontChoiceForFamily(a.settings.FontFamily)
	a.settingsFontSizeF = float64(a.settings.FontSize)
	a.settingsScrollPick = fmt.Sprintf("%d MB", a.settings.ScrollbackMB)
}

func (a *App) closeSettings() {
	a.settingsOpen = false
}

func (a *App) settingsModal(c *ui.Context, t *ui.Theme) {
	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}
	sections := []struct {
		label string
		icon  *ui.SVG
	}{
		{"外观", iconSun},
		{"终端", iconTerm},
		{"连接", iconLink},
		{"关于", iconHelp},
	}
	titles := []string{"外观", "终端", "连接", "关于"}
	subtitles := []string{
		"深色与浅色拥有独立的表面层级与终端配色，切换会立即生效。",
		"字号与字体立即作用于已打开的终端，回滚缓冲对新会话生效。",
		"同一主机的终端、SFTP 与监控共用一个 SSH 连接。",
		"Xu 是一个不依赖 WebView 的原生 SSH 工作台。",
	}
	section := a.settingsSection
	if section < 0 || section >= len(titles) {
		section = 0
	}

	dlgW, dlgH := fitDialog(c, 800, 580)
	ui.DialogBase(c, &a.settingsOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(dlgW).Height(dlgH).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Width(220).FillHeight().Padding(24, 18, 20, 18).
					Background(colPanelTop).Radius(14, 0, 0, 14).
					BorderWidth(0, 1, 0, 0).BorderColor(colLine).Children(func() {
					ui.Column(c).FillWidth().Gap(3).Padding(0, 0, 20, 0).Children(func() {
						ui.Text(c, "设置").FontSize(18).FontWeight(650).TextColor(colText)
						ui.Text(c, "PREFERENCES").FontSize(8).FontWeight(650).
							LetterSpacing(1.3).TextColor(colMuted2)
					})

					ui.Column(c).FillWidth().Gap(4).Children(func() {
						for i, sectionItem := range sections {
							i, sectionItem := i, sectionItem
							active := section == i
							b := ui.ButtonBase(c.Key(fmt.Sprintf("settings-nav-%d", i))).
								FillWidth().Height(38).Padding(0, 12).Radius(8).
								Gap(9).AlignItems(ui.Center).Justify(ui.Start)
							if active {
								b.Background(colAccentSoft)
							} else if b.Hovered() {
								b.Background(colPanel2.Alpha(0.72))
							}
							b.Children(func() {
								iconEl(c, sectionItem.icon, 14, condColor(active, colCyan, colMuted))
								ui.Text(c, sectionItem.label).FontSize(11.8).
									FontWeight(condWeight(active, 600, 500)).
									TextColor(condColor(active, colText, colText2)).NoWrap()
							})
							if b.Clicked() {
								a.settingsSection = i
							}
						}
					})

					ui.Text(c, "").Grow(1)
					ui.Box(c).FillWidth().Height(1).Background(colLine).
						Margin(0, 0, 12, 0)
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Image(c, titleIcon).Size(24, 24).Fit(ui.Contain).Shrink(0)
						ui.Column(c).Grow(1).Gap(1).Children(func() {
							ui.Text(c, "Xu 0.1.0").FontSize(9.2).Font("Cascadia Code").
								TextColor(colText2)
							ui.Text(c, "修改即时生效").FontSize(8.2).TextColor(colMuted2)
						})
					})
				})

				ui.Column(c).Grow(1).FillHeight().Padding(24, 24, 20, 24).Gap(16).Children(func() {
					ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Grow(1).Gap(5).Children(func() {
							ui.Text(c, titles[section]).FontSize(18).FontWeight(650).TextColor(colText)
							ui.Text(c, subtitles[section]).FontSize(11).TextColor(colMuted).
								LineHeight(1.55)
						})
						close := ui.ButtonBase(c.Key("settings-modal-close")).Size(26, 26).
							Radius(7).Center().Tooltip("关闭").Label("关闭")
						if close.Hovered() {
							close.Background(colPanel2)
						}
						close.Children(func() { iconEl(c, iconClose, 13, colMuted) })
						if close.Clicked() {
							a.closeSettings()
						}
					})

					ui.Scroll(c).Grow(1).Children(func() {
						ui.Column(c).FillWidth().Gap(6).Children(func() {
							ui.Column(c).FillWidth().Children(func() {
								switch section {
								case 1:
									a.settingsTerminal(c, t)
								case 2:
									a.settingsConnection(c, t)
								case 3:
									a.settingsAbout(c, t)
								default:
									a.settingsAppearance(c, t)
								}
							})
						})
					})
				})
			})
		})
	})
}

func (a *App) settingsView(c *ui.Context, t *ui.Theme) {
	subtitles := []string{
		"主题与面板布局",
		"字体、字号与回滚缓冲",
		"连接保活、认证和主机密钥",
		"版本、数据目录与快捷键",
	}
	sub := "外观、终端与连接行为"
	if a.settingsSection >= 0 && a.settingsSection < len(subtitles) {
		sub = subtitles[a.settingsSection]
	}

	ui.Column(c).Grow(1).Fill().Padding(24, 28, 20, 28).Gap(16).Children(func() {
		pageHeader(c, t, iconGear, "设置", sub, func() {
			if ghostAction(c, t, "settings-close-top", "关闭", nil).Clicked() {
				a.closeSettings()
			}
		})

		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Width(680).Gap(14).Children(func() {
				switch a.settingsSection {
				case 1:
					a.settingsTerminal(c, t)
				case 2:
					a.settingsConnection(c, t)
				case 3:
					a.settingsAbout(c, t)
				default:
					a.settingsAppearance(c, t)
				}
			})
		})
	})
}

func (a *App) settingsAppearance(c *ui.Context, t *ui.Theme) {
	settingsGroup(c, "主题", true, func() {
		ui.Row(c).Gap(8).Children(func() {
			a.themeChoiceCard(c, t, "set-theme-dark", "深色", 1, "dark")
			a.themeChoiceCard(c, t, "set-theme-light", "浅色", 2, "light")
			a.themeChoiceCard(c, t, "set-theme-system", "跟随系统", 0, "system")
		})
	})

	settingsGroup(c, "强调色", false, func() {
		settingsRow(c, "界面强调色", "用于选中态、图表、终端光标与主按钮。", func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				for i, label := range []string{"蓝色", "青色", "紫色", "橙色"} {
					a.accentSwatch(c, i, label)
				}
			})
		})
	})

	settingsGroup(c, "布局", false, func() {
		settingsRow(c, "桌面布局", "左侧栏是服务器监控；打开的会话与新标签页集中在顶部标签栏，文件、传输、隧道与命令在底部面板。", nil)
		settingsNote(c, iconActivity, "服务器监控与底部文件面板各自带收起按钮，收起后会保留一个可点击的展开入口，不占用顶部操作区。")
	})
}

func (a *App) accentSwatch(c *ui.Context, index int, label string) {
	accent, _, _, _, _, _ := accentPalette(a.uiDark(), index)
	active := a.settings.Accent == index
	b := ui.ButtonBase(c.Key(fmt.Sprintf("accent-%d", index))).Size(24, 24).Radius(7).
		Background(accent).Tooltip(label).Label(label)
	if active {
		b.Border(2, colText)
	} else {
		b.Border(2, ui.Transparent)
	}
	if b.Hovered() {
		b.Border(2, colLine2)
	}
	if b.Clicked() {
		a.setAccent(index)
	}
}

func (a *App) themeChoiceCard(c *ui.Context, t *ui.Theme, key, label string, mode int, kind string) {
	active := a.settings.Theme == mode
	b := ui.ButtonBase(c.Key(key)).Width(164).Height(88).Padding(0).Radius(9).
		Border(1, condColor(active, colAccentLine, colLine)).Background(colPanel2)
	if active {
		b.Background(colAccentSoft)
	} else if b.Hovered() {
		b.BorderColor(colLine2)
	}
	b.Children(func() {
		ui.Column(c).Width(164).Children(func() {
			thumb := ui.Box(c).Width(150).Height(52).Margin(6, 6, 2, 6).Radius(6).
				Border(1, colLine)
			if kind == "system" {
				thumb.Draw(func(p *ui.Painter, r ui.Rect) {
					half := r.W / 2
					p.Fill(ui.Rect{X: r.X, Y: r.Y, W: half, H: r.H}, ui.Hex("#0C1119"), 1)
					p.Fill(ui.Rect{X: r.X + half, Y: r.Y, W: r.W - half, H: r.H}, ui.Hex("#F7F9FC"), 1)
				})
			} else if kind == "dark" {
				thumb.Background(ui.Hex("#0C1119")).Children(func() {
					ui.Column(c).Padding(6).Gap(3).Children(func() {
						ui.Box(c).FillWidth().Height(8).Radius(2).Background(ui.Hex("#141C28"))
						ui.Box(c).FillWidth().Height(8).Radius(2).Background(ui.Hex("#1A2433"))
						ui.Box(c).Width(48).Height(8).Radius(2).Background(ui.Hex("#1C4F7A"))
					})
				})
			} else {
				thumb.Background(ui.Hex("#F7F9FC")).Children(func() {
					ui.Column(c).Padding(6).Gap(3).Children(func() {
						ui.Box(c).FillWidth().Height(8).Radius(2).Background(ui.Hex("#EEF2F8"))
						ui.Box(c).FillWidth().Height(8).Radius(2).Background(ui.Hex("#DDE4EF"))
						ui.Box(c).Width(48).Height(8).Radius(2).Background(ui.Hex("#6F8FBE"))
					})
				})
			}
			ui.Row(c).Height(24).Padding(0, 8).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).FontSize(11.5).FontWeight(condWeight(active, 600, 500)).
					TextColor(condColor(active, colText, colText2))
				ui.Text(c, "").Grow(1)
				if active {
					iconEl(c, iconCheck, 11, colCyan)
				}
			})
		})
	})
	if b.Clicked() && a.settings.Theme != mode {
		a.setThemeMode(mode)
	}
}

func (a *App) settingsTerminal(c *ui.Context, t *ui.Theme) {
	settingsGroup(c, "显示", true, func() {
		settingsRow(c, "字号", "终端等宽字体的显示大小。", func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				slider := ui.StepSlider(c, &a.settingsFontSizeF, 11, 18, 0.5).Width(150)
				if slider.Changed() {
					a.settingsFontSize = strconv.Itoa(int(a.settingsFontSizeF + 0.5))
					a.settings.FontSize = int(a.settingsFontSizeF + 0.5)
					a.applyTermSettings()
				}
				ui.Text(c, fmt.Sprintf("%.1f px", a.settingsFontSizeF)).FontSize(10.5).
					Font("Cascadia Code").TextColor(colText2).Width(48).TextAlign(ui.End)
			})
		})
		settingsRow(c, "字体", "仅显示已安装的等宽字体，选择后立即应用。", func() {
			pick := ui.Combobox(c, &a.settingsFontPick, terminalFontChoices(a.settings.FontFamily)).
				Width(250).Height(31).Label("终端字体")
			if pick.Changed() {
				a.settings.FontFamily = fontFamilyForChoice(a.settingsFontPick)
				a.applyTermSettings()
			}
		})
	})

	settingsGroup(c, "缓冲", false, func() {
		settingsRow(c, "回滚缓冲", "每个终端保留的输出上限。", func() {
			sel := ui.Select(c, &a.settingsScrollPick, []string{"10 MB", "20 MB", "50 MB"}).Width(120)
			if sel.Changed() {
				a.settingsScrollback = strings.TrimSuffix(a.settingsScrollPick, " MB")
			}
		})
	})

	settingsGroup(c, "行为", false, func() {
		settingsRow(c, "选中即复制", "选择文本后立即写入剪贴板。", func() {
			if ui.Switch(c, &a.settings.CopyOnSelect).Changed() {
				a.applyTermSettings()
			}
		})
		settingsRow(c, "查找高亮", "终端内搜索的命中项使用强调色。", func() {
			if ui.Switch(c, &a.settings.FindHighlight).Changed() {
				a.applyTermSettings()
			}
		})
	})
}

func (a *App) settingsConnection(c *ui.Context, t *ui.Theme) {
	settingsGroup(c, "保持与重连", true, func() {
		settingsKV(c, "心跳保活", "每 30 秒")
		settingsKV(c, "握手超时", "15 秒")
		settingsKV(c, "空闲回收", "5 分钟")
		settingsKV(c, "重连策略", "指数退避 · 最多 3 次")
		settingsRow(c, "断线自动重连", "网络恢复后自动恢复终端与 SFTP 会话。", func() {
			ui.Switch(c, &a.settings.AutoReconnect)
		})
	})

	settingsGroup(c, "安全", false, func() {
		settingsKV(c, "密码保存", "系统钥匙串")
		settingsKV(c, "数据库明文", "不保存")
		settingsKV(c, "主机密钥", "首次核对 · 变化阻断")
		settingsNote(c, iconShield, "私钥从 ~/.xu/keys 读取：id_ed25519、id_ecdsa、id_rsa。")
	})
}

func (a *App) settingsAbout(c *ui.Context, t *ui.Theme) {
	ui.Row(c).FillWidth().Padding(12, 14).Gap(12).Radius(9).
		Background(colPanel2).Border(1, colLine).AlignItems(ui.Center).Children(func() {
		ui.Image(c, titleIcon).Size(38, 38).Fit(ui.Contain).Shrink(0)
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, "Xu 0.1.0").FontSize(14.5).FontWeight(650).TextColor(colText)
			ui.Text(c, "SSH Workspace · mygo + Ghostty VT").FontSize(10.5).
				Font("Cascadia Code").TextColor(colMuted)
		})
	})

	settingsGroup(c, "数据", false, func() {
		settingsKV(c, "数据目录", "~/.xu")
		settingsKV(c, "会话数据库", "xu.db · SQLite")
		settingsKV(c, "终端引擎", "Ghostty VT")
	})

	settingsGroup(c, "快捷键", false, func() {
		pairs := [][2]string{
			{"新建连接", "Ctrl N"},
			{"本地终端", "Ctrl T"},
			{"新建标签页", "Ctrl Alt T"},
			{"连接管理器", "Ctrl Alt C"},
			{"终端查找", "Ctrl Shift F"},
			{"服务器监控", "Ctrl B"},
			{"底部面板", "Ctrl J"},
			{"全局搜索", "Ctrl K"},
		}
		for i := 0; i < len(pairs); i += 2 {
			i := i
			ui.Row(c).FillWidth().Gap(24).Children(func() {
				for j := 0; j < 2 && i+j < len(pairs); j++ {
					pair := pairs[i+j]
					ui.Row(c).Grow(1).AlignItems(ui.Center).Children(func() {
						ui.Text(c, pair[0]).FontSize(10.8).TextColor(colText2)
						ui.Text(c, "").Grow(1)
						ui.Box(c).Height(18).Padding(0, 6).Radius(4).Background(colPanel2).
							Border(1, colLine).Center().Children(func() {
							ui.Text(c, pair[1]).FontSize(9.2).Font("Cascadia Code").TextColor(colMuted2)
						})
					})
				}
			})
		}
	})
}

func settingsGroup(c *ui.Context, label string, first bool, build func()) {
	top := float32(13)
	if first {
		top = 0
	}
	col := ui.Column(c).FillWidth().Padding(top, 0, 13, 0).Gap(9)
	if !first {
		col.BorderWidth(1, 0, 0, 0).BorderColor(colLine)
	}
	col.Children(func() {
		if label != "" {
			ui.Text(c, label).FontSize(9.5).FontWeight(650).LetterSpacing(0.8).TextColor(colMuted2)
		}
		build()
	})
}

func settingsRow(c *ui.Context, title, desc string, control func()) {
	ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(colText2)
			if desc != "" {
				ui.Text(c, desc).FontSize(10).TextColor(colMuted).LineHeight(1.55)
			}
		})
		if control != nil {
			control()
		}
	})
}

func settingsKV(c *ui.Context, key, value string) {
	ui.Row(c).FillWidth().Padding(6, 0).BorderWidth(0, 0, 1, 0).BorderColor(colLine).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, key).FontSize(10.8).TextColor(colMuted)
		ui.Text(c, "").Grow(1)
		ui.Text(c, value).FontSize(10.2).Font("Cascadia Code").TextColor(colText2)
	})
}

func settingsNote(c *ui.Context, icon *ui.SVG, text string) {
	ui.Row(c).FillWidth().Padding(8, 10).Gap(7).Radius(7).
		Background(colPanel2).Border(1, colLine).AlignItems(ui.Start).Children(func() {
		iconEl(c, icon, 11, colMuted)
		ui.Text(c, text).FontSize(10).TextColor(colMuted).LineHeight(1.55)
	})
}

func (a *App) settingsFooter(c *ui.Context, t *ui.Theme) {
	if a.settingsErr != "" {
		ui.Box(c).FillWidth().Padding(9, 11).Radius(6).
			Background(colRed.Alpha(0.07)).Border(1, colRed.Alpha(0.18)).Children(func() {
			ui.Text(c, a.settingsErr).FontSize(11).TextColor(colRed)
		})
	}
	if a.settingsStatus != "" {
		ui.Box(c).FillWidth().Padding(9, 11).Radius(6).
			Background(colGreen.Alpha(0.07)).Border(1, colGreen.Alpha(0.18)).Children(func() {
			ui.Text(c, a.settingsStatus).FontSize(11).TextColor(colGreen)
		})
	}
	ui.Row(c).Gap(8).Children(func() {
		if primaryAction(c, "settings-save", "保存设置", iconSave).Clicked() {
			a.saveSettings()
		}
		ui.Text(c, "修改会立即作用于此窗口。").
			FontSize(10.5).TextColor(colMuted2).NoWrap()
	})
}

func settingsCard(c *ui.Context, title, desc string, build func()) {
	ui.Column(c).FillWidth().Padding(18).Background(colPanel).
		Border(1, colLine).Radius(8).Gap(12).Children(func() {
		if title != "" {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(colText)
				if desc != "" {
					ui.Text(c, desc).FontSize(11).TextColor(colMuted2).LineHeight(1.5)
				}
			})
		}
		build()
	})
}

func (a *App) loadSettings() {
	a.settings = defaultSettings()
	a.settingsFontSize = strconv.Itoa(a.settings.FontSize)
	a.settingsScrollback = strconv.Itoa(a.settings.ScrollbackMB)

	if a.store == nil {
		return
	}
	values, err := a.store.Settings(a.ctx)
	if err != nil {
		return
	}

	if v := values["terminal.font.family"]; v != "" {
		a.settings.FontFamily = v
	}
	if v := values["terminal.font.size"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 8 && n <= 40 {
			a.settings.FontSize = n
		}
	}
	if v := values["terminal.scrollback_mb"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 100 {
			a.settings.ScrollbackMB = n
		}
	}
	if v := values["terminal.theme"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 2 {
			a.settings.Theme = n
		}
	}
	if v := values["ui.accent"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 3 {
			a.settings.Accent = n
		}
	}
	if values["terminal.copy_on_select"] == "0" {
		a.settings.CopyOnSelect = false
	}
	if values["terminal.find_highlight"] == "0" {
		a.settings.FindHighlight = false
	}
	if values["connection.auto_reconnect"] == "0" {
		a.settings.AutoReconnect = false
	}

	a.settingsFontSize = strconv.Itoa(a.settings.FontSize)
	a.settingsScrollback = strconv.Itoa(a.settings.ScrollbackMB)
	a.settingsFontSizeF = float64(a.settings.FontSize)
	a.settingsScrollPick = fmt.Sprintf("%d MB", a.settings.ScrollbackMB)
	a.settingsFontPick = fontChoiceForFamily(a.settings.FontFamily)
}

func (a *App) saveSettings() {
	size, err := strconv.Atoi(strings.TrimSpace(a.settingsFontSize))
	if err != nil || size < 8 || size > 40 {
		a.settingsErr = "字号需在 8–40 之间"
		return
	}
	scroll, err := strconv.Atoi(strings.TrimSpace(a.settingsScrollback))
	if err != nil || scroll < 1 || scroll > 100 {
		a.settingsErr = "回滚缓冲需在 1–100 MB 之间"
		return
	}

	a.settings.FontFamily = strings.TrimSpace(a.settings.FontFamily)
	a.settings.FontSize = size
	a.settings.ScrollbackMB = scroll

	if a.store != nil {
		values := map[string]string{
			"terminal.font.family":      a.settings.FontFamily,
			"terminal.font.size":        strconv.Itoa(size),
			"terminal.scrollback_mb":    strconv.Itoa(scroll),
			"terminal.theme":            strconv.Itoa(a.settings.Theme),
			"ui.accent":                 strconv.Itoa(a.settings.Accent),
			"terminal.copy_on_select":   boolText(a.settings.CopyOnSelect),
			"terminal.find_highlight":   boolText(a.settings.FindHighlight),
			"connection.auto_reconnect": boolText(a.settings.AutoReconnect),
		}
		for key, value := range values {
			if err := a.store.SetSetting(a.ctx, key, value); err != nil {
				a.settingsErr = "保存失败: " + err.Error()
				return
			}
		}
		a.settingsStatus = "已保存"
	} else {
		a.settingsStatus = "已保存（仅本次运行，会话库未就绪）"
	}

	a.settingsErr = ""
	a.applyTermSettings()
	a.ApplyThemeSource()
}

func boolText(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func (a *App) termDisplay() termDisplay {
	// New terminals must see the palette of the current theme, even when the
	// app creates one before the first frame paints the interface.
	a.updatePalette(a.uiDark())
	size := a.settings.FontSize
	if size < 8 || size > 40 {
		size = 14
	}
	scroll := a.settings.ScrollbackMB
	if scroll < 1 || scroll > 100 {
		scroll = 10
	}
	light, dark := a.termThemes()
	return termDisplay{
		Font:         terminal.Font{Family: a.settings.FontFamily, Size: float32(size)},
		Scrollback:   scroll << 20,
		CopyOnSelect: a.settings.CopyOnSelect,
		Light:        light,
		Dark:         dark,
	}
}

func copyOnSelect(display termDisplay) terminal.CopyOnSelect {
	if display.CopyOnSelect {
		return terminal.CopyOnSelectClipboard
	}
	return terminal.CopyOnSelectNone
}

func (a *App) termThemes() (*terminal.Theme, *terminal.Theme) {
	light := terminal.LightTheme()
	light.Background = colTermBg
	light.Foreground = colText
	light.Cursor = colCyan
	light.CursorText = ui.Hex("#FFFFFF")
	light.Selection = colAccentSoft
	light.SelectionText = colText
	light.Search = colAmber
	light.SearchText = ui.Hex("#FFFFFF")
	light.SearchSelected = colRed
	light.SearchSelectedText = ui.Hex("#FFFFFF")
	light.Palette = [16]ui.Color{
		ui.Hex("#151D2E"), ui.Hex("#D94357"), ui.Hex("#0F9D6B"), ui.Hex("#B7791F"),
		ui.Hex("#2F6FEB"), ui.Hex("#7C5CF0"), ui.Hex("#0891B2"), ui.Hex("#F8FAFC"),
		ui.Hex("#7C889C"), ui.Hex("#E86A7B"), ui.Hex("#2BB985"), ui.Hex("#D79A3E"),
		ui.Hex("#5C93F2"), ui.Hex("#9B7FF5"), ui.Hex("#2AA7C6"), ui.Hex("#151D2E"),
	}

	dark := terminal.DarkTheme()
	dark.Background = colTermBg
	dark.Foreground = ui.Hex("#BCBEC4")
	dark.Cursor = colCyan
	dark.CursorText = colAccentText
	dark.Selection = ui.Hex("#214283")
	dark.SelectionText = colText
	dark.Search = colAmber
	dark.SearchText = colInk
	dark.SearchSelected = colRed
	dark.SearchSelectedText = colInk
	dark.Palette = [16]ui.Color{
		ui.Hex("#000000"), ui.Hex("#CD3131"), ui.Hex("#0DBC79"), ui.Hex("#E5E510"),
		ui.Hex("#2472C8"), ui.Hex("#BC3FBC"), ui.Hex("#11A8CD"), ui.Hex("#E5E5E5"),
		ui.Hex("#666666"), ui.Hex("#F14C4C"), ui.Hex("#23D18B"), ui.Hex("#F5F543"),
		ui.Hex("#3B8EEA"), ui.Hex("#D670D6"), ui.Hex("#29B8DB"), ui.Hex("#F0F0F0"),
	}
	if !a.settings.FindHighlight {
		light.Search = light.Background
		light.SearchSelected = light.Background
		dark.Search = dark.Background
		dark.SearchSelected = dark.Background
	}

	switch a.settings.Theme {
	case 1:
		return dark, dark
	case 2:
		return light, light
	default:
		return light, dark
	}
}

func (a *App) applyTermSettings() {
	display := a.termDisplay()
	for _, ts := range a.terms {
		if ts.Term == nil {
			continue
		}
		ts.Term.SetFont(display.Font)
		ts.Term.SetTheme(display.Light, display.Dark)
		ts.Term.SetCopyOnSelect(copyOnSelect(display))
	}
}

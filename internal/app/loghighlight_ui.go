package app

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// settingsLogHighlight is the "日志高亮" group in the terminal settings.
func (a *App) settingsLogHighlight(c *ui.Context, t *ui.Theme) {
	settingsGroup(c, "日志高亮", false, func() {
		settingsRow(c, "启用日志高亮", "给 SSH 终端的新输出着色；已有回看不会重绘。", func() {
			if ui.Switch(c, &a.settings.LogHighlight).Changed() {
				a.applyLogHighlight()
			}
		})
		settingsRow(c, "规则与格式", fmt.Sprintf("%d 条规则 · %d 条自定义格式",
			len(a.settings.LogRules), len(a.settings.LogFormats)), func() {
			if ghostAction(c, t, "log-rules-open", "规则与格式", iconEdit).Clicked() {
				a.logRulesOpen = true
				a.logRulesTab = 0
			}
		})
		settingsRow(c, "仅处理日志行", "只匹配内置或自定义格式开头的行，避免影响普通命令输出。", func() {
			if ui.Switch(c, &a.settings.LogAnchorOnly).Changed() {
				a.applyLogHighlight()
			}
		})
		settingsRow(c, "预设", "切换会覆盖当前规则，之后仍可编辑。", func() {
			if a.logPresetPick == "" {
				a.logPresetPick = logPresetNames()[0]
			}
			if sel := ui.Select(c, &a.logPresetPick, logPresetNames()).Width(160); sel.Changed() {
				a.applyLogPreset(a.logPresetPick)
			}
		})
		settingsNote(c, iconTerm, "含 ANSI 颜色、进度条和全屏程序的行会原样放行；本地 PowerShell 暂不支持。")
	})
}

func (a *App) applyLogPreset(name string) {
	rules := logRulesForPreset(name)
	if rules == nil {
		return
	}
	a.settings.LogRules = rules
	a.logPresetPick = name
	a.applyLogHighlight()
}

func (a *App) markLogRulesCustom() {
	if a.logPresetPick != "自定义" {
		a.logPresetPick = "自定义"
	}
}

func (a *App) addLogRule() {
	a.settings.LogRules = append(a.settings.LogRules, logRule{
		ID: nextLogRuleID(), Name: "新规则", Pattern: "ERROR", Color: 1, Bold: true, Enabled: true,
	})
	a.markLogRulesCustom()
	a.applyLogHighlight()
}

func (a *App) restoreLogRules() {
	a.settings.LogRules = defaultLogRules()
	a.logPresetPick = "通用日志"
	a.applyLogHighlight()
}

func (a *App) moveLogRule(index, delta int) {
	j := index + delta
	if index < 0 || index >= len(a.settings.LogRules) || j < 0 || j >= len(a.settings.LogRules) {
		return
	}
	rules := a.settings.LogRules
	rules[index], rules[j] = rules[j], rules[index]
	a.markLogRulesCustom()
	a.applyLogHighlight()
}

func (a *App) removeLogRule(id int64) {
	for i, rule := range a.settings.LogRules {
		if rule.ID == id {
			a.settings.LogRules = append(a.settings.LogRules[:i], a.settings.LogRules[i+1:]...)
			break
		}
	}
	a.markLogRulesCustom()
	a.applyLogHighlight()
}

func (a *App) logPalette() [16]ui.Color {
	light, dark := a.termThemes()
	th := light
	if a.uiDark() {
		th = dark
	}
	if th == nil {
		return terminal.DarkTheme().Palette
	}
	return th.Palette
}

type logRuleAction struct {
	kind  string
	index int
	id    int64
}

// logRulesModal edits the rules in place: each change recompiles the pack
// for the open SSH sessions at once.
func (a *App) logRulesModal(c *ui.Context, t *ui.Theme) {
	if !a.settingsOpen {
		a.logRulesOpen = false
		return
	}
	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}
	dlgW, dlgH := fitDialog(c, 680, 600)
	pal := a.logPalette()

	ui.DialogBase(c, &a.logRulesOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(dlgW).Height(dlgH).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Column(c).Grow(1).Padding(20).Gap(12).Children(func() {
				ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).Gap(3).Children(func() {
						ui.Text(c, "日志高亮").FontSize(15).FontWeight(650).TextColor(colText)
						ui.Text(c, "规则决定字段颜色；自定义格式决定哪些行算日志。").
							FontSize(9.4).TextColor(colMuted)
					})
					close := ui.ButtonBase(c.Key("log-rules-close")).Size(24, 24).
						Radius(6).Center().Tooltip("关闭").Label("关闭")
					if close.Hovered() {
						close.Background(colPanel2)
					}
					close.Children(func() { iconEl(c, iconClose, 13, colMuted) })
					if close.Clicked() {
						a.logRulesOpen = false
					}
				})

				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					a.logTabButton(c, 0, "规则")
					a.logTabButton(c, 1, "日志格式")
					ui.Text(c, "").Grow(1)
					ui.Text(c, "内置格式始终生效").FontSize(8.8).TextColor(colMuted2)
				})

				if a.logRulesTab == 1 {
					ui.Scroll(c.Key("log-formats-scroll")).Grow(1).Scrollbars(false).Children(func() {
						ui.Column(c).FillWidth().Gap(8).Children(func() {
							a.logFormatsPage(c)
						})
					})
				} else {
					ui.Scroll(c.Key("log-rules-scroll")).Grow(1).Scrollbars(false).Children(func() {
						ui.Column(c).FillWidth().Gap(8).Children(func() {
							if len(a.settings.LogRules) == 0 {
								ui.Column(c).FillWidth().Padding(24).Center().Children(func() {
									ui.Text(c, "还没有规则").FontSize(10.5).TextColor(colMuted)
								})
							}
							var action logRuleAction
							for i := range a.settings.LogRules {
								a.logRuleCard(c, t, i, pal, &action)
							}
							switch action.kind {
							case "up":
								a.moveLogRule(action.index, -1)
							case "down":
								a.moveLogRule(action.index, 1)
							case "delete":
								a.removeLogRule(action.id)
							}
						})
					})
				}

				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					if a.logRulesTab == 1 {
						if ghostAction(c, t, "log-format-add", "添加格式", iconPlus).Clicked() {
							a.addLogFormat()
						}
						if ghostAction(c, t, "log-format-clear", "清空", iconTrash).Clicked() {
							a.clearLogFormats()
						}
					} else {
						if ghostAction(c, t, "log-rule-add", "添加规则", iconPlus).Clicked() {
							a.addLogRule()
						}
						if ghostAction(c, t, "log-rule-defaults", "恢复默认", iconRefresh).Clicked() {
							a.restoreLogRules()
						}
					}
					ui.Text(c, "").Grow(1)
					if primaryAction(c, "log-rules-done", "完成", nil).Clicked() {
						a.logRulesOpen = false
					}
				})
			})
		})
	})
}

func (a *App) logTabButton(c *ui.Context, index int, label string) {
	active := a.logRulesTab == index
	button := ui.ButtonBase(c.Key(fmt.Sprintf("log-tab-%d", index))).Height(28).
		Padding(0, 12).Radius(6).Label(label)
	if active {
		button.Background(colAccentSoft)
	} else if button.Hovered() {
		button.Background(colPanel2)
	}
	button.Children(func() {
		ui.Text(c, label).FontSize(9.8).FontWeight(condWeight(active, 600, 500)).
			TextColor(condColor(active, colText, colText2)).NoWrap()
	})
	if button.Clicked() {
		a.logRulesTab = index
	}
}

func (a *App) logFormatsPage(c *ui.Context) {
	ui.Text(c, "内置格式（时间戳、[级别]、Nginx、JSON、[traceID]）始终生效。自定义格式的正则从行首匹配，可添加多条。").
		FontSize(9).TextColor(colMuted).LineHeight(1.5)

	examples := []struct {
		name    string
		pattern string
		desc    string
	}{
		{"MyApp", `^\[MYAPP\]`, "匹配 [MYAPP] 开头的行"},
		{"syslog", `^[A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}`, "匹配 Nov 11 14:52:04 开头"},
		{"TraceID + 日期", `^\[[0-9a-fA-F-]{16,64}\]\s+\d{4}-\d{2}-\d{2}`, "匹配 [1a66...] 2026-11-11 开头"},
	}
	ui.Text(c, "示例 · 点「使用」直接添加").FontSize(9.2).FontWeight(600).TextColor(colMuted2)
	for i, example := range examples {
		a.logFormatExample(c, i, example.name, example.pattern, example.desc)
	}

	a.logFormatTester(c)

	ui.Text(c, fmt.Sprintf("我的格式 · %d 条", len(a.settings.LogFormats))).
		FontSize(9.2).FontWeight(600).TextColor(colMuted2)
	if len(a.settings.LogFormats) == 0 {
		ui.Column(c).FillWidth().Padding(18).Center().Children(func() {
			ui.Text(c, "还没有自定义格式").FontSize(10.5).TextColor(colMuted)
		})
		return
	}
	var deleteID int64
	for i := range a.settings.LogFormats {
		a.logFormatCard(c, i, &deleteID)
	}
	if deleteID != 0 {
		a.removeLogFormat(deleteID)
	}
}

func (a *App) logFormatExample(c *ui.Context, index int, name, pattern, desc string) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Padding(6, 8).
		Background(colPanel2).Radius(6).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, name+" · "+desc).FontSize(8.9).TextColor(colMuted).NoWrap()
			ui.Text(c, pattern).Font("Cascadia Code").FontSize(8.6).
				TextColor(colText2).NoWrap()
		})
		button := ui.ButtonBase(c.Key(fmt.Sprintf("log-format-example-%d", index))).
			Height(24).Padding(0, 10).Radius(5).Label("使用")
		if button.Hovered() {
			button.Background(colPanel)
		}
		button.Children(func() {
			ui.Text(c, "使用").FontSize(9).TextColor(colCyan)
		})
		if button.Clicked() {
			a.addLogFormatExample(name, pattern)
		}
	})
}

func (a *App) logFormatTester(c *ui.Context) {
	ui.Text(c, "测试一行").FontSize(9.2).FontWeight(600).TextColor(colMuted2)
	ui.TextInput(c, &a.logFormatTest).FillWidth().Height(30).
		Font("Cascadia Code").FontSize(9.2).
		Placeholder("粘贴一行日志，检查是否会被识别")

	line := strings.TrimRight(strings.TrimSpace(a.logFormatTest), "\r\n")
	if line == "" {
		return
	}
	raw := []byte(line)
	if bytes.IndexByte(raw, 0x1b) >= 0 {
		ui.Text(c, "这一行含 ANSI 颜色码，会被原样放行，不参与高亮。").
			FontSize(8.8).TextColor(colAmber)
		return
	}
	if bytes.IndexByte(raw, '\r') >= 0 {
		ui.Text(c, "这一行含回车符（进度条/刷新行），会被原样放行。").
			FontSize(8.8).TextColor(colAmber)
		return
	}
	cfg := a.logHL.load()
	if cfg == nil || !cfg.enabled {
		ui.Text(c, "日志高亮当前未启用。").FontSize(8.8).TextColor(colAmber)
		return
	}
	if !a.settings.LogAnchorOnly {
		ui.Text(c, "「仅处理日志行」已关闭，所有纯文本行都会进入规则。").
			FontSize(8.8).TextColor(colMuted)
	} else if !cfg.isLogLine(raw) {
		ui.Text(c, "不会被识别为日志行：需要添加格式，或关闭「仅处理日志行」。").
			FontSize(8.8).TextColor(colRed)
		return
	} else {
		ui.Text(c, "会被识别为日志行").FontSize(8.8).TextColor(colGreen)
	}
	if spans := cfg.spans(raw); len(spans) == 0 {
		ui.Text(c, "但没有规则命中，不会变色；可以添加或调整规则。").
			FontSize(8.8).TextColor(colAmber)
	} else {
		ui.Text(c, fmt.Sprintf("规则命中 %d 处，会按规则着色。", len(spans))).
			FontSize(8.8).TextColor(colGreen)
	}
}

func (a *App) logFormatCard(c *ui.Context, index int, deleteID *int64) {
	format := &a.settings.LogFormats[index]
	key := fmt.Sprintf("log-format-%d", format.ID)
	border := colLine
	if !format.Enabled {
		border = colLine2
	}
	ui.Column(c).FillWidth().Padding(10, 9).Gap(7).Background(colPanelTop).
		Border(1, border).Radius(9).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c.Key(key+"-name"), &format.Name).Grow(1).Height(28).
				FontSize(10.2).Placeholder("格式名称")
			ui.Text(c, "启用").FontSize(9.2).TextColor(colMuted2)
			if ui.Switch(c, &format.Enabled).Changed() {
				a.applyLogHighlight()
			}
			a.logRuleIconButton(c, key+"-delete", iconTrash, "删除", func() {
				*deleteID = format.ID
			})
		})
		if pattern := ui.TextInput(c.Key(key+"-pattern"), &format.Pattern).
			FillWidth().Height(30).Font("Cascadia Code").FontSize(9.8).
			Placeholder(`从行首匹配的正则，例如 ^\[MYAPP\]`); pattern.Changed() {
			a.applyLogHighlight()
		}
		if err := a.logFormatErrs[format.ID]; err != "" {
			ui.Text(c, "匹配错误："+shortLabel(err, 150)).FontSize(8.8).
				TextColor(colRed).LineHeight(1.4)
		}
	})
}

func (a *App) addLogFormat() {
	a.appendLogFormat("新格式", `^\[YOUR_LOG_PREFIX\]`)
}

func (a *App) addLogFormatExample(name, pattern string) {
	for _, format := range a.settings.LogFormats {
		if strings.TrimSpace(format.Pattern) == pattern {
			a.status = "该格式已存在"
			return
		}
	}
	a.appendLogFormat(name, pattern)
}

func (a *App) appendLogFormat(name, pattern string) {
	a.settings.LogFormats = append(a.settings.LogFormats, logFormat{
		ID: nextLogRuleID(), Name: name, Pattern: pattern, Enabled: true,
	})
	a.applyLogHighlight()
}

func (a *App) removeLogFormat(id int64) {
	for i, format := range a.settings.LogFormats {
		if format.ID == id {
			a.settings.LogFormats = append(a.settings.LogFormats[:i], a.settings.LogFormats[i+1:]...)
			break
		}
	}
	a.applyLogHighlight()
}

func (a *App) clearLogFormats() {
	a.settings.LogFormats = nil
	a.applyLogHighlight()
}

func (a *App) logRuleCard(c *ui.Context, t *ui.Theme, index int, pal [16]ui.Color, action *logRuleAction) {
	rule := &a.settings.LogRules[index]
	key := fmt.Sprintf("log-rule-%d", rule.ID)
	colorIndex := rule.Color
	if colorIndex < 0 || colorIndex > 15 {
		colorIndex = 7
	}
	border := colLine
	if !rule.Enabled {
		border = colLine2
	}

	ui.Column(c).FillWidth().Padding(10, 9).Gap(7).Background(colPanelTop).
		Border(1, border).Radius(9).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(11, 11).Radius(3).Shrink(0).Background(pal[colorIndex])
			if name := ui.TextInput(c.Key(key+"-name"), &rule.Name).
				Grow(1).Height(28).FontSize(10.2).
				Placeholder("规则名称"); name.Changed() {
				a.markLogRulesCustom()
			}
			ui.Text(c, "启用").FontSize(9.2).TextColor(colMuted2)
			if ui.Switch(c, &rule.Enabled).Changed() {
				a.markLogRulesCustom()
				a.applyLogHighlight()
			}
			a.logRuleIconButton(c, key+"-up", iconChevronUp, "上移", func() {
				*action = logRuleAction{kind: "up", index: index}
			})
			a.logRuleIconButton(c, key+"-down", iconChevron, "下移", func() {
				*action = logRuleAction{kind: "down", index: index}
			})
			a.logRuleIconButton(c, key+"-delete", iconTrash, "删除", func() {
				*action = logRuleAction{kind: "delete", index: index, id: rule.ID}
			})
		})

		if pattern := ui.TextInput(c.Key(key+"-pattern"), &rule.Pattern).
			FillWidth().Height(30).
			Font("Cascadia Code").FontSize(9.8).
			Placeholder("正则表达式，或关键字"); pattern.Changed() {
			a.markLogRulesCustom()
			a.applyLogHighlight()
		}
		if err := a.logRuleErrs[rule.ID]; err != "" {
			ui.Text(c, "匹配错误："+shortLabel(err, 150)).FontSize(8.8).
				TextColor(colRed).LineHeight(1.4)
		}

		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			a.logRuleFlag(c, "关键字", &rule.Keyword)
			a.logRuleFlag(c, "整行", &rule.Whole)
			a.logRuleFlag(c, "加粗", &rule.Bold)
			ui.Text(c, "").Grow(1)
			ui.Row(c).Gap(3).Shrink(0).Children(func() {
				for i := 0; i < 16; i++ {
					i := i
					swatch := ui.ButtonBase(c.Key(fmt.Sprintf("%s-color-%d", key, i))).
						Size(15, 15).Radius(3).Background(pal[i]).
						Tooltip(fmt.Sprintf("调色板 %d", i)).Label(fmt.Sprintf("调色板 %d", i))
					if colorIndex == i {
						swatch.Border(2, colCyan)
					} else {
						swatch.Border(1, colLine)
					}
					if swatch.Clicked() {
						rule.Color = i
						a.markLogRulesCustom()
						a.applyLogHighlight()
					}
				}
			})
		})
	})
}

func (a *App) logRuleFlag(c *ui.Context, label string, value *bool) {
	ui.Row(c).Gap(5).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, label).FontSize(9.2).TextColor(colMuted2).NoWrap().Shrink(0)
		if ui.Switch(c, value).Changed() {
			a.markLogRulesCustom()
			a.applyLogHighlight()
		}
	})
}

func (a *App) logRuleIconButton(c *ui.Context, key string, icon *ui.SVG, tip string, action func()) {
	button := ui.ButtonBase(c.Key(key)).Size(22, 22).Radius(5).Center().
		Tooltip(tip).Label(tip)
	if button.Hovered() {
		button.Background(colPanel2)
	}
	button.Children(func() { iconEl(c, icon, 11, colMuted) })
	if button.Clicked() {
		action()
	}
}

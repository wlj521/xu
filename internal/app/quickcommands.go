package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

const (
	quickCmdScopeServer = iota
	quickCmdScopeGlobal
)

func (a *App) loadQuickCommands() {
	if a.store == nil {
		a.quickCommands = nil
		return
	}
	commands, err := a.store.QuickCommands(a.ctx)
	if err != nil {
		a.status = "读取快捷命令失败: " + err.Error()
		return
	}
	a.quickCommands = commands
}

func (a *App) quickCommandProfile() (store.Profile, bool) {
	ts := a.activeTerminal()
	if ts == nil || ts.profileID == 0 {
		return store.Profile{}, false
	}
	for _, p := range a.profiles {
		if p.ID == ts.profileID {
			return p, true
		}
	}
	return store.Profile{ID: ts.profileID, Name: ts.Title}, true
}

func (a *App) quickCommandProfileName(id int64) string {
	for _, p := range a.profiles {
		if p.ID == id {
			return p.Name
		}
	}
	return fmt.Sprintf("服务器 %d", id)
}

func (a *App) quickCommandGroups(profileID int64) (server, global []store.QuickCommand) {
	for _, q := range a.quickCommands {
		switch {
		case q.ProfileID == 0:
			global = append(global, q)
		case q.ProfileID == profileID:
			server = append(server, q)
		}
	}
	return server, global
}

func (a *App) toggleQuickCommands() {
	if a.activeTerminal() == nil {
		a.status = "请先打开一个终端"
		return
	}
	a.quickCmdOpen = !a.quickCmdOpen
}

func (a *App) runQuickCommand(q store.QuickCommand) {
	ts := a.activeTerminal()
	if ts == nil || ts.Term == nil || ts.exited {
		a.quickCmdOpen = false
		a.status = "当前终端不可用"
		return
	}
	if q.ProfileID != 0 && q.ProfileID != ts.profileID {
		a.quickCmdOpen = false
		a.status = "这条快捷命令属于其他服务器"
		return
	}

	a.quickCmdOpen = false
	ts.Term.Paste(q.Command)
	ts.focusRequest = true
	a.status = "已填入快捷命令：" + q.Name
}

func (a *App) openNewQuickCommand() {
	current, hasCurrent := a.quickCommandProfile()
	a.quickCmdOpen = false
	a.quickCmdEditOpen = true
	a.quickCmdID = 0
	a.quickCmdProfileID = 0
	a.quickCmdScope = quickCmdScopeGlobal
	if hasCurrent {
		a.quickCmdProfileID = current.ID
		a.quickCmdScope = quickCmdScopeServer
	}
	a.quickCmdName = ""
	a.quickCmdCommand = ""
	a.quickCmdErr = ""
}

func (a *App) openEditQuickCommand(q store.QuickCommand) {
	current, hasCurrent := a.quickCommandProfile()
	a.quickCmdOpen = false
	a.quickCmdEditOpen = true
	a.quickCmdID = q.ID
	a.quickCmdProfileID = 0
	if hasCurrent {
		a.quickCmdProfileID = current.ID
	}
	a.quickCmdScope = quickCmdScopeGlobal
	if q.ProfileID != 0 {
		a.quickCmdProfileID = q.ProfileID
		a.quickCmdScope = quickCmdScopeServer
	}
	a.quickCmdName = q.Name
	a.quickCmdCommand = q.Command
	a.quickCmdErr = ""
}

func (a *App) saveQuickCommandEditor() {
	name := strings.TrimSpace(a.quickCmdName)
	command := strings.TrimSpace(a.quickCmdCommand)
	switch {
	case name == "":
		a.quickCmdErr = "请填写名称"
		return
	case command == "":
		a.quickCmdErr = "请填写命令"
		return
	case strings.ContainsAny(command, "\r\n"):
		a.quickCmdErr = "暂不支持多行命令"
		return
	}

	profileID := int64(0)
	if a.quickCmdScope == quickCmdScopeServer {
		profileID = a.quickCmdProfileID
		if profileID == 0 {
			a.quickCmdErr = "请先保存连接，再创建服务器快捷命令"
			return
		}
	}

	for _, q := range a.quickCommands {
		if q.ID != a.quickCmdID && q.ProfileID == profileID && strings.EqualFold(q.Name, name) {
			a.quickCmdErr = "当前范围已有同名快捷命令"
			return
		}
	}

	if a.store == nil {
		a.quickCmdErr = "会话库不可用"
		return
	}

	q := &store.QuickCommand{
		ID:        a.quickCmdID,
		ProfileID: profileID,
		Name:      name,
		Command:   command,
	}
	if err := a.store.SaveQuickCommand(a.ctx, q); err != nil {
		a.quickCmdErr = "保存失败: " + err.Error()
		return
	}

	action := "已更新快捷命令 "
	if a.quickCmdID == 0 {
		action = "已添加快捷命令 "
	}
	a.quickCmdEditOpen = false
	a.quickCmdErr = ""
	a.loadQuickCommands()
	a.status = action + name
}

func (a *App) deleteQuickCommand(q store.QuickCommand) {
	res, _ := mygo.Dialog.Message(mygo.MessageOptions{
		Type:               mygo.MessageWarning,
		Message:            "删除快捷命令「" + q.Name + "」？",
		Buttons:            []string{"取消", "删除"},
		DestructiveButtons: []int{1},
	})
	if res.Button != 1 {
		return
	}
	if a.store == nil {
		a.status = "会话库不可用"
		return
	}
	if err := a.store.DeleteQuickCommand(a.ctx, q.ID); err != nil {
		a.status = "删除快捷命令失败: " + err.Error()
		return
	}
	a.loadQuickCommands()
	a.status = "已删除快捷命令 " + q.Name
}

func (a *App) quickCommandPopover(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	if ts == nil || !a.quickCmdAnchorSet {
		a.quickCmdOpen = false
		return
	}
	server, global := a.quickCommandGroups(ts.profileID)
	total := len(server) + len(global)

	ui.PopoverBase(c, a.quickCmdAnchor, &a.quickCmdOpen, func(panel ui.Element) {
		panel.Width(390).MaxHeight(470).Padding(10).Background(colPanel).
			Border(1, colLine2).Radius(10).
			Shadow(0, 18, 46, 0, ui.Hex("#000000").Alpha(0.38)).
			AttachTo(a.quickCmdAnchor, ui.AnchorBottomRight, ui.AnchorTopRight).
			MarginY(6)

		ui.Column(c).Gap(8).Children(func() {
			ui.Row(c).Height(28).Gap(7).AlignItems(ui.Center).Children(func() {
				iconEl(c, iconCommand, 13, colCyan)
				ui.Text(c, "快捷命令").FontSize(11.5).FontWeight(650).NoWrap()
				ui.Text(c, fmt.Sprintf("%d 条", total)).FontSize(8.8).
					Font("Cascadia Code").TextColor(colMuted2).NoWrap()
				ui.Text(c, "").Grow(1)
				if accentAction(c, "quick-cmd-new", "新建", iconPlus).Height(26).Clicked() {
					a.openNewQuickCommand()
				}
			})

			if total == 0 {
				ui.Column(c).FillWidth().Padding(22, 18).Center().Gap(6).Children(func() {
					iconEl(c, iconCommand, 22, colMuted2)
					ui.Text(c, "还没有快捷命令").FontSize(10.5).TextColor(colMuted)
					ui.Text(c, "点击“新建”添加常用目录或日志命令").FontSize(9).TextColor(colMuted2)
				})
			} else {
				ui.Scroll(c).MaxHeight(330).Scrollbars(false).Children(func() {
					ui.Column(c).FillWidth().Gap(7).Children(func() {
						if len(server) > 0 {
							a.quickCommandSection(c, t, "当前服务器 · "+shortLabel(a.quickCommandProfileName(ts.profileID), 24), server)
						}
						if len(global) > 0 {
							if len(server) > 0 {
								hairline(c)
							}
							a.quickCommandSection(c, t, "全局 · 所有服务器", global)
						}
					})
				})
			}

			ui.Box(c).FillWidth().Height(1).Background(colLine)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				iconEl(c, iconTerm, 10, colMuted2)
				ui.Text(c, "点击只填入终端，按 Enter 执行").FontSize(8.6).TextColor(colMuted2)
			})
		})
	})
}

func (a *App) quickCommandSection(c *ui.Context, t *ui.Theme, label string, commands []store.QuickCommand) {
	ui.Row(c).Padding(2, 2).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(8.8).FontWeight(650).LetterSpacing(0.5).
			TextColor(colMuted2).NoWrap()
	})
	for _, q := range commands {
		q := q
		var actioned bool
		row := ui.ButtonBase(c.Key(fmt.Sprintf("quick-cmd-%d", q.ID))).
			FillWidth().Height(48).Padding(7, 8).Gap(6).Radius(7).
			AlignItems(ui.Center).Label("快捷命令 " + q.Name).Tooltip(q.Command)
		if row.Hovered() {
			row.Background(t.SurfaceHover)
		}
		row.Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, shortLabel(q.Name, 28)).FontSize(10.8).FontWeight(600).
					TextColor(colText).NoWrap()
				ui.Text(c, shortLabel(q.Command, 44)).FontSize(8.8).
					Font("Cascadia Code").TextColor(colMuted).NoWrap()
			})

			edit := ui.ButtonBase(c.Key(fmt.Sprintf("quick-cmd-edit-%d", q.ID))).
				Size(22, 22).Radius(5).Center().Tooltip("编辑快捷命令").
				Label("编辑快捷命令 " + q.Name)
			if edit.Hovered() {
				edit.Background(colPanel2)
			}
			edit.Children(func() { iconEl(c, iconEdit, 11, colMuted) })
			if edit.Clicked() {
				actioned = true
				a.openEditQuickCommand(q)
			}

			del := ui.ButtonBase(c.Key(fmt.Sprintf("quick-cmd-delete-%d", q.ID))).
				Size(22, 22).Radius(5).Center().Tooltip("删除快捷命令").
				Label("删除快捷命令 " + q.Name)
			if del.Hovered() {
				del.Background(colRed.Alpha(0.1))
			}
			del.Children(func() {
				iconEl(c, iconTrash, 11, condColor(del.Hovered(), colRed, colMuted2))
			})
			if del.Clicked() {
				actioned = true
				a.deleteQuickCommand(q)
			}
		})
		if row.Clicked() && !actioned {
			a.runQuickCommand(q)
		}
	}
}

func (a *App) quickCommandModal(c *ui.Context, t *ui.Theme) {
	if !a.quickCmdEditOpen {
		return
	}
	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}
	title := "新建快捷命令"
	if a.quickCmdID != 0 {
		title = "编辑快捷命令"
	}
	serverName := a.quickCommandProfileName(a.quickCmdProfileID)
	hasServer := a.quickCmdProfileID != 0

	ui.DialogBase(c, &a.quickCmdEditOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(fitDialogWidth(c, 560)).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Row(c).Height(62).Shrink(0).Padding(0, 18).Gap(11).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
				ui.Box(c).Size(32, 32).Radius(8).
					Background(colCyan.Alpha(0.1)).Border(1, colCyan.Alpha(0.26)).Center().Children(func() {
					iconEl(c, iconCommand, 15, colCyan)
				})
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, title).FontSize(14.5).FontWeight(650)
					ui.Text(c, "命令会填入终端，由你按 Enter 执行").FontSize(9.5).TextColor(colMuted).NoWrap()
				})
				close := ui.ButtonBase(c.Key("quick-cmd-close")).Size(26, 26).
					Radius(7).Center().Tooltip("关闭").Label("关闭快捷命令")
				if close.Hovered() {
					close.Background(t.SurfaceHover)
				}
				close.Children(func() { iconEl(c, iconClose, 12, colMuted) })
				if close.Clicked() {
					a.quickCmdEditOpen = false
				}
			})

			ui.Column(c).Padding(16, 18).Gap(11).Children(func() {
				fieldColumn(c, "名称", func() {
					name := ui.TextInput(c.Key("quick-cmd-name"), &a.quickCmdName).
						FillWidth().Height(32).FontSize(11.5).
						Placeholder("例如：进入 jt808 项目").AutoFocus()
					if name.Submitted() {
						a.saveQuickCommandEditor()
					}
				})
				fieldColumn(c, "命令", func() {
					command := ui.TextInput(c.Key("quick-cmd-command"), &a.quickCmdCommand).
						FillWidth().Height(32).Font("Cascadia Code").FontSize(10.5).
						Placeholder("例如：cd /projects/jt808 && tail -f logs/app.log")
					if command.Submitted() {
						a.saveQuickCommandEditor()
					}
				})

				ui.Column(c).Gap(5).Children(func() {
					ui.Text(c, "作用范围").FontSize(9.5).FontWeight(600).TextColor(colMuted)
					if hasServer {
						ui.Tabs(c, &a.quickCmdScope, "当前服务器", "全局")
						scopeText := "所有已保存服务器都可以使用"
						if a.quickCmdScope == quickCmdScopeServer {
							scopeText = "当前服务器：" + serverName
						}
						ui.Text(c, scopeText).
							FontSize(8.8).TextColor(colMuted2).NoWrap()
					} else {
						ui.Text(c, "全局 · 所有服务器").FontSize(10.5).TextColor(colText2)
						ui.Text(c, "保存连接后，可以为单台服务器创建快捷命令").FontSize(8.8).TextColor(colMuted2)
					}
				})

				if a.quickCmdErr != "" {
					ui.Box(c).FillWidth().Padding(8, 10).Radius(7).
						Background(colRed.Alpha(0.08)).Border(1, colRed.Alpha(0.2)).Children(func() {
						ui.Text(c, a.quickCmdErr).FontSize(9.3).TextColor(colRed).LineHeight(1.4)
					})
				}
			})

			ui.Row(c).Height(56).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
				ui.Text(c, "暂不支持多行命令").FontSize(8.8).TextColor(colMuted2).NoWrap()
				ui.Text(c, "").Grow(1)
				if ghostAction(c, t, "quick-cmd-cancel", "取消", nil).Clicked() {
					a.quickCmdEditOpen = false
				}
				if primaryAction(c, "quick-cmd-save", "保存", iconCheck).Clicked() {
					a.saveQuickCommandEditor()
				}
			})
		})
	})
}

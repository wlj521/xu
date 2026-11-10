package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

type dockTab int

const (
	dockFiles dockTab = iota
	dockTransfers
	dockTunnels
	dockCommands
)

func (a *App) dockShell(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Height(tabStripHeight).Shrink(0).Background(colPanelTop).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine2).Children(func() {
		a.dockHead(c, t)
	})
}

// dockShellExpanded is the dock inside the draggable split above it: it fills
// the room the split gives it instead of a fixed height.
func (a *App) dockShellExpanded(c *ui.Context, t *ui.Theme) {
	ui.Column(c).FillHeight().Background(colPanelTop).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine2).Children(func() {
		a.dockHead(c, t)
		ui.Column(c).Grow(1).FillHeight().Children(func() {
			a.dockPanel(c, t)
		})
	})
}

func (a *App) dockHead(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	transferCount := len(a.transferLog)
	if a.transfer != nil {
		transferCount++
	}
	tunnelCount := 0
	if ts != nil {
		tunnelCount = len(a.forwards[ts.ID])
	}

	ui.Row(c).Height(tabStripHeight).Shrink(0).Padding(0, 9).Gap(4).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		tabs := []struct {
			tab   dockTab
			label string
			icon  *ui.SVG
			count int
		}{
			{dockFiles, "文件", iconFolder, -1},
			{dockTransfers, "传输", iconUpload, transferCount},
			{dockTunnels, "隧道", iconLink, tunnelCount},
			{dockCommands, "命令", iconCommand, -1},
		}
		for _, item := range tabs {
			item := item
			active := dockTab(a.bottomFileTab) == item.tab
			b := ui.ButtonBase(c.Key(fmt.Sprintf("dock-tab-%d", item.tab))).
				Height(tabPillHeight).Padding(0, 11).Radius(tabPillRadius).Gap(6).AlignItems(ui.Center)
			b = styleTabPill(b, active)
			b.Children(func() {
				iconEl(c, item.icon, 13, condColor(active, colCyan, colMuted))
				ui.Text(c, item.label).FontSize(10.8).FontWeight(condWeight(active, 600, 500)).
					TextColor(condColor(active, colText, colText2)).NoWrap()
				if item.count > 0 {
					countBG := colAccentSoft
					if active {
						countBG = colCyan.Alpha(0.16)
					}
					ui.Box(c).Height(14).Padding(0, 4).Radius(7).
						Background(countBG).Center().Children(func() {
						ui.Text(c, fmt.Sprintf("%d", item.count)).FontSize(8).
							Font("Cascadia Code").TextColor(colCyan).NoWrap()
					})
				}
			})
			if b.Clicked() {
				a.setDockTab(item.tab)
			}
		}

		ui.Text(c, "").Grow(1)

		if ts != nil {
			statusDot(c, condColor(!ts.exited, colGreen, colRed), 5)
			label := ts.Title
			if p := a.profileForTerm(ts); p != nil && p.Name != "" {
				label = p.Name
			}
			if ts.local {
				label = "本地终端"
			}
			ui.Text(c, shortLabel(label, 20)).FontSize(8.5).Font("Cascadia Code").
				TextColor(colMuted).NoWrap()
			if b := a.browsers[ts.ID]; b != nil {
				path := b.remoteDir
				if ts.local {
					path = b.localDir
				}
				if path != "" {
					ui.Text(c, shortLabel(path, 24)).FontSize(8.5).Font("Cascadia Code").
						TextColor(colMuted2).NoWrap()
				}
			}
		}

		chevron := iconChevron
		tip := "收起底部面板（Ctrl J）"
		if !a.bottomFiles {
			chevron = iconChevronUp
			tip = "展开底部面板（Ctrl J）"
		}
		collapse := ui.ButtonBase(c.Key("dock-toggle")).Size(22, 22).Radius(5).Center().
			Tooltip(tip).Label(tip)
		if collapse.Hovered() {
			collapse.Background(colPanel2)
		}
		collapse.Children(func() {
			iconEl(c, chevron, 11.5, colMuted)
		})
		if collapse.Clicked() {
			a.toggleDock()
		}
	})
}

func (a *App) dockPanel(c *ui.Context, t *ui.Theme) {
	switch dockTab(a.bottomFileTab) {
	case dockTransfers:
		a.dockTransfersPanel(c, t)
	case dockTunnels:
		a.dockTunnelsPanel(c, t)
	case dockCommands:
		a.dockCommandsPanel(c, t)
	default:
		a.dockFilesPanel(c, t)
	}
}

func (a *App) dockFilesPanel(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	if ts == nil {
		a.dockNotice(c, "未连接", "打开一个会话后可以浏览远端文件")
		return
	}
	var b *fileBrowser
	if ts.local {
		b = a.ensureLocalBrowser(ts)
	} else {
		var err error
		b, err = a.ensureFileBrowser(ts)
		if err != nil {
			a.dockNotice(c, "打开 SFTP 失败", err.Error())
			return
		}
	}

	ui.Column(c).Grow(1).FillHeight().Padding(0, 0, 6, 0).Gap(4).Children(func() {
		if a.promptOpen {
			a.promptBar(c)
		}
		if a.fileErr != "" {
			ui.Text(c, shortLabel(a.fileErr, 120)).FontSize(9).TextColor(colRed).NoWrap()
		}
		if ts.local {
			a.localPane(c, b, t)
			return
		}
		// SSH sessions show the remote browser: a full-width breadcrumb
		// above the directory tree and file table.
		a.bottomFilesPane(c, t, b)
	})
}

func (a *App) dockNotice(c *ui.Context, title, sub string) {
	ui.Column(c).Grow(1).Fill().Center().Gap(6).Padding(20).Children(func() {
		iconEl(c, iconFolder, 24, colMuted2)
		ui.Text(c, title).FontSize(11).FontWeight(600).TextColor(colMuted)
		ui.Text(c, sub).FontSize(9.5).TextColor(colMuted2).TextAlign(ui.Center)
	})
}

func (a *App) ensureDockBrowser() {
	ts := a.activeTerminal()
	if ts == nil {
		return
	}
	if ts.local {
		a.ensureLocalBrowser(ts)
		return
	}
	if _, err := a.ensureFileBrowser(ts); err != nil {
		a.fileErr = "打开 SFTP 失败: " + err.Error()
	}
}

func (a *App) setDockTab(tab dockTab) {
	if a.bottomFileTab < 0 || a.bottomFileTab > int(dockCommands) {
		a.bottomFileTab = int(dockFiles)
	}
	a.bottomFileTab = int(tab)
	a.bottomFiles = true
	a.formOpen = false
	a.pickerOpen = false
	a.settingsOpen = false
	a.fileErr = ""

	switch tab {
	case dockFiles:
		a.ensureDockBrowser()
	case dockTunnels:
		a.reloadDockForwards()
	case dockCommands:
		a.prepareBatchChoices()
	}
}

func (a *App) toggleDock() {
	if len(a.terms) == 0 {
		a.status = "请先打开一个终端"
		return
	}
	if a.bottomFiles {
		a.bottomFiles = false
		return
	}
	a.bottomFiles = true
	if dockTab(a.bottomFileTab) == dockFiles {
		a.ensureDockBrowser()
	}
}

func (a *App) addTransferLog(rec transferRecord) {
	a.transferLog = append([]transferRecord{rec}, a.transferLog...)
	if len(a.transferLog) > 12 {
		a.transferLog = a.transferLog[:12]
	}
}

func (a *App) dockTransfersPanel(c *ui.Context, t *ui.Theme) {
	ui.Scroll(c).Grow(1).Padding(10).Children(func() {
		ui.Column(c).Gap(7).Children(func() {
			if a.transfer == nil && len(a.transferLog) == 0 {
				ui.Column(c).FillWidth().Padding(20, 12).Center().Gap(6).Children(func() {
					iconEl(c, iconUpload, 22, colMuted2)
					ui.Text(c, "暂无传输任务").FontSize(11).TextColor(colMuted)
					ui.Text(c, "在文件面板里上传或下载文件，进度会显示在这里").FontSize(9.5).TextColor(colMuted2)
				})
				return
			}

			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "传输队列").FontSize(10).FontWeight(600).TextColor(colMuted)
				ui.Text(c, "").Grow(1)
				if len(a.transferLog) > 0 && quietAction(c, t, "transfer-clear", "清空记录", nil, false).Clicked() {
					a.transferLog = nil
				}
			})

			if a.transfer != nil {
				tr := a.transfer
				pct := 0
				if tr.total > 0 {
					pct = int(tr.done * 100 / tr.total)
				}
				ui.Box(c).FillWidth().Padding(9, 10).Radius(7).Background(colPanel2.Alpha(0.55)).
					Border(1, colLine).Children(func() {
					ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
						iconEl(c, transferIcon(tr.label), 13, colCyan)
						ui.Column(c).Grow(1).Gap(4).Children(func() {
							ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
								ui.Text(c, shortLabel(tr.label, 42)).FontSize(10).TextColor(colText).NoWrap()
								ui.Text(c, "").Grow(1)
								ui.Text(c, fmt.Sprintf("%d%%", pct)).FontSize(9).Font("Cascadia Code").TextColor(colCyan)
							})
							if tr.target != "" {
								ui.Text(c, "保存到 "+shortLabel(tr.target, 86)).FontSize(8.3).
									Font("Cascadia Code").TextColor(colMuted2).NoWrap()
							}
							ui.Progress(c, float64(pct)/100).FillWidth().Height(3)
						})
						if ghostAction(c, t, "transfer-cancel", "取消", nil).Clicked() && tr.cancel != nil {
							tr.cancel()
						}
					})
				})
			}

			for i, rec := range a.transferLog {
				rec := rec
				ui.Row(c).MinHeight(30).Gap(9).AlignItems(ui.Center).Children(func() {
					iconEl(c, transferIcon(rec.Label), 12, condColor(rec.Err == "", colGreen, colRed))
					ui.Column(c).Grow(1).Gap(1).Children(func() {
						ui.Text(c, shortLabel(rec.Label, 44)).FontSize(9.5).TextColor(colMuted).NoWrap()
						if rec.Target != "" {
							ui.Text(c, "保存到 "+shortLabel(rec.Target, 86)).FontSize(8.2).
								Font("Cascadia Code").TextColor(colMuted2).NoWrap()
						}
					})
					if rec.Total > 0 {
						ui.Text(c, humanSize(rec.Done)+" / "+humanSize(rec.Total)).FontSize(8.5).
							Font("Cascadia Code").TextColor(colMuted2).NoWrap()
					}
					status := "完成"
					if rec.Canceled {
						status = "已取消"
					} else if rec.Err != "" {
						status = "失败"
					}
					ui.Text(c, status).FontSize(8.5).TextColor(condColor(rec.Err == "", colGreen, colRed)).NoWrap()
					_ = i
				})
			}
		})
	})
}

func transferIcon(label string) *ui.SVG {
	if strings.Contains(label, "下载") {
		return iconDownload
	}
	return iconUpload
}

func (a *App) reloadDockForwards() {
	ts := a.activeTerminal()
	if ts == nil || ts.local {
		return
	}
	a.savedForwards = nil
	if a.store != nil && ts.profileID != 0 {
		if rules, err := a.store.Forwards(a.ctx, ts.profileID); err == nil {
			a.savedForwards = rules
		}
	}
}

func (a *App) dockTunnelsPanel(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	if ts == nil {
		a.dockNotice(c, "未连接", "打开 SSH 会话后可以创建端口转发")
		return
	}
	if ts.local {
		a.dockNotice(c, "本地终端不支持隧道", "打开一台 SSH 服务器后再配置转发")
		return
	}
	list := a.forwards[ts.ID]

	ui.Scroll(c).Grow(1).Padding(10).Children(func() {
		ui.Column(c).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "新建隧道").FontSize(10).FontWeight(600).TextColor(colMuted)
				ui.Tabs(c, &a.forwardKind, "本地", "远程", "SOCKS5")
				ui.Text(c, "").Grow(1)
				if primaryAction(c, "dock-forward-start", "启动隧道", iconLink).Clicked() {
					a.startForward(ts)
				}
			})

			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.forwardListen).Width(240).Height(28).Font("Cascadia Code").
					FontSize(9.5).Placeholder("监听地址 127.0.0.1:8080")
				if a.forwardKind != 2 {
					ui.TextInput(c, &a.forwardTarget).Width(250).Height(28).Font("Cascadia Code").
						FontSize(9.5).Placeholder("目标地址 10.0.0.5:3306")
				}
				ui.Checkbox(c, &a.forwardSave, "保存规则")
				ui.Checkbox(c, &a.forwardAuto, "自动启动")
			})

			if a.forwardErr != "" {
				ui.Text(c, shortLabel(a.forwardErr, 90)).FontSize(9).TextColor(colRed)
			}

			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "运行中的隧道").FontSize(10).FontWeight(600).TextColor(colMuted)
				ui.Text(c, "").Grow(1)
				miniTag(c, fmt.Sprintf("%d 条", len(list)), condColor(len(list) > 0, colGreen, colMuted))
			})
			if len(list) == 0 {
				ui.Text(c, "还没有运行中的隧道").FontSize(9.5).TextColor(colMuted2)
			}
			for i, e := range list {
				i, e := i, e
				ui.Row(c).Height(30).Gap(8).AlignItems(ui.Center).Children(func() {
					miniTag(c, forwardKindName(e.kind), colCyan)
					ui.Text(c, shortLabel(forwardRowText(e), 46)).Grow(1).FontSize(9.5).
						Font("Cascadia Code").TextColor(colMuted).NoWrap()
					if ghostAction(c, t, fmt.Sprintf("dock-forward-stop-%d", i), "停止", nil).Clicked() {
						a.stopForward(ts.ID, i)
					}
				})
			}

			if len(a.savedForwards) > 0 {
				ui.Text(c, "已保存的规则").FontSize(10).FontWeight(600).TextColor(colMuted)
				for i, r := range a.savedForwards {
					i, r := i, r
					ui.Row(c).Height(28).Gap(8).AlignItems(ui.Center).Children(func() {
						miniTag(c, forwardKindName(r.Kind), colBlue)
						ui.Text(c, shortLabel(savedForwardText(r), 46)).Grow(1).FontSize(9.5).
							Font("Cascadia Code").TextColor(colMuted).NoWrap()
						if r.AutoStart {
							miniTag(c, "自动", colGreen)
						}
						if quietAction(c, t, fmt.Sprintf("dock-forward-run-%d", i), "启动", nil, false).Clicked() {
							a.startForwardSpec(ts, r.Kind, r.Listen, r.Target, false)
						}
						if quietAction(c, t, fmt.Sprintf("dock-forward-del-%d", i), "删除", nil, false).Clicked() {
							a.deleteSavedForward(r.ID, ts)
						}
					})
				}
			}
		})
	})
}

func (a *App) prepareBatchChoices() {
	if len(a.batchChoices) > 0 {
		return
	}
	a.batchChoices = make([]batchChoice, 0, len(a.profiles))
	for _, p := range a.profiles {
		a.batchChoices = append(a.batchChoices, batchChoice{id: p.ID, name: p.Name, selected: true})
	}
	a.batchState.Selected = &a.batchSel
}

func (a *App) dockCommandsPanel(c *ui.Context, t *ui.Theme) {
	if len(a.profiles) == 0 {
		a.dockNotice(c, "还没有保存的连接", "先保存连接，再向多台服务器批量执行命令")
		return
	}
	a.prepareBatchChoices()

	ui.Column(c).Grow(1).Padding(8, 10).Gap(6).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "批量命令").FontSize(10).FontWeight(600).TextColor(colMuted)
			target := quietAction(c, t, "dock-batch-target",
				fmt.Sprintf("目标 %d / %d", a.selectedBatchCount(), len(a.batchChoices)), iconServer, false)
			target.Menu(func(m *ui.Menu) {
				for i := range a.batchChoices {
					i := i
					ch := a.batchChoices[i]
					label := ch.name
					if ch.selected {
						label = "✓ " + label
					}
					if m.Item(label).Chosen() {
						a.batchChoices[i].selected = !a.batchChoices[i].selected
					}
				}
				m.Separator()
				if m.Item("全选").Chosen() {
					for i := range a.batchChoices {
						a.batchChoices[i].selected = true
					}
				}
				if m.Item("全不选").Chosen() {
					for i := range a.batchChoices {
						a.batchChoices[i].selected = false
					}
				}
			})
			ui.Text(c, "").Grow(1)
			if a.batchRunning {
				if dangerAction(c, "dock-batch-cancel", "取消", iconClose).Clicked() {
					a.cancelBatch()
				}
			} else if primaryAction(c, "dock-batch-run", "执行", iconCommand).Clicked() {
				a.runBatch()
			}
		})

		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			input := ui.TextInput(c, &a.batchCmd).Grow(1).Height(30).Font("Cascadia Code").
				FontSize(10).Placeholder("输入要在所有目标会话中执行的命令，回车执行…")
			if input.Submitted() && !a.batchRunning {
				a.runBatch()
			}
			ui.Text(c, fmt.Sprintf("%d 台", a.selectedBatchCount())).FontSize(8.5).
				Font("Cascadia Code").TextColor(colMuted2).NoWrap()
		})

		if a.batchErr != "" {
			ui.Text(c, shortLabel(a.batchErr, 90)).FontSize(9).TextColor(colRed)
		}

		ui.Box(c).FillWidth().Grow(1).Padding(8, 9).Radius(6).Background(colTermBg).
			Border(1, colTermLine).Children(func() {
			ui.Scroll(c).Grow(1).Children(func() {
				out := a.batchOutput()
				if out == "" {
					ui.Text(c, "执行结果会显示在这里").FontSize(9.5).TextColor(colMuted2)
				} else {
					ui.Text(c, out).Font("Cascadia Code").FontSize(9.5).TextColor(colMuted).
						LineHeight(1.45)
				}
			})
		})
	})
}

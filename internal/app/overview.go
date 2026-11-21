package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"xu/internal/monitorx"
)

type serverOverviewRow struct {
	tabIndex   int
	tabID      int64
	termID     int64
	title      string
	endpoint   string
	sample     *monitorx.Sample
	online     bool
	monitorErr string
	alerts     []monitorAlertNotice
}

func (a *App) openServerOverview() {
	if a.finding {
		a.closeFind()
	}
	a.overviewOpen = true
	for _, ts := range a.terms {
		if ts == nil || ts.local || ts.exited || ts.client == nil {
			continue
		}
		a.startMonitorBackground(ts)
	}
}

func (a *App) serverOverviewRows() []serverOverviewRow {
	rows := make([]serverOverviewRow, 0, len(a.tabs))
	for i, tab := range a.tabs {
		if tab.kind != tabTerminal || tab.term == nil || tab.term.local {
			continue
		}
		ts := tab.term
		row := serverOverviewRow{
			tabIndex: i,
			tabID:    tab.id,
			termID:   ts.ID,
			title:    a.tabDisplayTitle(tab),
			endpoint: fmt.Sprintf("%s@%s:%d", ts.profile.Username, ts.profile.Host, ts.profile.Port),
			online:   !ts.exited,
			alerts:   a.activeMonitorAlertsForTerm(ts.ID),
		}
		if ms := a.monitors[ts.ID]; ms != nil {
			row.monitorErr = ms.err
			if len(ms.samples) > 0 {
				latest := ms.samples[len(ms.samples)-1]
				row.sample = &latest
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func (a *App) overviewAlertColor(row serverOverviewRow) ui.Color {
	for _, alert := range row.alerts {
		if alert.Critical {
			return colRed
		}
	}
	if len(row.alerts) > 0 {
		return colAmber
	}
	return colGreen
}

func (a *App) rowHasAlert(row serverOverviewRow, kind string) bool {
	for _, alert := range row.alerts {
		if alert.Kind == kind {
			return true
		}
	}
	return false
}

func (a *App) serverOverviewModal(c *ui.Context, t *ui.Theme) {
	rows := a.serverOverviewRows()
	online := 0
	alerts := 0
	for _, row := range rows {
		if row.online {
			online++
		}
		if len(row.alerts) > 0 {
			alerts++
		}
	}

	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}
	w, h := fitDialog(c, 940, 680)
	ui.DialogBase(c, &a.overviewOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(w).Height(h).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Row(c).Height(64).Shrink(0).Padding(0, 18).Gap(10).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
				ui.Box(c).Size(30, 30).Radius(8).
					Background(colCyan.Alpha(0.08)).Border(1, colCyan.Alpha(0.22)).Center().Children(func() {
					iconEl(c, iconActivity, 15, colCyan)
				})
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, "服务器总览与告警").FontSize(14.5).FontWeight(650)
					ui.Text(c, fmt.Sprintf("%d 台在线 · %d 台告警 · 每 2 秒刷新", online, alerts)).
						FontSize(9.5).TextColor(condColor(alerts > 0, colAmber, colMuted)).NoWrap()
				})
				if iconButton(c, t, "overview-close", iconClose, "关闭").Clicked() {
					a.overviewOpen = false
				}
			})

			ui.Scroll(c).Grow(1).FillWidth().Padding(14, 16).Children(func() {
				ui.Column(c).FillWidth().Gap(9).Children(func() {
					if len(rows) == 0 {
						ui.Column(c).FillWidth().Padding(34, 28).Center().Gap(8).Children(func() {
							iconEl(c, iconServer, 28, colMuted2)
							ui.Text(c, "没有已连接的 SSH 服务器").FontSize(12).FontWeight(600)
							ui.Text(c, "打开 SSH 会话后会自动开始采集 CPU、内存、磁盘和延迟").
								FontSize(9.8).TextColor(colMuted)
							if ghostAction(c, t, "overview-connect", "连接管理", iconConnections).Clicked() {
								a.overviewOpen = false
								a.openConnectionManager()
							}
						})
						return
					}
					for _, row := range rows {
						row := row
						a.serverOverviewRow(c, t, row)
					}
				})
			})

			ui.Row(c).Height(54).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
				iconEl(c, iconAlert, 11, colMuted2)
				ui.Text(c, "阈值与桌面通知可在「设置 → 连接 → 监控告警」中调整").
					FontSize(8.8).TextColor(colMuted2).NoWrap()
				ui.Text(c, "").Grow(1)
				if ghostAction(c, t, "overview-settings", "告警设置", iconGear).Clicked() {
					a.overviewOpen = false
					a.openSettings()
					a.settingsSection = 3
				}
				if ghostAction(c, t, "overview-done", "关闭", nil).Clicked() {
					a.overviewOpen = false
				}
			})
		})
	})
}

func (a *App) serverOverviewRow(c *ui.Context, t *ui.Theme, row serverOverviewRow) {
	alertColor := a.overviewAlertColor(row)
	bg := colPanel2.Alpha(0.55)
	border := colLine
	if len(row.alerts) > 0 {
		bg = alertColor.Alpha(0.055)
		border = alertColor.Alpha(0.28)
	}
	ui.Box(c).FillWidth().Padding(11, 12).Radius(10).
		Background(bg).Border(1, border).Children(func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			statusDot(c, condColor(row.online, alertColor, colRed), 7)
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
					ui.Text(c, shortLabel(row.title, 26)).FontSize(11.5).FontWeight(650).TextColor(colText).NoWrap()
					if !row.online {
						miniTag(c, "已断开", colRed)
					} else if len(row.alerts) > 0 {
						miniTag(c, fmt.Sprintf("%d 个告警", len(row.alerts)), alertColor)
					} else if row.sample != nil {
						miniTag(c, "正常", colGreen)
					} else {
						miniTag(c, "采集中", colCyan)
					}
				})
				ui.Text(c, shortLabel(row.endpoint, 58)).FontSize(8.6).
					Font("Cascadia Code").TextColor(colMuted).NoWrap()
			})
			if row.sample != nil && row.online {
				disk, _, diskTotal := monitorDiskUsage(*row.sample)
				a.overviewMetric(c, "CPU", fmt.Sprintf("%.0f%%", row.sample.CPU),
					fmt.Sprintf("%d 核", max(1, row.sample.CPUs)),
					condColor(a.rowHasAlert(row, monitorAlertCPU), colRed, colCyan))
				a.overviewMetric(c, "内存", fmt.Sprintf("%.0f%%", percent(row.sample.MemUsed, row.sample.MemTotal)),
					humanSize(int64(row.sample.MemUsed)), condColor(a.rowHasAlert(row, monitorAlertMemory), colRed, colViolet))
				a.overviewMetric(c, "磁盘", fmt.Sprintf("%.0f%%", disk),
					humanSize(int64(diskTotal)), condColor(a.rowHasAlert(row, monitorAlertDisk), colRed, colAmber))
				latency := "—"
				if row.sample.LatencyMs > 0 {
					latency = fmt.Sprintf("%.0f ms", row.sample.LatencyMs)
				}
				a.overviewMetric(c, "延迟", latency, "SSH RTT",
					condColor(a.rowHasAlert(row, monitorAlertLatency), colRed, colGreen))
			} else if !row.online {
				ui.Text(c, "连接已断开，切回该标签可重连").FontSize(9.2).TextColor(colRed)
			} else if row.monitorErr != "" {
				ui.Text(c, "监控不可用").FontSize(9).TextColor(colRed)
			} else {
				ui.Text(c, "正在采集…").FontSize(9).TextColor(colMuted2)
			}
			label := "查看 " + row.title
			if ghostAction(c, t, fmt.Sprintf("overview-view-%d", row.tabID), "查看", iconChevronRight).Label(label).Clicked() {
				a.overviewPendingTab = row.tabID
			}
		})

		if len(row.alerts) > 0 {
			ui.Row(c).FillWidth().Gap(5).Padding(2, 0, 0, 0).Children(func() {
				for _, alert := range row.alerts[:min(3, len(row.alerts))] {
					chipColor := colAmber
					if alert.Critical {
						chipColor = colRed
					}
					ui.Box(c).Height(18).Padding(0, 6).Radius(4).
						Background(chipColor.Alpha(0.09)).Center().Children(func() {
						ui.Text(c, shortLabel(alert.Message, 46)).FontSize(8.1).
							TextColor(chipColor).NoWrap()
					})
				}
			})
		} else if row.sample != nil {
			ui.Row(c).Gap(7).Padding(2, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "更新于 "+row.sample.At.Format("15:04:05")).FontSize(8).
					Font("Cascadia Code").TextColor(colMuted2)
				if row.monitorErr != "" {
					ui.Text(c, shortLabel(row.monitorErr, 60)).FontSize(8).TextColor(colRed)
				}
			})
		}
	})
}

// finishServerOverviewNavigation runs after the dialog has closed in this
// frame. Switching tabs while an overlay was still up could leave the
// target tab hidden behind it for a frame, and the click looked lost.
func (a *App) finishServerOverviewNavigation() {
	if a.overviewPendingTab == 0 {
		return
	}
	tabID := a.overviewPendingTab
	a.overviewPendingTab = 0
	a.overviewOpen = false
	if idx := a.tabIndexByID(tabID); idx >= 0 {
		a.activateTabIndex(idx)
		if ts := a.activeTerminal(); ts != nil && !ts.local {
			a.startMonitorFor(ts, true)
			a.monitorOpen = true
		}
	}
}

func (a *App) overviewMetric(c *ui.Context, label, value, detail string, color ui.Color) {
	ui.Column(c).Width(82).Gap(2).Children(func() {
		ui.Text(c, label).FontSize(8).TextColor(colMuted2).NoWrap()
		ui.Text(c, value).FontSize(13).FontWeight(650).Font("Cascadia Code").TextColor(color).NoWrap()
		ui.Text(c, shortLabel(detail, 12)).FontSize(7.8).TextColor(colMuted2).NoWrap()
	})
}

func (a *App) monitorAlertBanner(c *ui.Context, t *ui.Theme) {
	count := a.activeMonitorAlertServerCount()
	if count == 0 {
		return
	}
	notice, _ := a.latestActiveMonitorAlert()
	color := colAmber
	if notice.Critical {
		color = colRed
	}
	ui.Row(c).Height(30).Shrink(0).Padding(0, 12).Gap(7).AlignItems(ui.Center).
		Background(color.Alpha(0.1)).BorderWidth(0, 0, 1, 0).
		BorderColor(color.Alpha(0.24)).Children(func() {
		iconEl(c, iconAlert, 12, color)
		ui.Text(c, fmt.Sprintf("%d 个服务器告警 · %s", count, shortLabel(notice.Message, 88))).
			FontSize(9).TextColor(color).NoWrap()
		ui.Text(c, "").Grow(1)
		ui.Text(c, "只在状态变化时提醒").FontSize(8.2).TextColor(colMuted2).NoWrap()
		if ghostAction(c, t, "monitor-alert-overview", "查看总览", iconActivity).Clicked() {
			a.openServerOverview()
		}
	})
}

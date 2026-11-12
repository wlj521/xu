package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"xu/internal/monitorx"
	"xu/internal/store"
)

func (a *App) shellInspector(c *ui.Context, t *ui.Theme) {
	// The same surface as the terminal pane, so the monitor reads as part of
	// the workspace; the outlined sections provide the structure.
	ui.Column(c).Width(monitorWidth(c)).FillHeight().Background(colTermBg).
		BorderWidth(0, 0, 0, 1).BorderColor(colLine).Children(func() {
		a.inspectorMonitorHead(c, t)
		a.inspectorContent(c, t)
	})
}

// toggleMonitor collapses or expands the left-hand server monitor.
func (a *App) toggleMonitor() {
	a.monitorCollapsed = !a.monitorCollapsed
	if a.monitorCollapsed {
		a.status = "服务器监控已收起"
	} else {
		a.status = "服务器监控已展开"
	}
	if !a.monitorCollapsed {
		if ts := a.activeTerminal(); ts != nil && !ts.local {
			a.startMonitorFor(ts, false)
		}
	}
}

func (a *App) inspectorMonitorHead(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	live := false
	sub := "未连接"
	host := ""
	if ts != nil {
		if ts.local {
			sub = "本机 · 本地终端"
		} else {
			sub = ts.profile.Host
			host = ts.profile.Host
			if ms := a.monitors[ts.ID]; ms != nil && len(ms.samples) > 0 {
				live = true
			}
		}
	} else if p, ok := a.activeConnectingProfile(); ok {
		// The session does not exist yet, but the target is already known.
		sub = p.Host
		host = p.Host
	}

	// The same surface and height as the tab strip next to it, so the top
	// chrome reads as one continuous band and both bottom hairlines meet.
	// The title is gone: the address already says which server this
	// monitor belongs to.
	ui.Row(c).Height(tabStripHeight).Shrink(0).Padding(0, 9).Gap(7).AlignItems(ui.Center).
		Background(colPanelTop).BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		if host != "" {
			// The server's address stays visible even when no metrics arrive
			// yet, and one click copies it.
			copyBtn := ui.ButtonBase(c.Key("inspector-copy-ip")).
				Height(23).Padding(0, 7).Radius(6).Gap(6).AlignItems(ui.Center)
			// A faint accent chip instead of a white pill: it sits on the
			// panel header without a contrasting plate behind it.
			copyBtn.Background(colCyan.Alpha(0.07)).Border(1, colCyan.Alpha(0.16))
			if copyBtn.Hovered() {
				copyBtn.Background(colCyan.Alpha(0.13))
				copyBtn.BorderColor(colCyan.Alpha(0.3))
			}
			copyBtn.Tooltip("复制 IP 地址").Label("复制 IP 地址 " + host)
			copyBtn.Children(func() {
				ui.Text(c, "IP").FontSize(8).FontWeight(650).LetterSpacing(0.4).
					TextColor(colCyan.Alpha(0.85)).NoWrap()
				ui.Text(c, shortLabel(host, 20)).FontSize(10).Font("Cascadia Code").
					TextColor(colText2).NoWrap()
				iconEl(c, iconCopy, 11, condColor(copyBtn.Hovered(), colCyan, colMuted2))
			})
			if copyBtn.Clicked() {
				c.WriteClipboard(host)
				a.status = "已复制 IP：" + host
				c.Toast("已复制 IP · " + host)
			}
		} else {
			ui.Text(c, sub).FontSize(9.2).Font("Cascadia Code").
				TextColor(colMuted2).NoWrap()
		}

		if live {
			ui.Box(c).Height(16).Padding(0, 5).Radius(8).Background(colGreen.Alpha(0.11)).
				Center().Children(func() {
				ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
					statusDot(c, colGreen, 4)
					ui.Text(c, "LIVE").FontSize(7.5).Font("Cascadia Code").LetterSpacing(0.6).
						TextColor(colGreen)
				})
			})
		}

		ui.Text(c, "").Grow(1)
		collapse := ui.ButtonBase(c.Key("inspector-collapse")).Size(24, 24).Radius(6).Center().
			Tooltip("收起监控（Ctrl B）").Label("收起监控")
		if collapse.Hovered() {
			collapse.Background(colPanel2)
		}
		collapse.Children(func() {
			iconEl(c, iconChevronLeft, 13, colMuted)
		})
		if collapse.Clicked() {
			a.toggleMonitor()
		}
	})
}

func (a *App) inspectorRail(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Width(44).FillHeight().Background(colTermBg).
		BorderWidth(0, 0, 0, 1).BorderColor(colLine).Padding(8, 5).Gap(2).
		AlignItems(ui.Center).Children(func() {
		expand := ui.ButtonBase(c.Key("inspector-expand")).Size(28, 28).Radius(7).Center().
			Tooltip("展开监控（Ctrl B）").Label("展开监控")
		if expand.Hovered() {
			expand.Background(t.SurfaceHover)
		}
		expand.Children(func() {
			iconEl(c, iconChevronRight, 14, colMuted)
		})
		if expand.Clicked() {
			a.toggleMonitor()
		}
		ui.Box(c).Height(4).Shrink(0)
		for _, r := range "服务器监控" {
			ui.Text(c, string(r)).FontSize(9.5).FontWeight(600).TextColor(colMuted2).NoWrap()
		}
		ui.Text(c, "").Grow(1)
		iconEl(c, iconActivity, 14, colMuted2)
	})
}

func (a *App) inspectorContent(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	if ts == nil {
		title := "未连接"
		sub := "点顶部 + 新建标签页，从快速连接里选择服务器"
		if p, ok := a.activeConnectingProfile(); ok {
			title = "正在连接 " + profileDisplayName(p)
			sub = "连接成功后在这里显示 CPU、内存、磁盘与网络"
		} else if a.quickTabActive() {
			title = "快速连接"
			sub = "点击连接卡片打开终端；连上后会在这里显示实时监控"
		}
		ui.Column(c).Grow(1).Fill().Center().Gap(8).Padding(20).Children(func() {
			iconEl(c, iconActivity, 24, colMuted2)
			ui.Text(c, title).FontSize(fsBody).FontWeight(600)
			ui.Text(c, sub).FontSize(10).TextColor(colMuted2).TextAlign(ui.Center)
		})
		return
	}
	if ts.local {
		ui.Column(c).Grow(1).Fill().Center().Gap(8).Padding(20).Children(func() {
			iconEl(c, iconTerm, 24, colMuted2)
			ui.Text(c, "本地终端").FontSize(11.5).FontWeight(600).TextColor(colText2)
			ui.Text(c, "本地会话没有可采集的远程指标。\n打开一个 SSH 连接即可查看 CPU、内存、磁盘与网络。").
				FontSize(9.5).TextColor(colMuted).TextAlign(ui.Center).LineHeight(1.7)
			if ghostAction(c, t, "inspector-manage", "连接管理", iconConnections).Clicked() {
				a.openConnectionManager()
			}
		})
		return
	}

	ms := a.monitors[ts.ID]
	if ms == nil || len(ms.samples) == 0 {
		inspectorNotice(c, t, "正在采集服务器指标…", "连接建立后每 2 秒刷新一次")
		return
	}
	latest := ms.samples[len(ms.samples)-1]
	profileName := ts.profile.Host
	if p := a.profileForTerm(ts); p != nil && p.Name != "" {
		profileName = p.Name
	}

	ui.Scroll(c).Grow(1).Padding(9).Scrollbars(false).Children(func() {
		ui.Column(c).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Stretch).Children(func() {
				// Both bars take the accent setting, softened toward the card
				// so the filled part reads as a tint rather than a block.
				a.inspectorStatCard(c, "CPU 使用率", latest.CPU, colCyan,
					fmt.Sprintf("%d 核", max(1, latest.CPUs)))
				a.inspectorStatCard(c, "内存", percent(latest.MemUsed, latest.MemTotal), colCyan,
					fmt.Sprintf("%s / %s", humanSize(int64(latest.MemUsed)), humanSize(int64(latest.MemTotal))))
			})

			if latest.SwapTotal > 0 {
				a.inspectorSwapCard(c, latest)
			}

			recent := recentSamples(ms.samples)

			inspectorCard(c, t, func() {
				iface := a.netIface[ts.ID]
				a.inspectorNetHead(c, t, ts, iface, latest)
				rx, tx := netRates(latest, iface)
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "↓ "+humanSize(int64(rx))+"/s").FontSize(10).FontWeight(600).
						Font("Cascadia Code").TextColor(colGreen)
					ui.Text(c, "↑ "+humanSize(int64(tx))+"/s").FontSize(10).FontWeight(600).
						Font("Cascadia Code").TextColor(colBlue)
					ui.Text(c, "").Grow(1)
				})
				a.inspectorNetBars(c, t, recent, iface)
				ui.Row(c).Children(func() {
					ui.Text(c, "-60s").FontSize(8).Font("Cascadia Code").TextColor(colMuted2)
					ui.Text(c, "").Grow(1)
					ui.Text(c, "now").FontSize(8).Font("Cascadia Code").TextColor(colMuted2)
				})
			})

			inspectorCard(c, t, func() {
				meta := "本机"
				if latest.LatencyMs > 0 {
					meta = fmt.Sprintf("本机 · %.0f ms", latest.LatencyMs)
				}
				inspectorCardHead(c, t, "延迟 · PING", meta, iconActivity)
				a.inspectorLatency(c, t, recent)
			})

			inspectorDisks(c, t, latest)
			inspectorProcesses(c, t, latest)

			inspectorCard(c, t, func() {
				inspectorCardHead(c, t, "主机", shortLabel(profileName, 18), iconServer)
				inspectorKV(c, t, "系统", shortLabel(condValue(latest.OS, "Linux"), 24))
				inspectorKV(c, t, "内核", shortLabel(condValue(latest.Kernel, "—"), 24))
				inspectorKV(c, t, "运行", condValue(formatUptime(latest.UptimeSec), "—"))
				inspectorKV(c, t, "负载", fmt.Sprintf("%.2f / %.2f / %.2f", latest.Load1, latest.Load5, latest.Load15))
				ui.Row(c).Gap(5).AlignItems(ui.Center).Padding(2, 0, 0, 0).Children(func() {
					iconEl(c, iconShield, 10, colGreen)
					ui.Text(c, "AES-256-GCM · 主机密钥已核对").FontSize(8.2).
						Font("Cascadia Code").TextColor(colMuted2)
				})
			})

		})
	})
}

// inspectorStatCard is one CPU/memory tile: the value on the label row and a
// horizontal bar underneath, which reads faster than a ring at this width.
func (a *App) inspectorStatCard(c *ui.Context, label string, pct float64, color ui.Color, sub string) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	ui.Column(c).Grow(1).Padding(9, 10).Border(1, colLine2).Radius(9).Gap(7).Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(9.5).TextColor(colMuted).NoWrap()
			ui.Text(c, "").Grow(1)
			ui.Text(c, fmt.Sprintf("%.0f%%", pct)).FontSize(14).FontWeight(650).
				Font("Cascadia Code").TextColor(color).NoWrap()
		})
		barFill(c, pct/100, color.Mix(colTermBg, 0.35), 6)
		ui.Text(c, sub).FontSize(8.2).Font("Cascadia Code").TextColor(colMuted2).NoWrap()
	})
}

func (a *App) inspectorHead(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()

	name, sub := "未连接", "选择会话开始"
	dot := colMuted2
	avatarIcon := iconServer
	avatarColor := colMuted2
	var tags []inspectorTag
	exited := false

	if ts != nil {
		dot = colGreen
		exited = ts.exited
		if exited {
			dot = colRed
		}
		avatarColor = colCyan
		if ts.local {
			avatarIcon = iconTerm
			name = "本地终端"
			sub = ts.Title
			tags = append(tags,
				inspectorTag{"LOCAL", colGreen},
				inspectorTag{"PowerShell", colBlue},
				inspectorTag{"本机", colMuted},
			)
		} else {
			name = shortLabel(ts.Title, 22)
			if p := a.profileForTerm(ts); p != nil && p.Name != "" {
				name = shortLabel(p.Name, 22)
			}
			sub = ts.profile.Host
			tags = append(tags,
				inspectorTag{"SSH", colGreen},
				inspectorTag{fmt.Sprintf(":%d", ts.profile.Port), colMuted},
			)
			if ms := a.monitors[ts.ID]; ms != nil && len(ms.samples) > 0 {
				s := ms.samples[len(ms.samples)-1]
				if osName := osTag(s.OS); osName != "" {
					tags = append(tags, inspectorTag{osName, colBlue})
				}
				if s.Arch != "" {
					tags = append(tags, inspectorTag{shortLabel(s.Arch, 10), colMuted})
				}
			}
		}
	}
	if exited {
		tags = append([]inspectorTag{{"已断开", colRed}}, tags...)
	}

	ui.Column(c).Padding(16, 16, 12, 16).Gap(11).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Row(c).Gap(11).AlignItems(ui.Center).Children(func() {
			avatar := ui.Box(c).Width(40).Height(40).Radius(11).
				Background(avatarColor.Alpha(0.09)).Border(1, avatarColor.Alpha(0.2)).Center()
			avatar.Children(func() { iconEl(c, avatarIcon, 18, avatarColor) })

			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
					statusDot(c, dot, 6)
					ui.Text(c, name).FontSize(13.5).FontWeight(650).TextColor(colText).NoWrap()
				})
				ui.Text(c, shortLabel(sub, 30)).FontSize(10).Font("Cascadia Code").
					TextColor(colMuted).NoWrap()
			})

		})

		if len(tags) > 0 {
			ui.Row(c).Gap(6).Children(func() {
				for _, tag := range tags {
					miniTag(c, tag.text, tag.color)
				}
			})
		}
	})
}

type inspectorTag struct {
	text  string
	color ui.Color
}

func (a *App) inspectorTabs(c *ui.Context, t *ui.Theme) {
	labels := []string{"概览", "进程", "磁盘"}
	ui.Row(c).Height(42).Shrink(0).Padding(0, 10).Gap(3).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		for i, label := range labels {
			i, label := i, label
			active := a.inspectorTab == i
			btn := ui.ButtonBase(c.Key(fmt.Sprintf("ins-tab-%d", i))).Height(30).Grow(1).Radius(8).Center()
			if active {
				btn.Background(colPanel2).Border(1, colLine2)
			} else if btn.Hovered() {
				btn.Background(t.SurfaceHover)
			}
			btn.Children(func() {
				ui.Text(c, label).FontSize(11).FontWeight(condWeight(active, 600, 500)).
					TextColor(condColor(active, colText, colMuted)).NoWrap()
			})
			if btn.Clicked() {
				a.inspectorTab = i
			}
		}
	})
}

func (a *App) inspectorServer(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	if ts == nil {
		if p, ok := a.activeConnectingProfile(); ok {
			ui.Column(c).Grow(1).Fill().Center().Gap(9).Padding(24).Children(func() {
				ui.Box(c).Size(56, 56).Radius(16).
					Background(colCyan.Alpha(0.08)).Border(1, colCyan.Alpha(0.22)).Center().Children(func() {
					iconEl(c, iconServer, 24, colCyan)
				})
				ui.Text(c, "正在连接 "+shortLabel(profileDisplayName(p), 22)).FontSize(fsBody).FontWeight(600)
				ui.Text(c, fmt.Sprintf("%s@%s:%d", p.Username, p.Host, p.Port)).
					FontSize(9.8).Font("Cascadia Code").TextColor(colMuted)
				ui.Text(c, a.connectProgressText()).FontSize(9.5).TextColor(colMuted2)
			})
			return
		}
		ui.Column(c).Grow(1).Fill().Center().Gap(9).Padding(24).Children(func() {
			ui.Box(c).Size(56, 56).Radius(16).
				Background(colPanel2.Alpha(0.6)).Border(1, colLine).Center().Children(func() {
				iconEl(c, iconServer, 24, colMuted2)
			})
			ui.Text(c, "未连接").FontSize(fsBody).FontWeight(600)
			ui.Text(c, "点击顶部「新建连接」或使用标签栏「+」选择会话").FontSize(10.5).TextColor(colMuted)
			ui.Text(c, "点顶部 + 从快速连接打开已保存的服务器").FontSize(9.5).TextColor(colMuted2)
		})
		return
	}

	if ts.local {
		switch a.inspectorTab {
		case 1, 2:
			inspectorNotice(c, t, "本地终端没有远程进程和磁盘数据", "连接一台 SSH 服务器后自动采集")
		default:
			ui.Scroll(c).Grow(1).Padding(9).Children(func() {
				ui.Column(c).Gap(8).Children(func() {
					inspectorCard(c, t, func() {
						inspectorCardHead(c, t, "本地终端", "LOCAL")
						inspectorKV(c, t, "程序", ts.Title)
						inspectorKV(c, t, "类型", "本地 Shell")
						inspectorKV(c, t, "工作目录", shortLabel(terminalDir(ts), 26))
						inspectorKVMono(c, t, "编码", "UTF-8")
					})
					inspectorCard(c, t, func() {
						inspectorCardHead(c, t, "远程功能", "")
						inspectorKV(c, t, "SFTP 文件", "需要 SSH 连接")
						inspectorKV(c, t, "服务器监控", "需要 SSH 连接")
						inspectorKV(c, t, "端口转发", "需要 SSH 连接")
					})
					inspectorCard(c, t, func() {
						inspectorCardHead(c, t, "提示", "")
						ui.Text(c, "点击顶部的「新建连接」接入服务器，左侧栏会显示实时 CPU、内存、网络、进程和磁盘。").
							FontSize(10.5).TextColor(colMuted).LineHeight(1.55)
					})
					inspectorCard(c, t, func() {
						inspectorCardHead(c, t, "快捷键", "")
						inspectorKVMono(c, t, "新建连接", "Ctrl  N")
						inspectorKVMono(c, t, "文件管理", "Ctrl  Shift  O")
					})
				})
			})
		}
		return
	}

	ms := a.monitors[ts.ID]
	var latest monitorx.Sample
	hasSample := ms != nil && len(ms.samples) > 0
	if hasSample {
		latest = ms.samples[len(ms.samples)-1]
	}

	if !hasSample {
		if a.inspectorTab == 1 || a.inspectorTab == 2 {
			inspectorNotice(c, t, "正在采集服务器指标…", "连接建立后每 2 秒刷新一次")
			return
		}
	}

	ui.Scroll(c).Grow(1).Padding(9).Children(func() {
		ui.Column(c).Gap(8).Children(func() {
			switch a.inspectorTab {
			case 1:
				inspectorProcesses(c, t, latest)
			case 2:
				inspectorDisks(c, t, latest)
			default:
				inspectorCard(c, t, func() {
					meta := ""
					if latest.UptimeSec > 0 {
						meta = formatUptime(latest.UptimeSec) + " uptime"
					}
					inspectorCardHead(c, t, "系统信息", meta)
					osName := latest.OS
					if osName == "" {
						osName = "Linux"
					}
					kernel := latest.Kernel
					if kernel == "" {
						kernel = "—"
					}
					inspectorKV(c, t, "系统", shortLabel(osName, 24))
					inspectorKV(c, t, "负载", fmt.Sprintf("%.2f / %.2f / %.2f", latest.Load1, latest.Load5, latest.Load15))
					inspectorKV(c, t, "内核", shortLabel(kernel, 22))
					inspectorKV(c, t, "架构", condValue(latest.Arch, "—"))
					inspectorKV(c, t, "运行时间", condValue(formatUptime(latest.UptimeSec), "—"))
					inspectorKV(c, t, "核心", fmt.Sprintf("%d", latest.CPUs))
				})

				inspectorCard(c, t, func() {
					inspectorCardHeadLive(c, t, "资源")
					ui.Row(c).Gap(4).Children(func() {
						a.ringGauge(c, t, "CPU", latest.CPU, colCyan)
						a.ringGauge(c, t, "内存", percent(latest.MemUsed, latest.MemTotal), colViolet)
						a.ringGauge(c, t, "交换", percent(latest.SwapUsed, latest.SwapTotal), colAmber)
					})
					ui.Column(c).Gap(7).Padding(4, 0, 0, 0).Children(func() {
						usageRow(c, t, "内存", latest.MemUsed, latest.MemTotal, colViolet)
						usageRow(c, t, "交换", latest.SwapUsed, latest.SwapTotal, colAmber)
					})
				})

				inspectorCard(c, t, func() {
					inspectorCardHeadLive(c, t, "网络")
					ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "↓ "+humanSize(int64(latest.RxRate))+"/s").FontSize(10).Font("Cascadia Code").TextColor(colGreen)
						ui.Text(c, "↑ "+humanSize(int64(latest.TxRate))+"/s").FontSize(10).Font("Cascadia Code").TextColor(colCyan)
						ui.Text(c, "").Grow(1)
						ui.Text(c, "live").FontSize(9).TextColor(colMuted2)
					})
					if ms != nil {
						a.inspectorSpark(c, t, ms.samples)
					}
					ui.Row(c).Children(func() {
						ui.Text(c, "-60s").FontSize(9).TextColor(colMuted2)
						ui.Text(c, "").Grow(1)
						ui.Text(c, "now").FontSize(9).TextColor(colMuted2)
					})
				})

				inspectorDisks(c, t, latest)
			}
		})
	})
}

func (a *App) profileForTerm(ts *termSession) *store.Profile {
	if ts == nil || ts.profileID == 0 {
		return nil
	}
	for i := range a.profiles {
		if a.profiles[i].ID == ts.profileID {
			return &a.profiles[i]
		}
	}
	return nil
}

func terminalDir(ts *termSession) string {
	if ts == nil || ts.Term == nil {
		return "~"
	}
	if dir := ts.Term.Dir(); dir != "" {
		return dir
	}
	return "~"
}

func condValue(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func inspectorNotice(c *ui.Context, t *ui.Theme, title, sub string) {
	ui.Column(c).Grow(1).Fill().Center().Gap(7).Padding(24).Children(func() {
		iconEl(c, iconActivity, 24, colMuted2)
		ui.Text(c, title).FontSize(fsBody).TextColor(colMuted)
		ui.Text(c, sub).FontSize(10).TextColor(colMuted2)
	})
}

// inspectorCard is a flat section of the monitor: the panel's surface shows
// through, and a hairline outline keeps the sections apart without a plate.
func inspectorCard(c *ui.Context, t *ui.Theme, build func()) {
	ui.Column(c).Padding(8, 10).Border(1, colLine2).Radius(9).Gap(7).Children(build)
}

func inspectorCardHead(c *ui.Context, t *ui.Theme, title, meta string, icons ...*ui.SVG) {
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		if len(icons) > 0 && icons[0] != nil {
			iconEl(c, icons[0], 10, colMuted2)
		}
		ui.Text(c, title).FontSize(8.4).FontWeight(700).LetterSpacing(1.0).TextColor(colMuted)
		ui.Text(c, "").Grow(1)
		if meta != "" {
			ui.Text(c, meta).FontSize(8).Font("Cascadia Code").TextColor(colMuted2).NoWrap()
		}
	})
}

func inspectorCardHeadLive(c *ui.Context, t *ui.Theme, title string) {
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(8.4).FontWeight(700).LetterSpacing(1.0).TextColor(colMuted)
		ui.Text(c, "").Grow(1)
		statusDot(c, colGreen, 4)
		ui.Text(c, "LIVE").FontSize(8).Font("Cascadia Code").LetterSpacing(0.6).TextColor(colGreen)
	})
}

func inspectorKV(c *ui.Context, t *ui.Theme, key, value string) {
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Text(c, key).FontSize(10.5).TextColor(colMuted)
		ui.Text(c, "").Grow(1)
		ui.Text(c, value).FontSize(10.5).FontWeight(500).TextColor(colText2).NoWrap()
	})
}

func inspectorKVFont(c *ui.Context, t *ui.Theme, key, value string, mono bool) {
	size := float32(10.5)
	if mono {
		size = 9.8
	}
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Text(c, key).FontSize(10.5).TextColor(colMuted)
		ui.Text(c, "").Grow(1)
		text := ui.Text(c, value).FontSize(size).FontWeight(500).TextColor(colText2).NoWrap()
		if mono {
			text.Font("Cascadia Code")
		}
	})
}

func inspectorKVMono(c *ui.Context, t *ui.Theme, key, value string) {
	inspectorKVFont(c, t, key, value, true)
}

func (a *App) ringGauge(c *ui.Context, t *ui.Theme, label string, pct float64, color ui.Color) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	slices := chart.Pie([]float64{pct, 100 - pct}, 0)

	ui.Column(c).Grow(1).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(60).Height(60).Draw(func(p *ui.Painter, r ui.Rect) {
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			outer := min(r.W, r.H)/2 - 1
			inner := outer - 5
			p.FillPath(chart.Sector(cx, cy, inner, outer, slices[1]), colTrack)
			p.FillPath(chart.Sector(cx, cy, inner, outer, slices[0]), color)
		}).Children(func() {
			ui.Column(c).Fill().Center().Gap(0).Children(func() {
				ui.Text(c, fmt.Sprintf("%.0f%%", pct)).FontSize(12.5).FontWeight(650).TextColor(colText)
				ui.Text(c, label).FontSize(8).TextColor(colMuted2)
			})
		})
	})
}

func usageRow(c *ui.Context, t *ui.Theme, label string, used, total uint64, color ui.Color) {
	pct := percent(used, total)
	ui.Column(c).Gap(4).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%s  %s / %s", label, humanSize(int64(used)), humanSize(int64(total)))).
				FontSize(10).TextColor(colMuted).NoWrap()
			ui.Text(c, "").Grow(1)
			ui.Text(c, fmt.Sprintf("%.0f%%", pct)).FontSize(9.5).Font("Cascadia Code").TextColor(colMuted2)
		})
		barFill(c, pct/100, color, 4)
	})
}

func barFill(c *ui.Context, frac float64, color ui.Color, h float32) ui.Element {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return ui.Box(c).FillWidth().Height(h).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, colTrack, h/2)
		if frac > 0 {
			p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * float32(frac), H: r.H}, color, h/2)
		}
	})
}

// recentSamples keeps the last minute, which is what the chart labels claim.
func recentSamples(samples []monitorx.Sample) []monitorx.Sample {
	if len(samples) < 2 {
		return samples
	}
	cut := samples[len(samples)-1].At.Add(-time.Minute)
	i := 0
	for i < len(samples)-1 && samples[i].At.Before(cut) {
		i++
	}
	return samples[i:]
}

// netRates is the rate of one interface, or the aggregate for "".
func netRates(s monitorx.Sample, iface string) (float64, float64) {
	if iface == "" {
		return s.RxRate, s.TxRate
	}
	for _, in := range s.Interfaces {
		if in.Name == iface {
			return in.RxRate, in.TxRate
		}
	}
	return 0, 0
}

// netSeries turns samples into one interface's rates (or the aggregate).
func netSeries(samples []monitorx.Sample, iface string) ([]float64, []float64) {
	rx := make([]float64, len(samples))
	tx := make([]float64, len(samples))
	for i, s := range samples {
		if iface == "" {
			rx[i], tx[i] = s.RxRate, s.TxRate
			continue
		}
		for _, in := range s.Interfaces {
			if in.Name == iface {
				rx[i], tx[i] = in.RxRate, in.TxRate
				break
			}
		}
	}
	return rx, tx
}

// inspectorNetHead is the network card's header with the interface picker:
// the aggregate by default, or one interface's counters.
func (a *App) inspectorNetHead(c *ui.Context, t *ui.Theme, ts *termSession, iface string, s monitorx.Sample) {
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		iconEl(c, iconActivity, 10, colMuted2)
		ui.Text(c, "网络 · NET").FontSize(8.4).FontWeight(700).LetterSpacing(1.0).
			TextColor(colMuted).NoWrap()
		ui.Text(c, "").Grow(1)

		name := iface
		if name == "" {
			name = "全部网卡"
		}
		sel := ui.ButtonBase(c.Key("net-iface")).Height(18).Padding(0, 5).Radius(4).
			Gap(3).AlignItems(ui.Center).Label("选择网卡").Tooltip("选择网卡")
		sel.Background(colPanel2.Alpha(0.6)).Border(1, colLine)
		if sel.Hovered() {
			sel.Background(colPanel2)
			sel.BorderColor(colLine2)
		}
		sel.Children(func() {
			ui.Text(c, shortLabel(name, 12)).FontSize(8.2).Font("Cascadia Code").
				TextColor(colMuted).NoWrap()
			iconEl(c, iconChevron, 9, colMuted2)
		})
		sel.Menu(func(m *ui.Menu) {
			if m.Item("全部网卡").Chosen() {
				a.netIface[ts.ID] = ""
			}
			for _, in := range s.Interfaces {
				in := in
				if m.Item(in.Name).Chosen() {
					a.netIface[ts.ID] = in.Name
				}
			}
		})

		ui.Text(c, "60s").FontSize(8).Font("Cascadia Code").TextColor(colMuted2).NoWrap()
	})
}

// inspectorNetBars draws one minute of network traffic as paired bars:
// download next to upload, one bucket per few seconds, which reads as two
// separate series at this width far better than overlapping lines.
func (a *App) inspectorNetBars(c *ui.Context, t *ui.Theme, samples []monitorx.Sample, iface string) {
	if len(samples) < 2 {
		ui.Text(c, "采集中…").FontSize(10).TextColor(colMuted2)
		return
	}

	rxVals, txVals := netSeries(samples, iface)
	maxRate := 1.0
	for i := range rxVals {
		maxRate = max(maxRate, rxVals[i], txVals[i])
	}
	maxRate *= 1.2

	n := min(15, len(samples))
	rx, tx := rateBuckets(rxVals, txVals, n)

	chart.Plot(c, chart.Axes{}, func(area ui.Element) {
		area.Draw(func(p *ui.Painter, r ui.Rect) {
			for _, frac := range []float32{0.25, 0.5, 0.75} {
				yy := r.Y + r.H*frac
				p.Line(r.X, yy, r.X+r.W, yy, 1, colLine)
			}
			slot := r.W / float32(n)
			pad := slot * 0.16
			gap := max(float32(1), slot*0.08)
			barW := (slot - 2*pad - gap) / 2
			if barW < 1.5 {
				barW = 1.5
			}
			for i := 0; i < n; i++ {
				x0 := r.X + float32(i)*slot + pad
				rxH := r.H * (rx[i] / float32(maxRate))
				txH := r.H * (tx[i] / float32(maxRate))
				p.Fill(ui.Rect{X: x0, Y: r.Y + r.H - rxH, W: barW, H: rxH}, colGreen, 2)
				p.Fill(ui.Rect{X: x0 + barW + gap, Y: r.Y + r.H - txH, W: barW, H: txH}, colBlue, 2)
			}
		})
	}).Height(64).Gap(2)
}

// rateBuckets folds rate series into n buckets and keeps the peak of each,
// so a burst stays visible instead of averaging away.
func rateBuckets(rxVals, txVals []float64, n int) ([]float32, []float32) {
	rx := make([]float32, max(0, n))
	tx := make([]float32, max(0, n))
	if n <= 0 || len(rxVals) == 0 {
		return rx, tx
	}
	rxS, txS := rxVals, txVals
	if len(rxS) > n*2 {
		rxS = rxS[len(rxS)-n*2:]
	}
	if len(txS) > n*2 {
		txS = txS[len(txS)-n*2:]
	}
	for i := 0; i < n; i++ {
		lo := i * len(rxS) / n
		hi := (i + 1) * len(rxS) / n
		if hi <= lo {
			hi = lo + 1
		}
		if hi > len(rxS) {
			hi = len(rxS)
		}
		for j := lo; j < hi; j++ {
			rx[i] = max(rx[i], float32(rxS[j]))
			if j < len(txS) {
				tx[i] = max(tx[i], float32(txS[j]))
			}
		}
	}
	return rx, tx
}

// inspectorLatency draws the SSH round trip over the last minute.
func (a *App) inspectorLatency(c *ui.Context, t *ui.Theme, samples []monitorx.Sample) {
	first := 0.0
	maxMs := 0.0
	seen := 0
	for _, s := range samples {
		if s.LatencyMs <= 0 {
			continue
		}
		if first == 0 {
			first = s.LatencyMs
		}
		maxMs = max(maxMs, s.LatencyMs)
		seen++
	}
	if seen < 2 || len(samples) < 2 {
		ui.Text(c, "采集中…").FontSize(10).TextColor(colMuted2)
		return
	}

	last := first
	vals := make([]float64, len(samples))
	for i, s := range samples {
		if s.LatencyMs > 0 {
			last = s.LatencyMs
		}
		vals[i] = last
	}
	maxMs = max(maxMs*1.25, 10)
	y := chart.NewLinear(0, maxMs)

	chart.Plot(c, chart.Axes{}, func(area ui.Element) {
		area.Draw(func(p *ui.Painter, r ui.Rect) {
			for _, frac := range []float32{0.25, 0.5, 0.75} {
				yy := r.Y + r.H*frac
				p.Line(r.X, yy, r.X+r.W, yy, 1, colLine)
			}
			xs := make([]float32, len(samples))
			ys := make([]float32, len(samples))
			for i := range samples {
				xs[i] = float32(i) / float32(len(samples)-1)
				ys[i] = y.At(vals[i])
			}
			p.StrokePath(chart.Line(r, xs, ys, chart.CurveMonotone), 1.4, colCyan)
		})
	}).Height(40).Gap(2)
}

func (a *App) inspectorSpark(c *ui.Context, t *ui.Theme, samples []monitorx.Sample) {
	if len(samples) < 2 {
		ui.Text(c, "采集中…").FontSize(10).TextColor(colMuted2)
		return
	}

	maxRate := 1.0
	for _, s := range samples {
		maxRate = max(maxRate, s.RxRate, s.TxRate)
	}
	maxRate *= 1.15

	from, to := samples[0].At, samples[len(samples)-1].At
	if !to.After(from) {
		to = from.Add(time.Second)
	}
	x := chart.NewTime(from, to)
	y := chart.NewLinear(0, maxRate)

	chart.Plot(c, chart.Axes{}, func(area ui.Element) {
		area.Draw(func(p *ui.Painter, r ui.Rect) {
			for _, frac := range []float32{0.25, 0.5, 0.75} {
				yy := r.Y + r.H*frac
				p.Line(r.X, yy, r.X+r.W, yy, 1, colLine)
			}
			xs := make([]float32, len(samples))
			rx := make([]float32, len(samples))
			tx := make([]float32, len(samples))
			for i, s := range samples {
				xs[i] = x.At(s.At)
				rx[i] = y.At(s.RxRate)
				tx[i] = y.At(s.TxRate)
			}
			p.StrokePath(chart.Line(r, xs, rx, chart.CurveMonotone), 1.6, colGreen)
			p.StrokePath(chart.Line(r, xs, tx, chart.CurveMonotone), 1.4, colBlue)
		})
	}).Height(64).Gap(2)
}

// inspectorSwapCard shows swap use as the third resource row, separate from
// the CPU/memory tiles so it can be hidden when the server has no swap.
func (a *App) inspectorSwapCard(c *ui.Context, s monitorx.Sample) {
	pct := percent(s.SwapUsed, s.SwapTotal)
	ui.Column(c).FillWidth().Padding(7, 10).Border(1, colLine2).Radius(9).Gap(5).Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "交换").FontSize(9.5).TextColor(colMuted).NoWrap()
			ui.Text(c, "").Grow(1)
			ui.Text(c, fmt.Sprintf("%.0f%% · %s / %s", pct,
				humanSize(int64(s.SwapUsed)), humanSize(int64(s.SwapTotal)))).
				FontSize(8.2).Font("Cascadia Code").TextColor(colMuted2).NoWrap()
		})
		barFill(c, pct/100, colAmber.Mix(colTermBg, 0.3), 5)
	})
}

func inspectorProcesses(c *ui.Context, t *ui.Theme, s monitorx.Sample) {
	inspectorCard(c, t, func() {
		inspectorCardHead(c, t, "进程", "内存 · CPU", iconCommand)
		if len(s.Processes) == 0 {
			ui.Text(c, "正在采集进程…").FontSize(10).TextColor(colMuted2)
			return
		}
		maxRSS := s.Processes[0].RSS
		if maxRSS == 0 {
			maxRSS = 1
		}
		for _, p := range s.Processes {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				ui.Text(c, shortLabel(p.Name, 11)).FontSize(10.5).TextColor(colMuted).
					Width(62).NoWrap()
				barFill(c, float64(p.RSS)/float64(maxRSS), colCyan.Alpha(0.75), 3).Grow(1)
				ui.Text(c, humanSize(int64(p.RSS))).FontSize(9.5).Font("Cascadia Code").
					TextColor(colMuted2).Width(48).TextAlign(ui.End)
				ui.Text(c, fmt.Sprintf("%.1f%%", p.CPU)).FontSize(9).Font("Cascadia Code").
					TextColor(colMuted2).Width(38).TextAlign(ui.End).NoWrap()
			})
		}
	})
}

func inspectorDisks(c *ui.Context, t *ui.Theme, s monitorx.Sample) {
	inspectorCard(c, t, func() {
		if len(s.Disks) > 0 {
			inspectorCardHead(c, t, "磁盘", "已用 / 总量", iconServer)
			for _, d := range s.Disks {
				color := colCyan
				switch pct := percent(d.Used, d.Total); {
				case pct >= 90:
					color = colRed
				case pct >= 75:
					color = colAmber
				}
				diskRow(c, t, shortLabel(d.Mount, 9), d.Used, d.Total, color)
			}
			return
		}
		inspectorCardHead(c, t, "磁盘", fmt.Sprintf("%.0f%% · %s", percent(s.DiskUsed, s.DiskTotal), humanSize(int64(s.DiskTotal))), iconServer)
		diskRow(c, t, "/", s.DiskUsed, s.DiskTotal, colCyan)
	})
}

func diskRow(c *ui.Context, t *ui.Theme, label string, used, total uint64, color ui.Color) {
	pct := percent(used, total)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(9.5).Font("Cascadia Code").TextColor(colText).Width(46).NoWrap()
		barFill(c, pct/100, color, 3).Grow(1)
		ui.Text(c, fmt.Sprintf("%s / %s", humanSize(int64(used)), humanSize(int64(total)))).
			FontSize(8.4).Font("Cascadia Code").TextColor(colMuted2).
			Width(100).TextAlign(ui.End).NoWrap()
	})
}

func osTag(osName string) string {
	osName = strings.TrimSpace(osName)
	if osName == "" {
		return ""
	}
	fields := strings.Fields(osName)
	name := fields[0]
	for _, f := range fields[1:] {
		if f[0] < '0' || f[0] > '9' {
			continue
		}
		ver := f
		if parts := strings.Split(ver, "."); len(parts) > 2 {
			ver = parts[0] + "." + parts[1]
		}
		return shortLabel(name+" "+ver, 16)
	}
	return shortLabel(name, 16)
}

func formatUptime(sec uint64) string {
	if sec == 0 {
		return ""
	}
	days := sec / 86400
	hours := (sec % 86400) / 3600
	if days > 0 {
		return fmt.Sprintf("%d 天 %d 小时", days, hours)
	}
	mins := (sec % 3600) / 60
	return fmt.Sprintf("%d 小时 %d 分", hours, mins)
}

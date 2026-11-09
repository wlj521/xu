package app

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"

	"xu/internal/monitorx"
)

type monitorState struct {
	termID    int64
	collector *monitorx.Collector
	samples   []monitorx.Sample
	err       string
}

func (a *App) openMonitor() {
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	ts := a.activeTerminal()
	if ts == nil {
		a.status = "请先打开一个终端"
		return
	}
	if ts.local {
		a.monitorCollapsed = false
		return
	}
	if a.finding {
		a.closeFind()
	}
	a.monitorCollapsed = false
	a.startMonitorFor(ts, false)
}

func (a *App) startMonitor(ts *termSession) {
	a.startMonitorFor(ts, true)
}

func (a *App) startMonitorBackground(ts *termSession) {
	a.startMonitorFor(ts, false)
}

func (a *App) startMonitorFor(ts *termSession, open bool) {
	if ts == nil {
		return
	}
	if ms := a.monitors[ts.ID]; ms != nil {
		if open {
			a.monitorOpen = true
			a.filesOpen = false
		}
		return
	}

	ms := &monitorState{termID: ts.ID}
	col, err := monitorx.Start(a.ctx, ts.client, 2*time.Second,
		func(s monitorx.Sample) {
			a.post(func() {
				if a.monitors[ts.ID] != ms {
					return
				}
				ms.samples = append(ms.samples, s)
				if len(ms.samples) > 180 {
					ms.samples = ms.samples[len(ms.samples)-180:]
				}
			})
		},
		func(err error) {
			a.post(func() {
				if a.monitors[ts.ID] == ms {
					ms.err = err.Error()
				}
			})
		},
	)
	if err != nil {
		a.status = "启动监控失败: " + err.Error()
		return
	}

	ms.collector = col
	a.monitors[ts.ID] = ms
	if open {
		a.monitorOpen = true
		a.filesOpen = false
		a.fileErr = ""
	}
}

func (a *App) stopMonitor(termID int64) {
	ms := a.monitors[termID]
	if ms == nil {
		return
	}
	delete(a.monitors, termID)
	if ms.collector != nil {
		go ms.collector.Stop()
	}
}

func (a *App) restartMonitor(ts *termSession) {
	if ts == nil {
		return
	}
	a.stopMonitor(ts.ID)
	a.startMonitor(ts)
}

func (a *App) monitorView(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	if ts == nil {
		a.monitorOpen = false
		return
	}
	ms := a.monitors[ts.ID]

	ui.Column(c).Grow(1).Padding(24, 28, 20, 28).Gap(14).Children(func() {
		pageHeader(c, t, iconActivity, "服务器监控", ts.Title, func() {
			if ghostAction(c, t, "monitor-terminal", "终端", iconTerm).Clicked() {
				a.monitorOpen = false
			}
			if ghostAction(c, t, "monitor-files", "文件", iconFolder).Clicked() {
				a.monitorOpen = false
				a.filesOpen = true
				a.openFiles()
			}
			if ms != nil {
				if ghostAction(c, t, "monitor-restart", "重新采集", iconRefresh).Clicked() {
					a.restartMonitor(ts)
				}
			}
		})

		if ms == nil {
			cardPanel(c, func() {
				ui.Column(c).FillWidth().Center().Gap(10).Padding(24, 0).Children(func() {
					iconEl(c, iconActivity, 28, colMuted2)
					ui.Text(c, "还没有开始采集").FontSize(fsTitle).FontWeight(600)
					ui.Text(c, "连接服务器后可实时查看 CPU、内存、网络和磁盘").
						FontSize(10.5).TextColor(colMuted)
					if primaryAction(c, "monitor-start", "开始监控", iconActivity).Clicked() {
						a.startMonitor(ts)
					}
				})
			})
			return
		}

		if ms.err != "" {
			ui.Box(c).FillWidth().Padding(10, 12).Radius(8).
				Background(colRed.Alpha(0.07)).Border(1, colRed.Alpha(0.18)).Children(func() {
				ui.Text(c, ms.err).FontSize(10.5).TextColor(colRed)
			})
		}

		if len(ms.samples) == 0 {
			cardPanel(c, func() {
				ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
					iconEl(c, iconRefresh, 16, colCyan)
					ui.Text(c, "正在采集第一组数据…").FontSize(fsBody).TextColor(colMuted)
				})
			})
			return
		}

		latest := ms.samples[len(ms.samples)-1]
		a.monitorCards(c, t, latest)

		ui.Row(c).Grow(1).Gap(10).Children(func() {
			a.monitorChart(c, t, "CPU", ms.samples, 100, func(s monitorx.Sample) float64 {
				return s.CPU
			}, t.Accent, "%.1f%%")
			a.monitorChart(c, t, "内存", ms.samples, 100, func(s monitorx.Sample) float64 {
				return percent(s.MemUsed, s.MemTotal)
			}, colViolet, "%.1f%%")
		})

		ui.Row(c).Grow(1).Gap(10).Children(func() {
			a.networkChart(c, t, ms.samples)
			a.monitorChart(c, t, "磁盘", ms.samples, 100, func(s monitorx.Sample) float64 {
				return percent(s.DiskUsed, s.DiskTotal)
			}, colAmber, "%.1f%%")
		})
	})
}

func (a *App) monitorCards(c *ui.Context, t *ui.Theme, s monitorx.Sample) {
	ui.Row(c).Gap(10).Children(func() {
		statCard(c, t, "CPU",
			fmt.Sprintf("%.1f%%", s.CPU),
			fmt.Sprintf("%d 核 · load %.2f", s.CPUs, s.Load1),
			colCyan,
		)
		statCard(c, t, "内存",
			fmt.Sprintf("%.1f%%", percent(s.MemUsed, s.MemTotal)),
			fmt.Sprintf("%s / %s", humanSize(int64(s.MemUsed)), humanSize(int64(s.MemTotal))),
			colViolet,
		)
		statCard(c, t, "磁盘",
			fmt.Sprintf("%.1f%%", percent(s.DiskUsed, s.DiskTotal)),
			fmt.Sprintf("%s / %s", humanSize(int64(s.DiskUsed)), humanSize(int64(s.DiskTotal))),
			colAmber,
		)
		statCard(c, t, "网络",
			fmt.Sprintf("↓ %s/s", humanSize(int64(s.RxRate))),
			fmt.Sprintf("↑ %s/s", humanSize(int64(s.TxRate))),
			colGreen,
		)
	})
}

func statCard(c *ui.Context, t *ui.Theme, title, value, sub string, accent ui.Color) {
	ui.Column(c).Grow(1).Padding(14).Background(colPanel).
		Border(1, colLine).Radius(8).Gap(5).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			statusDot(c, accent, 5)
			ui.Text(c, title).FontSize(10).FontWeight(600).TextColor(colMuted2)
		})
		ui.Text(c, value).FontSize(22).FontWeight(650)
		if sub != "" {
			ui.Text(c, sub).FontSize(10).TextColor(colMuted).NoWrap()
		}
	})
}

func (a *App) monitorChart(c *ui.Context, t *ui.Theme, title string, samples []monitorx.Sample, yMax float64, value func(monitorx.Sample) float64, color ui.Color, format string) {
	ui.Column(c).Grow(1).Gap(8).Padding(14).Background(colPanel).
		Border(1, colLine).Radius(8).Children(func() {
		latest := 0.0
		if len(samples) > 0 {
			latest = value(samples[len(samples)-1])
		}

		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, title).FontSize(12).FontWeight(600)
			ui.Text(c, fmt.Sprintf(format, latest)).FontSize(18).FontWeight(650).TextColor(color)
			ui.Text(c, fmt.Sprintf("%d 个样本", len(samples))).FontSize(9.5).TextColor(colMuted2)
		})

		plotSeries(c, t, samples, yMax, value, color)
	})
}

func (a *App) networkChart(c *ui.Context, t *ui.Theme, samples []monitorx.Sample) {
	ui.Column(c).Grow(1).Gap(8).Padding(14).Background(colPanel).
		Border(1, colLine).Radius(8).Children(func() {
		var latest monitorx.Sample
		if len(samples) > 0 {
			latest = samples[len(samples)-1]
		}

		rxColor := colBlue
		txColor := colGreen

		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "网络").FontSize(12).FontWeight(600)
			ui.Text(c, fmt.Sprintf("↓ %s/s", humanSize(int64(latest.RxRate)))).FontSize(10.5).Font("Cascadia Code").TextColor(rxColor)
			ui.Text(c, fmt.Sprintf("↑ %s/s", humanSize(int64(latest.TxRate)))).FontSize(10.5).Font("Cascadia Code").TextColor(txColor)
		})

		if len(samples) < 2 {
			ui.Text(c, "采集中…").FontSize(10).TextColor(colMuted2)
			return
		}

		maxRate := 1.0
		for _, s := range samples {
			maxRate = math.Max(maxRate, s.RxRate)
			maxRate = math.Max(maxRate, s.TxRate)
		}
		maxRate *= 1.1

		from, to := samples[0].At, samples[len(samples)-1].At
		if !to.After(from) {
			to = from.Add(time.Second)
		}
		x := chart.NewTime(from, to)
		y := chart.NewLinear(0, maxRate).Nice(4)
		ticks := y.Ticks(4)

		chart.Plot(c, chart.Axes{Left: ticks, Bottom: x.Ticks(4)}, func(area ui.Element) {
			area.Draw(func(p *ui.Painter, r ui.Rect) {
				for _, tick := range ticks {
					p.Line(r.X, chart.Y(r, tick.At), r.X+r.W, chart.Y(r, tick.At), 1, colLine)
				}

				xs := make([]float32, len(samples))
				rx := make([]float32, len(samples))
				tx := make([]float32, len(samples))
				for i, s := range samples {
					xs[i] = x.At(s.At)
					rx[i] = y.At(s.RxRate)
					tx[i] = y.At(s.TxRate)
				}

				p.FillPath(chart.Area(r, xs, nil, rx, chart.CurveMonotone), rxColor.Alpha(0.12))
				p.StrokePath(chart.Line(r, xs, rx, chart.CurveMonotone), 2, rxColor)
				p.StrokePath(chart.Line(r, xs, tx, chart.CurveMonotone), 2, txColor)
			})
		}).Height(170).Gap(6)
	})
}

func plotSeries(c *ui.Context, t *ui.Theme, samples []monitorx.Sample, yMax float64, value func(monitorx.Sample) float64, color ui.Color) {
	if len(samples) < 2 {
		ui.Text(c, "采集中…").FontSize(10).TextColor(colMuted2)
		return
	}

	from, to := samples[0].At, samples[len(samples)-1].At
	if !to.After(from) {
		to = from.Add(time.Second)
	}
	if yMax <= 0 {
		yMax = 1
	}

	x := chart.NewTime(from, to)
	y := chart.NewLinear(0, yMax).Nice(4)
	ticks := y.Ticks(4)

	chart.Plot(c, chart.Axes{Left: ticks, Bottom: x.Ticks(4)}, func(area ui.Element) {
		area.Draw(func(p *ui.Painter, r ui.Rect) {
			for _, tick := range ticks {
				p.Line(r.X, chart.Y(r, tick.At), r.X+r.W, chart.Y(r, tick.At), 1, colLine)
			}

			xs := make([]float32, len(samples))
			ys := make([]float32, len(samples))
			for i, s := range samples {
				xs[i] = x.At(s.At)
				ys[i] = y.At(value(s))
			}

			p.FillPath(chart.Area(r, xs, nil, ys, chart.CurveMonotone), color.Alpha(0.15))
			p.StrokePath(chart.Line(r, xs, ys, chart.CurveMonotone), 2, color)
		})
	}).Height(170).Gap(6)
}

func percent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) * 100 / float64(total)
}

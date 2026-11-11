package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// CheckForUpdates checks as the user asked: the dialog opens at once and
// shows the check, the result, or a newer version.
func (a *App) checkUpdates(manual bool) {
	if a.updateChecking {
		if manual {
			a.updateOpen = true
		}
		return
	}
	if !mygo.Updater.Enabled() {
		if manual {
			a.updateOpen = true
			a.updateErr = "当前版本不支持自动更新（开发版，或安装目录不可写）"
			a.updateDetail = ""
		}
		return
	}

	a.updateChecking = true
	a.updateErr = ""
	a.updateDetail = ""
	a.updateStatus = ""
	a.updateAvailable = nil
	a.updateProgress = 0
	a.updateInstalling = false
	if manual {
		a.updateOpen = true
	}

	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
		defer cancel()
		upd, err := mygo.Updater.Check(ctx)
		a.post(func() {
			a.updateChecking = false
			if err != nil {
				if manual || a.updateOpen {
					a.updateErr = friendlyUpdateError(err)
					a.updateDetail = err.Error()
					a.updateOpen = true
				}
				return
			}
			if upd == nil {
				if manual {
					a.updateStatus = "当前已是最新版本 · Xu " + mygo.App.Version()
					a.updateOpen = true
				}
				return
			}
			a.updateAvailable = upd
			a.updateOpen = true
		})
	}()
}

// AutoCheckUpdates checks at most once a day in the background. It runs a
// few seconds after launch so the check never delays the first window.
func (a *App) AutoCheckUpdates() {
	select {
	case <-time.After(5 * time.Second):
	case <-a.ctx.Done():
		return
	}
	if a.store == nil || !mygo.Updater.Enabled() {
		return
	}
	settings, err := a.store.Settings(a.ctx)
	if err != nil {
		return
	}
	if v := settings["update.last_check"]; v != "" {
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil &&
			time.Since(time.Unix(ts, 0)) < 24*time.Hour {
			return
		}
	}
	_ = a.store.SetSetting(a.ctx, "update.last_check", strconv.FormatInt(time.Now().Unix(), 10))
	a.checkUpdates(false)
}

func (a *App) installUpdate() {
	if a.updateAvailable == nil || a.updateInstalling {
		return
	}
	upd := a.updateAvailable
	a.updateInstalling = true
	a.updateErr = ""
	a.updateDetail = ""
	a.updateProgress = 0

	go func() {
		err := upd.Install(a.ctx, func(done, total int64) {
			a.post(func() {
				if total > 0 {
					a.updateProgress = float64(done) / float64(total)
				}
			})
		})
		a.post(func() {
			a.updateInstalling = false
			if err != nil {
				a.updateErr = "安装更新失败，请稍后重试。"
				a.updateDetail = err.Error()
				return
			}
			a.updateProgress = 1
			mygo.App.Relaunch()
		})
	}()
}

func friendlyUpdateError(err error) string {
	if strings.Contains(err.Error(), "404") {
		return "更新源暂未就绪：GitHub 最新版本还没有附带更新清单，请先发布带自动更新的版本。"
	}
	return "检查更新失败，请稍后再试。"
}

// updateModal is the app's own update dialog, in the same theme and language
// as the rest of Xu instead of the framework's separate update window.
func (a *App) updateModal(c *ui.Context, t *ui.Theme) {
	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}

	ui.DialogBase(c, &a.updateOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(fitDialogWidth(c, 540)).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Column(c).Padding(22).Gap(13).Children(func() {
				ui.Row(c).FillWidth().Gap(11).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(36, 36).Radius(10).
						Background(colCyan.Alpha(0.08)).Border(1, colCyan.Alpha(0.22)).Center().Children(func() {
						iconEl(c, iconRefresh, 17, colCyan)
					})
					ui.Column(c).Grow(1).Gap(2).Children(func() {
						ui.Text(c, "检查更新").FontSize(15).FontWeight(650).TextColor(colText)
						ui.Text(c, "当前版本 Xu "+mygo.App.Version()).FontSize(9.4).
							Font("Cascadia Code").TextColor(colMuted2)
					})
					close := ui.ButtonBase(c.Key("update-close")).Size(24, 24).
						Radius(6).Center().Tooltip("关闭").Label("关闭")
					if close.Hovered() {
						close.Background(colPanel2)
					}
					close.Children(func() { iconEl(c, iconClose, 13, colMuted) })
					if close.Clicked() {
						a.updateOpen = false
					}
				})

				switch {
				case a.updateInstalling:
					pct := int(a.updateProgress * 100)
					ui.Text(c, fmt.Sprintf("正在下载并安装新版本… %d%%", pct)).
						FontSize(10.5).TextColor(colText2)
					barFill(c, a.updateProgress, colCyan, 6)
					ui.Text(c, "下载完成后 Xu 会自动重启。").
						FontSize(9.2).TextColor(colMuted2)

				case a.updateErr != "":
					ui.Text(c, a.updateErr).FontSize(10.5).TextColor(colRed).
						LineHeight(1.6)
					if a.updateDetail != "" {
						ui.Text(c, shortLabel(a.updateDetail, 220)).FontSize(8.8).
							Font("Cascadia Code").TextColor(colMuted2).LineHeight(1.5)
					}
					ui.Row(c).Gap(8).Children(func() {
						label, key := "重试检查", "update-retry-check"
						if a.updateAvailable != nil {
							label, key = "重试更新", "update-retry-install"
						}
						if primaryAction(c, key, label, iconRefresh).Clicked() {
							if a.updateAvailable != nil {
								a.installUpdate()
							} else {
								a.checkUpdates(true)
							}
						}
						if ghostAction(c, t, "update-err-close", "关闭", nil).Clicked() {
							a.updateOpen = false
						}
					})

				case a.updateChecking:
					ui.Text(c, "正在检查更新…").FontSize(10.5).TextColor(colText2)
					ui.Text(c, "正在读取 GitHub 上的最新版本信息。").
						FontSize(9.2).TextColor(colMuted2)

				case a.updateAvailable != nil:
					ui.Text(c, "发现新版本 Xu "+a.updateAvailable.Version).
						FontSize(13).FontWeight(650).TextColor(colCyan)
					if notes := strings.TrimSpace(a.updateAvailable.Notes); notes != "" {
						ui.Text(c, shortLabel(notes, 520)).FontSize(9.6).
							TextColor(colMuted).LineHeight(1.65)
					} else {
						ui.Text(c, "新版本已经准备好，可以立即安装。").
							FontSize(9.6).TextColor(colMuted)
					}
					ui.Row(c).Gap(8).Children(func() {
						if primaryAction(c, "update-install", "立即更新", iconDownload).Clicked() {
							a.installUpdate()
						}
						if ghostAction(c, t, "update-later", "稍后", nil).Clicked() {
							a.updateOpen = false
						}
					})

				default:
					ui.Text(c, "当前已是最新版本 · Xu "+mygo.App.Version()).
						FontSize(10.5).TextColor(colText2)
					ui.Row(c).Gap(8).Children(func() {
						if ghostAction(c, t, "update-up-to-date-close", "关闭", nil).Clicked() {
							a.updateOpen = false
						}
					})
				}
			})
		})
	})
}

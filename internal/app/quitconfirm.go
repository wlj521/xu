package app

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *App) hasOpenTabs() bool {
	return len(a.tabs) > 0 || len(a.terms) > 0
}

func (a *App) openQuitConfirm() {
	if a.quitConfirmOpen {
		return
	}
	a.quitConfirmOpen = true
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *App) handleWindowClose(e *mygo.CloseEvent) {
	if a.quitAllowed || a.quitInProgress || a.updateRelaunch || !a.hasOpenTabs() {
		return
	}
	e.PreventDefault()
	a.openQuitConfirm()
}

func (a *App) handleBeforeQuit(e *mygo.QuitEvent) {
	if e.SessionEnding || a.quitAllowed || a.updateRelaunch {
		a.quitInProgress = true
		return
	}
	if !a.hasOpenTabs() {
		a.quitInProgress = true
		return
	}
	e.PreventDefault()
	a.openQuitConfirm()
}

func (a *App) handleQuitCanceled() {
	a.quitInProgress = false
	a.updateRelaunch = false
}

func (a *App) confirmQuit() {
	a.quitConfirmOpen = false
	a.quitAllowed = true
	a.quitRequested = true
	if a.win != nil {
		a.win.Close()
	}
}

func (a *App) cancelQuit() {
	a.quitConfirmOpen = false
	a.quitAllowed = false
}

func (a *App) quitConfirmModal(c *ui.Context, t *ui.Theme) {
	tabs := len(a.tabs)
	connections := 0
	for _, ts := range a.terms {
		if ts != nil && !ts.local && !ts.exited {
			connections++
		}
	}
	restoreText := "下次启动会恢复这些标签，但不会自动连接。"
	if !a.settings.RestoreWorkspace {
		restoreText = "工作区恢复已关闭，下次启动不会恢复这些标签。"
	}

	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}
	ui.DialogBase(c, &a.quitConfirmOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(fitDialogWidth(c, 520)).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Row(c).Height(62).Shrink(0).Padding(0, 18).Gap(10).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
				ui.Box(c).Size(30, 30).Radius(8).
					Background(colRed.Alpha(0.09)).Border(1, colRed.Alpha(0.22)).Center().Children(func() {
					iconEl(c, iconPower, 15, colRed)
				})
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, "退出 Xu？").FontSize(14.5).FontWeight(650)
					ui.Text(c, fmt.Sprintf("还有 %d 个未关闭的标签页", tabs)).
						FontSize(9.5).TextColor(colAmber).NoWrap()
				})
			})

			ui.Column(c).FillWidth().Padding(18, 18).Gap(11).Children(func() {
				ui.Text(c, fmt.Sprintf("退出会断开全部 SSH 连接（当前 %d 个在线会话），本地终端也会关闭，正在进行的传输会暂停。", connections)).
					FontSize(10.5).TextColor(colText2).LineHeight(1.55)
				ui.Text(c, restoreText).FontSize(9.5).TextColor(colMuted).
					LineHeight(1.5)
				ui.Box(c).FillWidth().Padding(9, 10).Radius(6).
					Background(colAmber.Alpha(0.07)).Border(1, colAmber.Alpha(0.18)).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
						iconEl(c, iconAlert, 12, colAmber)
						ui.Text(c, "之后启动只会恢复标签，不会自动建立连接；点击标签内的「连接」后再拨号。").
							FontSize(9).TextColor(colMuted).LineHeight(1.45)
					})
				})
			})

			ui.Row(c).Height(54).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
				ui.Text(c, "退出后未完成传输会保留为暂停任务").
					FontSize(8.8).TextColor(colMuted2).NoWrap()
				ui.Text(c, "").Grow(1)
				if ghostAction(c, t, "quit-cancel", "取消", nil).Clicked() {
					a.cancelQuit()
				}
				if dangerAction(c, "quit-confirm", "退出并断开", iconPower).Clicked() {
					a.confirmQuit()
				}
			})
		})
	})
}

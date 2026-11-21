package app

import (
	"fmt"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
)

type remoteTransferTarget struct {
	termID    int64
	profileID int64
	label     string
}

func (a *App) openRemoteTransferDialog(b *fileBrowser) {
	if b == nil {
		a.fileErr = "文件面板已关闭，请重试"
		return
	}
	f := selectedEntry(b.remoteEntries, b.remoteSel)
	if f == nil {
		a.fileErr = "请先在右侧选择文件或文件夹"
		return
	}
	source := a.termSessionByID(b.termID)
	if source == nil || source.local || source.exited {
		a.fileErr = "源服务器会话不可用"
		return
	}

	a.remoteTransferOpen = true
	a.remoteTransferSourceID = b.termID
	a.remoteTransferSourcePath = path.Join(b.remoteDir, f.Name())
	a.remoteTransferSourceDir = f.IsDir()
	a.remoteTransferTargetChoice = ""
	a.remoteTransferTargetDir = "/"
	a.remoteTransferConflictChoice = transferConflictLabel(a.transferConflictPolicy())
	a.remoteTransferErr = ""
	a.fileErr = ""
	a.remoteTransferChooseDefaultTarget()
}

func (a *App) remoteTransferTargets(sourceID int64) []remoteTransferTarget {
	source := a.termSessionByID(sourceID)
	var sourceProfileID int64
	if source != nil {
		sourceProfileID = source.profileID
	}
	out := make([]remoteTransferTarget, 0, len(a.tabs))
	for idx, tab := range a.tabs {
		ts := tab.term
		if tab.kind != tabTerminal || ts == nil || ts.local || ts.exited || ts.ID == sourceID {
			continue
		}
		if sourceProfileID != 0 && ts.profileID == sourceProfileID {
			continue
		}
		server := transferServerFromSSHProfile(ts.profile)
		if ts.profileID != 0 {
			server = a.transferServerLabel(ts.profileID)
		}
		label := fmt.Sprintf("%d  %s", idx+1, a.tabDisplayTitle(tab))
		if server != "" && !strings.Contains(label, server) {
			label += " · " + server
		}
		out = append(out, remoteTransferTarget{
			termID:    ts.ID,
			profileID: ts.profileID,
			label:     label,
		})
	}
	return out
}

func (a *App) remoteTransferChooseDefaultTarget() {
	targets := a.remoteTransferTargets(a.remoteTransferSourceID)
	if len(targets) == 0 {
		a.remoteTransferTargetChoice = ""
		return
	}
	for _, target := range targets {
		if target.label == a.remoteTransferTargetChoice {
			return
		}
	}
	a.remoteTransferTargetChoice = targets[0].label
}

func (a *App) remoteTransferSelectedTarget() (remoteTransferTarget, bool) {
	for _, target := range a.remoteTransferTargets(a.remoteTransferSourceID) {
		if target.label == a.remoteTransferTargetChoice {
			return target, true
		}
	}
	return remoteTransferTarget{}, false
}

func (a *App) enqueueRemoteTransfer(sourceID, targetID int64, sourcePath, targetDir string, sourceDir bool, conflict transferConflict) int64 {
	_, sourceServer := a.transferTaskIdentity(sourceID)
	targetProfileID, targetServer := a.transferTaskIdentity(targetID)
	if sourceServer == "" {
		sourceServer = fmt.Sprintf("会话 #%d", sourceID)
	}
	if targetServer == "" {
		targetServer = fmt.Sprintf("会话 #%d", targetID)
	}
	base := path.Base(sourcePath)
	label := "发送 " + base
	if sourceDir {
		label = "发送文件夹 " + base
	}
	targetPath := path.Join(targetDir, base)
	taskTarget := targetServer + " · " + targetPath
	payload := transferPayload{
		Conflict: int(conflict),
		Remote: &transferRemotePayload{
			SourcePath:      sourcePath,
			TargetDir:       targetDir,
			TargetProfileID: targetProfileID,
			TargetServer:    targetServer,
			Directory:       sourceDir,
			TargetSessionID: targetID,
		},
	}
	return a.enqueueTransferJob(transferRemote, label, taskTarget, sourceID, payload)
}

func (a *App) submitRemoteTransfer() {
	source := a.termSessionByID(a.remoteTransferSourceID)
	if source == nil || source.local || source.exited {
		a.remoteTransferErr = "源服务器会话已关闭"
		return
	}
	target, ok := a.remoteTransferSelectedTarget()
	if !ok {
		a.remoteTransferErr = "请选择已连接的目标服务器"
		return
	}
	targetDir := path.Clean(strings.TrimSpace(a.remoteTransferTargetDir))
	if targetDir == "." || !strings.HasPrefix(targetDir, "/") {
		a.remoteTransferErr = "目标目录必须是绝对路径，例如 /srv/app"
		return
	}
	conflict := transferConflictValue(a.remoteTransferConflictChoice)
	a.status = "已加入远程传输队列"
	a.enqueueRemoteTransfer(
		source.ID, target.termID,
		a.remoteTransferSourcePath, targetDir,
		a.remoteTransferSourceDir, conflict,
	)
	a.remoteTransferOpen = false
	a.remoteTransferErr = ""
}

func (a *App) remoteTransferModal(c *ui.Context, t *ui.Theme) {
	targets := a.remoteTransferTargets(a.remoteTransferSourceID)
	a.remoteTransferChooseDefaultTarget()
	options := make([]string, 0, len(targets))
	for _, target := range targets {
		options = append(options, target.label)
	}
	sourceTerm := a.termSessionByID(a.remoteTransferSourceID)
	sourceServer := ""
	if sourceTerm != nil {
		_, sourceServer = a.transferTaskIdentity(sourceTerm.ID)
		if sourceServer == "" {
			sourceServer = transferServerFromSSHProfile(sourceTerm.profile)
		}
	}
	if sourceServer == "" {
		sourceServer = fmt.Sprintf("会话 #%d", a.remoteTransferSourceID)
	}

	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}
	ui.DialogBase(c, &a.remoteTransferOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(fitDialogWidth(c, 620)).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Row(c).Height(64).Shrink(0).Padding(0, 18).Gap(10).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
				ui.Box(c).Size(30, 30).Radius(8).
					Background(colViolet.Alpha(0.1)).Border(1, colViolet.Alpha(0.24)).Center().Children(func() {
					iconEl(c, iconServer, 15, colViolet)
				})
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, "发送到其他服务器").FontSize(14.5).FontWeight(650)
					ui.Text(c, "在两个已连接的 SSH 服务器之间流式复制，不先保存到本机").
						FontSize(9.5).TextColor(colMuted).NoWrap()
				})
				if iconButton(c, t, "remote-transfer-close", iconClose, "关闭").Clicked() {
					a.remoteTransferOpen = false
				}
			})

			ui.Column(c).FillWidth().Padding(16, 18).Gap(12).Children(func() {
				ui.Box(c).FillWidth().Padding(10).Radius(8).Background(colPanelTop).
					Border(1, colLine).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						iconEl(c, iconSync, 13, colCyan)
						ui.Column(c).Grow(1).Gap(3).Children(func() {
							ui.Text(c, "源服务器 · "+shortLabel(sourceServer, 58)).FontSize(9.5).
								FontWeight(600).TextColor(colText).NoWrap()
							ui.Text(c, shortLabel(a.remoteTransferSourcePath, 86)).FontSize(8.6).
								Font("Cascadia Code").TextColor(colMuted2).NoWrap()
						})
						miniTag(c, condText(a.remoteTransferSourceDir, "文件夹", "文件"),
							condColor(a.remoteTransferSourceDir, colAmber, colCyan))
					})
				})

				fieldColumn(c, "目标服务器", func() {
					if len(options) == 0 {
						ui.Box(c).FillWidth().Padding(8, 10).Radius(6).
							Background(colAmber.Alpha(0.08)).Border(1, colAmber.Alpha(0.2)).Children(func() {
							ui.Text(c, "没有其他已连接的 SSH 标签，请先打开目标服务器").
								FontSize(9).TextColor(colAmber).NoWrap()
						})
						return
					}
					ui.Select(c, &a.remoteTransferTargetChoice, options).FillWidth()
				})

				fieldColumn(c, "目标目录", func() {
					ui.TextInput(c, &a.remoteTransferTargetDir).Label("目标目录").
						Placeholder("例如 /srv/app")
				})

				fieldColumn(c, "同名冲突", func() {
					ui.Select(c, &a.remoteTransferConflictChoice,
						[]string{"覆盖", "跳过", "重命名"}).FillWidth()
				})

				ui.Text(c, "文件夹会保留目录结构；已完成的文件会跳过，未完成文件从远端临时文件继续。").
					FontSize(8.8).TextColor(colMuted2).LineHeight(1.45)
				if sourceTerm != nil {
					if target, ok := a.remoteTransferSelectedTarget(); ok &&
						(sourceTerm.profileID == 0 || target.profileID == 0) {
						ui.Text(c, "源或目标服务器尚未保存，应用重启后该任务不能继续。").
							FontSize(8.8).TextColor(colAmber).LineHeight(1.45)
					}
				}
				if a.remoteTransferErr != "" {
					ui.Box(c).FillWidth().Padding(8, 9).Radius(6).
						Background(colRed.Alpha(0.07)).Border(1, colRed.Alpha(0.18)).Children(func() {
						ui.Text(c, a.remoteTransferErr).FontSize(9).TextColor(colRed).LineHeight(1.4)
					})
				}
			})

			ui.Row(c).Height(54).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
				ui.Text(c, "任务会进入传输队列，可暂停、继续或重试").
					FontSize(8.8).TextColor(colMuted2).NoWrap()
				ui.Text(c, "").Grow(1)
				if ghostAction(c, t, "remote-transfer-cancel", "取消", nil).Clicked() {
					a.remoteTransferOpen = false
				}
				if primaryAction(c, "remote-transfer-submit", "开始发送", iconSync).
					Disabled(len(options) == 0).Clicked() {
					a.submitRemoteTransfer()
				}
			})
		})
	})
}

func condText(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

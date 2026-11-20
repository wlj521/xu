package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func (a *App) backupImportMenu(m *ui.Menu) {
	if m.Item("Xu 备份或 OpenSSH config…").Chosen() {
		a.openBackupImporter()
	}
	if m.Item("FinalShell 导出…").Chosen() {
		a.importFinalShell()
	}
}

func (a *App) confirmBackupImport() {
	if !a.backupOpen {
		return
	}
	a.backupOpen = false
	a.applyBackupPreview()
}

func (a *App) backupModal(c *ui.Context, t *ui.Theme) {
	if !a.backupOpen {
		return
	}
	shade := ui.Hex("#000000").Alpha(0.6)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.32)
	}
	w, h := fitDialog(c, 640, 560)
	backup := a.backupPreview
	collisions := backupCollisionCount(a.profiles, backup.Profiles)

	ui.DialogBase(c, &a.backupOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(w).Height(h).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Column(c).FillHeight().Children(func() {
				a.backupHeader(c, t)
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).FillWidth().Padding(16, 18).Gap(11).Children(func() {
						a.backupPreviewBody(c, backup, collisions)
					})
				})
				a.backupFooter(c, t)
			})
		})
	})
}

func (a *App) backupHeader(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(64).Shrink(0).Padding(0, 18).Gap(11).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Box(c).Size(30, 30).Radius(8).
			Background(colCyan.Alpha(0.1)).Border(1, colCyan.Alpha(0.26)).Center().Children(func() {
			iconEl(c, iconDownload, 15, colCyan)
		})
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, "导入 "+a.backupSource).FontSize(14.5).FontWeight(650)
			ui.Text(c, filepath.Base(a.backupPath)).FontSize(9.5).Font("Cascadia Code").
				TextColor(colMuted).NoWrap()
		})
		if iconButton(c, t, "backup-close", iconClose, "关闭").Clicked() {
			a.backupOpen = false
		}
	})
}

func (a *App) backupPreviewBody(c *ui.Context, backup xuBackup, collisions int) {
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		backupCount(c, "连接", len(backup.Profiles), colCyan)
		backupCount(c, "分组", len(backup.Groups), colViolet)
		backupCount(c, "快捷命令", len(backup.QuickCommands), colGreen)
		backupCount(c, "转发规则", len(backup.Forwards), colAmber)
	})

	ui.Row(c).FillWidth().Padding(9, 10).Gap(7).Radius(7).
		Background(colPanel2).Border(1, colLine).AlignItems(ui.Start).Children(func() {
		iconEl(c, iconShield, 12, colGreen)
		ui.Text(c, "备份不包含密码和 known_hosts；密码连接导入后需要在连接时重新输入。").
			FontSize(9.5).TextColor(colMuted).LineHeight(1.45)
	})

	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, "同名或同主机连接").FontSize(11.8).FontWeight(600).TextColor(colText2)
			ui.Text(c, fmt.Sprintf("%d 个连接与现有记录冲突", collisions)).
				FontSize(9.8).TextColor(colMuted)
		})
		ui.Select(c, &a.backupConflictPick, []string{"跳过", "覆盖", "重命名"}).Width(110)
	})

	if len(backup.Profiles) > 0 {
		ui.Column(c).FillWidth().Gap(5).Children(func() {
			ui.Text(c, "预览").FontSize(9.2).FontWeight(650).LetterSpacing(0.7).TextColor(colMuted2)
			limit := len(backup.Profiles)
			if limit > 5 {
				limit = 5
			}
			for i := 0; i < limit; i++ {
				p := backup.Profiles[i]
				ui.Row(c).FillWidth().Height(34).Padding(0, 9).Radius(7).
					Background(colPanel2.Alpha(0.5)).Gap(8).AlignItems(ui.Center).Children(func() {
					iconEl(c, iconServer, 12, colCyan)
					ui.Column(c).Grow(1).Gap(1).Children(func() {
						ui.Text(c, p.Name).FontSize(10.5).FontWeight(550).TextColor(colText2).NoWrap()
						ui.Text(c, fmt.Sprintf("%s@%s:%d", p.Username, p.Host, p.Port)).
							FontSize(8.5).Font("Cascadia Code").TextColor(colMuted2).NoWrap()
					})
					if p.Group != "" {
						miniTag(c, p.Group, colMuted2)
					}
				})
			}
			if len(backup.Profiles) > limit {
				ui.Text(c, fmt.Sprintf("还有 %d 个连接…", len(backup.Profiles)-limit)).
					FontSize(9).TextColor(colMuted2)
			}
		})
	}

	if a.backupSource == "Xu 备份" && len(backup.Settings) > 0 {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "同时恢复应用设置").FontSize(11).FontWeight(550).TextColor(colText2)
				ui.Text(c, fmt.Sprintf("主题、终端、传输等 %d 项；不包含工作区标签。", len(backup.Settings))).
					FontSize(9.3).TextColor(colMuted)
			})
			ui.Switch(c, &a.backupIncludeSettings)
		})
	}

	if len(a.backupWarnings) > 0 {
		ui.Box(c).FillWidth().Padding(9, 10).Radius(7).
			Background(colAmber.Alpha(0.07)).Border(1, colAmber.Alpha(0.2)).Children(func() {
			ui.Text(c, strings.Join(a.backupWarnings, "；")).
				FontSize(9.5).TextColor(colAmber).LineHeight(1.45)
		})
	}

}

func backupCount(c *ui.Context, label string, count int, color ui.Color) {
	ui.Column(c).Grow(1).Height(54).Padding(8, 10).Radius(8).Gap(1).
		Background(color.Alpha(0.06)).Border(1, color.Alpha(0.18)).Children(func() {
		ui.Text(c, fmt.Sprintf("%d", count)).FontSize(16).FontWeight(650).TextColor(color)
		ui.Text(c, label).FontSize(8.5).TextColor(colMuted2).NoWrap()
	})
}

func (a *App) backupFooter(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(56).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		iconEl(c, iconShield, 11, colGreen)
		ui.Text(c, "导入前请确认来源可信").FontSize(9).TextColor(colMuted2).NoWrap()
		ui.Text(c, "").Grow(1)
		if ghostAction(c, t, "backup-cancel", "取消", nil).Clicked() {
			a.backupOpen = false
		}
		if primaryAction(c, "backup-apply", "开始导入", iconDownload).Clicked() {
			a.confirmBackupImport()
		}
	})
}

func backupCollisionCount(existing []store.Profile, imported []backupProfile) int {
	byName := make(map[string]bool, len(existing))
	byEndpoint := make(map[string]bool, len(existing))
	for _, profile := range existing {
		byName[strings.ToLower(profile.Name)] = true
		byEndpoint[fsProfileKey(profile.Host, profile.Port, profile.Username)] = true
	}
	count := 0
	for _, profile := range imported {
		name := strings.ToLower(strings.TrimSpace(profile.Name))
		if name == "" {
			name = strings.ToLower(profile.Username + "@" + profile.Host)
		}
		endpoint := fsProfileKey(profile.Host, profile.Port, profile.Username)
		if byName[name] || byEndpoint[endpoint] {
			count++
		}
		byName[name] = true
		byEndpoint[endpoint] = true
	}
	return count
}

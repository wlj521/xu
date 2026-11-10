package app

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

var chmodBits = [...]os.FileMode{
	0o400, 0o200, 0o100,
	0o040, 0o020, 0o010,
	0o004, 0o002, 0o001,
}

func chmodChecks(mode os.FileMode) [9]bool {
	var checks [9]bool
	for i, bit := range chmodBits {
		checks[i] = mode&bit != 0
	}
	return checks
}

func fileModeFromChecks(checks [9]bool) os.FileMode {
	var mode os.FileMode
	for i, bit := range chmodBits {
		if checks[i] {
			mode |= bit
		}
	}
	return mode
}

func (a *App) openChmodDialog(b *fileBrowser, remote bool) {
	var f os.FileInfo
	if remote {
		f = selectedEntry(b.remoteEntries, b.remoteSel)
	} else {
		f = selectedEntry(b.localEntries, b.localSel)
	}
	if f == nil {
		a.fileErr = "请先选择一个文件"
		return
	}
	if remote && b.client == nil {
		a.fileErr = "SSH 连接不可用"
		return
	}

	a.promptOpen = false
	a.promptAction = nil
	a.fileErr = ""
	a.chmodOpen = true
	a.chmodBrowser = b
	a.chmodName = f.Name()
	a.chmodRemote = remote
	a.chmodMode = chmodChecks(f.Mode().Perm())
	a.chmodErr = ""
}

func (a *App) chmodModal(c *ui.Context, t *ui.Theme) {
	if !a.chmodOpen {
		return
	}

	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}

	ui.DialogBase(c, &a.chmodOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(fitDialogWidth(c, 440)).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			a.chmodHeader(c)
			a.chmodBody(c)
			a.chmodFooter(c, t)
		})
	})
}

func (a *App) chmodHeader(c *ui.Context) {
	ui.Row(c).Height(62).Shrink(0).Padding(0, 18).Gap(11).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Box(c).Size(30, 30).Radius(8).
			Background(colCyan.Alpha(0.1)).Border(1, colCyan.Alpha(0.26)).Center().Children(func() {
			iconEl(c, iconLock, 15, colCyan)
		})
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, "修改文件权限").FontSize(14.5).FontWeight(650)
			ui.Text(c, "所有者 / 组 / 其他").FontSize(9.5).TextColor(colMuted).NoWrap()
		})
		close := ui.ButtonBase(c.Key("chmod-close")).Size(26, 26).Radius(6).
			Center().Tooltip("关闭").Label("关闭")
		if close.Hovered() {
			close.Background(colPanel2)
		}
		close.Children(func() {
			iconEl(c, iconClose, 13, colMuted)
		})
		if close.Clicked() {
			a.chmodOpen = false
			a.chmodErr = ""
		}
	})
}

func (a *App) chmodBody(c *ui.Context) {
	ui.Column(c).Padding(14, 18).Gap(10).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, a.chmodName).FontSize(12).FontWeight(600).
				Font("Cascadia Code").TextColor(colText).NoWrap()
			ui.Text(c, "勾选每一类用户拥有的权限").FontSize(9.3).TextColor(colMuted)
		})
		a.chmodGroup(c, "所有者", 0)
		a.chmodGroup(c, "组", 3)
		a.chmodGroup(c, "其他", 6)
		if a.chmodErr != "" {
			ui.Box(c).FillWidth().Padding(8, 10).Radius(7).
				Background(colRed.Alpha(0.08)).Border(1, colRed.Alpha(0.2)).Children(func() {
				ui.Text(c, a.chmodErr).FontSize(9.3).TextColor(colRed).LineHeight(1.4)
			})
		}
	})
}

func (a *App) chmodGroup(c *ui.Context, label string, start int) {
	ui.Box(c).FillWidth().Padding(10, 12).Radius(8).
		Background(colPanel2.Alpha(0.72)).Border(1, colLine).Children(func() {
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, label).FontSize(10).FontWeight(650).TextColor(colText2)
			ui.Row(c).Gap(18).AlignItems(ui.Center).Children(func() {
				for i, name := range []string{"读取", "写入", "执行"} {
					ui.Checkbox(c.Key(fmt.Sprintf("chmod-%d-%d", start, i)),
						&a.chmodMode[start+i], name).FontSize(10.5).TextColor(colText2)
				}
			})
		})
	})
}

func (a *App) chmodFooter(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(56).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		ui.Text(c, fmt.Sprintf("权限 %03o", fileModeFromChecks(a.chmodMode))).
			FontSize(9.5).Font("Cascadia Code").TextColor(colMuted)
		ui.Text(c, "").Grow(1)
		if ghostAction(c, t, "chmod-cancel", "取消", nil).Clicked() {
			a.chmodOpen = false
			a.chmodErr = ""
		}
		if primaryAction(c, "chmod-apply", "确定", iconCheck).Clicked() {
			a.applyChmod()
		}
	})
}

func (a *App) applyChmod() {
	b := a.chmodBrowser
	if b == nil || a.chmodName == "" {
		a.chmodErr = "文件面板已关闭，请重试"
		return
	}

	mode := fileModeFromChecks(a.chmodMode)
	var err error
	if a.chmodRemote {
		if b.client == nil {
			err = fmt.Errorf("SSH 连接不可用")
		} else {
			err = b.client.Chmod(path.Join(b.remoteDir, a.chmodName), mode)
		}
	} else {
		err = os.Chmod(filepath.Join(b.localDir, a.chmodName), mode.Perm())
	}
	if err != nil {
		a.chmodErr = "修改失败: " + err.Error()
		return
	}

	name, remote := a.chmodName, a.chmodRemote
	a.chmodOpen = false
	a.chmodErr = ""
	a.chmodBrowser = nil
	if remote {
		b.loadRemote()
	} else {
		b.loadLocal()
	}
	a.status = fmt.Sprintf("权限已修改：%s → %03o", name, mode.Perm())
}

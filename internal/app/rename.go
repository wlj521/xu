package app

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *App) promptRename(b *fileBrowser, remote bool) {
	var f os.FileInfo
	if remote {
		f = selectedEntry(b.remoteEntries, b.remoteSel)
	} else {
		f = selectedEntry(b.localEntries, b.localSel)
	}
	if f == nil {
		a.fileErr = "请先选择一个文件或文件夹"
		return
	}
	if remote && b.client == nil {
		a.fileErr = "SSH 连接不可用"
		return
	}

	a.closeNewEntry()
	a.fileErr = ""
	a.renameOpen = true
	a.renameBrowser = b
	a.renameRemote = remote
	a.renameOld = f.Name()
	a.renameName = f.Name()
	a.renameIsDir = f.IsDir()
	a.renameErr = ""
	a.renamePick = true
}

func (a *App) closeRename() {
	a.renameOpen = false
	a.renameBrowser = nil
	a.renameRemote = false
	a.renameOld = ""
	a.renameName = ""
	a.renameIsDir = false
	a.renameErr = ""
	a.renamePick = false
}

func (a *App) renameModal(c *ui.Context, t *ui.Theme) {
	if !a.renameOpen {
		return
	}
	if a.renameBrowser == nil {
		a.closeRename()
		return
	}

	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}

	panel := ui.DialogBase(c, &a.renameOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(fitDialogWidth(c, 460)).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			a.renameHeader(c)
			a.renameBody(c)
			a.renameFooter(c, t)
		})
	})
	if panel.Dismissed() {
		a.closeRename()
	}
}

func (a *App) renameHeader(c *ui.Context) {
	title, where, icon := "重命名文件", "本机磁盘", iconFile
	if a.renameIsDir {
		title, icon = "重命名文件夹", iconFolder
	}
	if a.renameRemote {
		where = "远端 · SFTP"
	}

	ui.Row(c).Height(62).Shrink(0).Padding(0, 18).Gap(11).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Box(c).Size(30, 30).Radius(8).
			Background(colCyan.Alpha(0.1)).Border(1, colCyan.Alpha(0.26)).Center().Children(func() {
			iconEl(c, icon, 15, colCyan)
		})
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(14.5).FontWeight(650)
			ui.Text(c, where).FontSize(9.5).TextColor(colMuted).NoWrap()
		})
	})
}

func (a *App) renameBody(c *ui.Context) {
	b := a.renameBrowser
	dir := b.localDir
	if a.renameRemote {
		dir = b.remoteDir
	}

	ui.Column(c).Padding(16, 18).Gap(10).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, a.renameOld).FontSize(12).FontWeight(600).
				Font("Cascadia Code").TextColor(colText).NoWrap()
			ui.Text(c, "位于 "+shortenMiddle(dir, 56)).FontSize(9.5).
				Font("Cascadia Code").TextColor(colMuted2).NoWrap()
		})

		input := ui.TextInput(c.Key("rename-name"), &a.renameName).
			FillWidth().Height(32).Font("Cascadia Code").FontSize(11).
			Placeholder("新名称").AutoFocus()
		if a.renamePick {
			input.SetTextSelection(0, len([]rune(a.renameName)))
			a.renamePick = false
		}
		if input.Submitted() {
			a.submitRename()
		}
		if a.renameErr != "" {
			ui.Box(c).FillWidth().Padding(8, 10).Radius(7).
				Background(colRed.Alpha(0.08)).Border(1, colRed.Alpha(0.2)).Children(func() {
				ui.Text(c, a.renameErr).FontSize(9.3).TextColor(colRed).LineHeight(1.4)
			})
		}
	})
}

func (a *App) renameFooter(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(56).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		iconEl(c, iconEdit, 11, colMuted2)
		ui.Text(c, "名称不能为空，且不能包含 / 或 \\").FontSize(9).TextColor(colMuted2).NoWrap()
		ui.Text(c, "").Grow(1)
		if ghostAction(c, t, "rename-cancel", "取消", nil).Clicked() {
			a.closeRename()
		}
		if primaryAction(c, "rename-submit", "重命名", iconCheck).Clicked() {
			a.submitRename()
		}
	})
}

func (a *App) submitRename() {
	b := a.renameBrowser
	if b == nil {
		a.renameErr = "文件面板已关闭，请重试"
		return
	}

	name := strings.TrimSpace(a.renameName)
	if !validName(name) {
		a.renameErr = "名称不能为空，且不能包含 / 或 \\"
		return
	}
	if name == a.renameOld {
		a.renameErr = "名称未改变"
		return
	}

	var err error
	if a.renameRemote {
		if b.client == nil {
			err = fmt.Errorf("SSH 连接不可用")
		} else {
			err = b.client.Rename(
				path.Join(b.remoteDir, a.renameOld),
				path.Join(b.remoteDir, name),
			)
		}
	} else {
		err = os.Rename(
			filepath.Join(b.localDir, a.renameOld),
			filepath.Join(b.localDir, name),
		)
	}
	if err != nil {
		a.renameErr = "重命名失败: " + err.Error()
		return
	}

	oldName, remote := a.renameOld, a.renameRemote
	a.closeRename()
	if remote {
		b.loadRemote()
	} else {
		b.loadLocal()
	}
	a.status = fmt.Sprintf("已重命名：%s → %s", oldName, name)
}

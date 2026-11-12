package app

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// New file and folder creation ask in the app's own dialog over the file
// panel, as the delete question does. The inline bar stays for renaming,
// where the old name is already there to edit.

func (a *App) promptNewFile(b *fileBrowser, remote bool) { a.openNewEntry(b, remote, false) }

func (a *App) promptMkdir(b *fileBrowser, remote bool) { a.openNewEntry(b, remote, true) }

func (a *App) openNewEntry(b *fileBrowser, remote, dir bool) {
	if remote && b.client == nil {
		a.fileErr = "SSH 连接不可用"
		return
	}

	// One prompt at a time: creating replaces the inline rename bar.
	a.promptOpen = false
	a.promptAction = nil
	a.fileErr = ""
	a.newEntryOpen = true
	a.newEntryBrowser = b
	a.newEntryRemote = remote
	a.newEntryDir = dir
	a.newEntryName = ""
	a.newEntryErr = ""
}

func (a *App) closeNewEntry() {
	a.newEntryOpen = false
	a.newEntryBrowser = nil
	a.newEntryName = ""
	a.newEntryErr = ""
}

func (a *App) newEntryModal(c *ui.Context, t *ui.Theme) {
	if !a.newEntryOpen {
		return
	}
	if a.newEntryBrowser == nil {
		a.closeNewEntry()
		return
	}

	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}

	ui.DialogBase(c, &a.newEntryOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(fitDialogWidth(c, 460)).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			a.newEntryHeader(c)
			a.newEntryBody(c)
			a.newEntryFooter(c, t)
		})
	})
}

func (a *App) newEntryHeader(c *ui.Context) {
	title, where, icon := "新建文件", "本机磁盘", iconFile
	if a.newEntryRemote {
		where = "远端 · SFTP"
	}
	if a.newEntryDir {
		title, icon = "新建文件夹", iconNewFolder
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

func (a *App) newEntryBody(c *ui.Context) {
	dir := a.newEntryBrowser.localDir
	if a.newEntryRemote {
		dir = a.newEntryBrowser.remoteDir
	}

	ui.Column(c).Padding(16, 18).Gap(10).Children(func() {
		ui.Text(c, "位于 "+shortenMiddle(dir, 56)).FontSize(9.5).
			Font("Cascadia Code").TextColor(colMuted2).NoWrap()
		input := ui.TextInput(c.Key("new-entry-name"), &a.newEntryName).
			FillWidth().Height(32).Font("Cascadia Code").FontSize(11).
			Placeholder("名称").AutoFocus()
		if input.Submitted() {
			a.submitNewEntry()
		}
		if a.newEntryErr != "" {
			ui.Box(c).FillWidth().Padding(8, 10).Radius(7).
				Background(colRed.Alpha(0.08)).Border(1, colRed.Alpha(0.2)).Children(func() {
				ui.Text(c, a.newEntryErr).FontSize(9.3).TextColor(colRed).LineHeight(1.4)
			})
		}
	})
}

func (a *App) newEntryFooter(c *ui.Context, t *ui.Theme) {
	icon := iconFile
	if a.newEntryDir {
		icon = iconNewFolder
	}
	ui.Row(c).Height(56).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		iconEl(c, icon, 11, colMuted2)
		ui.Text(c, "名称不能为空，且不能包含 / 或 \\").FontSize(9).TextColor(colMuted2).NoWrap()
		ui.Text(c, "").Grow(1)
		if ghostAction(c, t, "new-entry-cancel", "取消", nil).Clicked() {
			a.closeNewEntry()
		}
		if primaryAction(c, "new-entry-create", "创建", iconCheck).Clicked() {
			a.submitNewEntry()
		}
	})
}

func (a *App) submitNewEntry() {
	b := a.newEntryBrowser
	if b == nil {
		a.newEntryErr = "文件面板已关闭，请重新打开"
		return
	}

	name := strings.TrimSpace(a.newEntryName)
	if !validName(name) {
		a.newEntryErr = "名称不能为空，且不能包含 / 或 \\"
		return
	}

	var err error
	if a.newEntryRemote {
		if b.client == nil {
			a.newEntryErr = "SSH 连接不可用"
			return
		}
		target := path.Join(b.remoteDir, name)
		if a.newEntryDir {
			err = b.client.Mkdir(target)
		} else {
			var f interface{ Close() error }
			f, err = b.client.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
			if err == nil {
				err = f.Close()
			}
		}
	} else {
		target := filepath.Join(b.localDir, name)
		if a.newEntryDir {
			err = os.Mkdir(target, 0o755)
		} else {
			var f *os.File
			f, err = os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err == nil {
				err = f.Close()
			}
		}
	}
	if err != nil {
		a.newEntryErr = "创建失败: " + err.Error()
		return
	}

	isDir, remote := a.newEntryDir, a.newEntryRemote
	a.closeNewEntry()
	if isDir {
		a.status = "已新建文件夹 " + name
	} else {
		a.status = "已新建文件 " + name
	}
	if remote {
		b.loadRemote()
	} else {
		b.loadLocal()
	}
}

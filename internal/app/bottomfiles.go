package app

import (
	"fmt"
	"path"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *App) toggleBottomFiles() {
	a.toggleDock()
}

func (a *App) bottomFilesView(c *ui.Context, t *ui.Theme, ts *termSession, b *fileBrowser) {
	ui.Column(c).FillHeight().Background(colPanel).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		ui.Row(c).Height(6).Shrink(0).Center().Children(func() {
			ui.Box(c).Width(34).Height(2).Radius(1).Background(colLine2)
		})
		a.bottomFilesHead(c, t, b)
		switch a.bottomFileTab {
		case 1:
			a.bottomTransfers(c, t)
		case 2:
			a.bottomCommands(c, t)
		default:
			a.bottomFilesPane(c, t, b)
		}
	})
}

func (a *App) bottomFilesHead(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	ui.Row(c).Height(43).Shrink(0).Padding(0, 9).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		for i, label := range []string{"文件", "传输", "命令"} {
			i, label := i, label
			active := a.bottomFileTab == i
			btn := ui.ButtonBase(c.Key(fmt.Sprintf("bf-tab-%d", i))).Height(28).Padding(0, 11).Radius(7).Center()
			if active {
				btn.Background(colPanel2)
			} else if btn.Hovered() {
				btn.Background(t.SurfaceHover)
			}
			btn.Children(func() {
				ui.Text(c, label).FontSize(11).TextColor(condColor(active, colText, colMuted)).NoWrap()
			})
			if btn.Clicked() {
				a.bottomFileTab = i
			}
		}

		ui.Text(c, "").Grow(1)

		if iconButton(c, t, "bf-refresh", iconRefresh, "刷新").Clicked() {
			b.loadRemote()
		}
		if iconButton(c, t, "bf-upload", iconUpload, "上传").Clicked() {
			a.uploadViaDialog(b)
		}
		if iconButton(c, t, "bf-download", iconDownload, "下载").Clicked() {
			a.downloadSelected(b)
		}
		if iconButton(c, t, "bf-newfolder", iconNewFolder, "新建文件夹").Clicked() {
			a.promptMkdir(b, true)
		}
		if iconButton(c, t, "bf-expand", iconExpand, "展开完整文件管理").Clicked() {
			a.filesOpen = true
		}
		if iconButton(c, t, "bf-collapse", iconChevron, "收起文件面板").Clicked() {
			a.bottomFiles = false
		}

		more := iconButton(c, t, "bf-more", iconMore, "更多")
		more.Menu(func(m *ui.Menu) {
			if m.Item("上级目录").Chosen() {
				a.navigateRemote(b, path.Dir(b.remoteDir))
			}
			if m.Item("刷新").Shortcut(ui.Cmd, ui.KeyR).Chosen() {
				b.loadRemote()
			}
			m.Separator()
			if m.Item("重命名").Chosen() {
				a.promptRename(b, true)
			}
			if m.Item("删除").Chosen() {
				a.deleteSelected(b, true)
			}
			m.Separator()
			if m.Item("上传文件…").Chosen() {
				a.uploadViaDialog(b)
			}
			if m.Item("下载所选").Chosen() {
				a.downloadSelected(b)
			}
		})
	})
}

func (a *App) bottomFilesPane(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	ui.Row(c).FillHeight().AlignItems(ui.Stretch).Children(func() {
		a.bottomFileTree(c, t, b)

		ui.Column(c).FillHeight().Children(func() {
			a.bottomPathBar(c, t, b)

			if a.promptOpen {
				a.promptBar(c)
			}
			if a.transfer != nil {
				a.transferBar(c, t)
			}
			if a.fileErr != "" {
				ui.Text(c, shortLabel(a.fileErr, 100)).FontSize(9).TextColor(colRed).Padding(2, 12)
			}
			if b.remoteErr != "" {
				ui.Text(c, shortLabel(b.remoteErr, 100)).FontSize(9).TextColor(colRed).Padding(2, 12)
			}

			if ui.Table(c, &b.remoteState, bottomFileColumns(), len(b.remoteEntries), func(row, col int) {
				bottomRemoteCell(c, b, row, col)
			}).Grow(1).Submitted() {
				a.openRemoteSelected(b)
			}
		})
	})
}

func (a *App) bottomPathBar(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	ui.Row(c).Height(30).Shrink(0).Padding(0, 8).Gap(2).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Background(colPanel).Children(func() {
		up := ui.ButtonBase(c.Key("bf-path-up")).Size(22, 22).Center().Radius(4).
			Tooltip("上级目录").Disabled(b.remoteDir == "/")
		if up.Hovered() && b.remoteDir != "/" {
			up.Background(t.SurfaceHover)
		}
		up.Children(func() {
			iconEl(c, iconFolderUp, 12, condColor(b.remoteDir == "/", colMuted2, colMuted))
		})
		if up.Clicked() {
			a.navigateRemote(b, path.Dir(b.remoteDir))
		}

		a.pathCrumb(c, t, b, "/", "/", b.remoteDir == "/")
		if b.remoteDir != "/" {
			acc := "/"
			segs := strings.Split(strings.Trim(b.remoteDir, "/"), "/")
			for i, seg := range segs {
				if seg == "" {
					continue
				}
				acc = path.Join(acc, seg)
				ui.Text(c, "›").FontSize(9).TextColor(colMuted2)
				a.pathCrumb(c, t, b, shortLabel(seg, 18), acc, i == len(segs)-1)
			}
		}

		ui.Text(c, "").Grow(1)
		ui.Text(c, fmt.Sprintf("%d 项", len(b.remoteEntries))).FontSize(8.5).
			Font("Cascadia Code").TextColor(colMuted2).NoWrap()
	})
}

func (a *App) pathCrumb(c *ui.Context, t *ui.Theme, b *fileBrowser, label, target string, active bool) {
	btn := ui.ButtonBase(c.Key("bf-crumb-"+target)).Height(20).Padding(0, 4).Radius(3).
		Center().Tooltip(target)
	if active {
		btn.Background(colCyan.Alpha(0.08))
	} else if btn.Hovered() {
		btn.Background(t.SurfaceHover)
	}
	btn.Children(func() {
		ui.Text(c, label).FontSize(9.5).Font("Cascadia Code").
			TextColor(condColor(active, colCyan, colMuted)).NoWrap()
	})
	if btn.Clicked() && !active {
		a.navigateRemote(b, target)
	}
}

func (a *App) bottomFileTree(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	ui.Column(c).Width(200).FillHeight().Background(colTreeBg).
		BorderWidth(0, 1, 0, 0).BorderColor(colLine).Children(func() {
		ui.Row(c).Height(30).Shrink(0).Padding(0, 9).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
			iconEl(c, iconFolder, 12, colMuted2)
			ui.Text(c, "目录").FontSize(9.5).FontWeight(650).LetterSpacing(0.4).
				TextColor(colMuted2).NoWrap()
			ui.Text(c, "").Grow(1)
			label := "SFTP"
			if ts := a.activeTerminal(); ts != nil {
				label = shortLabel(ts.Title, 22)
			}
			ui.Text(c, label).FontSize(8.5).Font("Cascadia Code").
				TextColor(colMuted2).NoWrap()
		})

		ui.Column(c).Grow(1).Padding(5).Children(func() {
			ui.Scroll(c).Grow(1).Gap(1).Children(func() {
				a.treeDirRow(c, t, b, "/", 0)
			})
		})
	})
}

func (a *App) treeDirRow(c *ui.Context, t *ui.Theme, b *fileBrowser, dir string, depth int) {
	kids := b.treeKids[dir]
	loaded := b.treeLoaded[dir]
	open := b.treeOpen[dir]
	active := b.remoteDir == dir
	hasKids := !loaded || len(kids) > 0

	ui.Row(c).FillWidth().Height(24).Gap(1).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(float32(depth) * 12).Shrink(0)

		if hasKids {
			twisty := ui.ButtonBase(c.Key("bf-tw-"+dir)).Size(16, 16).Center().Radius(4).
				Tooltip(condLabel(open, "收起源目录", "展开源目录"))
			if twisty.Hovered() {
				twisty.Background(t.SurfaceHover)
			}
			twisty.Children(func() {
				icon := iconChevronRight
				if open {
					icon = iconChevron
				}
				iconEl(c, icon, 9, colMuted2)
			})
			if twisty.Clicked() {
				if open {
					b.treeOpen[dir] = false
				} else {
					b.treeOpen[dir] = true
					b.loadTreeDir(dir)
				}
			}
		} else {
			ui.Box(c).Width(16).Shrink(0)
		}

		row := ui.ButtonBase(c.Key("bf-tree-"+dir)).Grow(1).Height(22).
			Padding(0, 4).Radius(4).Gap(6).AlignItems(ui.Center)
		if active {
			row.Background(colCyan.Alpha(0.08))
		} else if row.Hovered() {
			row.Background(t.SurfaceHover)
		}
		row.Children(func() {
			color := colAmber
			if active {
				color = colCyan
			}
			iconEl(c, iconFolder, 12, color)
			label := path.Base(dir)
			if dir == "/" {
				label = "/"
			}
			ui.Text(c, shortLabel(label, 22)).FontSize(10.5).
				TextColor(condColor(active, colCyan, colMuted)).NoWrap()
		})
		if row.Clicked() {
			a.navigateRemote(b, dir)
		}
	})

	if open {
		if msg := b.treeErr[dir]; msg != "" {
			ui.Text(c, shortLabel(msg, 34)).FontSize(8.5).TextColor(colRed).
				Padding(1, 8, 2, float32(depth)*12+24).NoWrap()
		}
		for _, kid := range kids {
			a.treeDirRow(c, t, b, kid, depth+1)
		}
	}
}

func condLabel(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

func (a *App) bottomCommands(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Fill().Center().Gap(6).Padding(20).Children(func() {
		iconEl(c, iconHistory, 24, colMuted2)
		ui.Text(c, "命令历史").FontSize(11).TextColor(colMuted)
		if len(a.commandHistory) == 0 {
			ui.Text(c, "在终端下方的命令栏输入命令，回车即可执行").FontSize(9).TextColor(colMuted2)
			return
		}
		ui.Column(c).Width(420).Gap(2).Children(func() {
			for i := len(a.commandHistory) - 1; i >= 0 && i >= len(a.commandHistory)-6; i-- {
				cmd := a.commandHistory[i]
				btn := ui.ButtonBase(c.Key("hist-"+cmd)).Height(26).Padding(0, 8).Radius(6).Gap(6)
				if btn.Hovered() {
					btn.Background(t.SurfaceHover)
				}
				btn.Children(func() {
					iconEl(c, iconCommand, 11, colMuted2)
					ui.Text(c, shortLabel(cmd, 54)).FontSize(9.5).Font("Cascadia Code").TextColor(colMuted)
				})
				if btn.Clicked() {
					a.commandInput = cmd
					a.bottomFileTab = 0
				}
			}
		})
	})
}

func (a *App) bottomTransfers(c *ui.Context, t *ui.Theme) {
	if a.transfer == nil {
		ui.Column(c).Grow(1).Fill().Center().Gap(6).Padding(20).Children(func() {
			iconEl(c, iconUpload, 24, colMuted2)
			ui.Text(c, "暂无传输任务").FontSize(11).TextColor(colMuted)
			ui.Text(c, "把文件拖进远端目录，或用上方的上传按钮").FontSize(9).TextColor(colMuted2)
		})
		return
	}
	ui.Column(c).Grow(1).Padding(12).Gap(8).Children(func() {
		ui.Text(c, "传输").FontSize(10).Bold().TextColor(colMuted)
		a.transferBar(c, t)
	})
}

func (a *App) uploadViaDialog(b *fileBrowser) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:    "选择要上传的文件",
		Multiple: true,
	})
	if err != nil || len(paths) == 0 {
		return
	}
	a.uploadDropped(b, paths)
}

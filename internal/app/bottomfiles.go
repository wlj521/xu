package app

import (
	"fmt"
	"path"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// bottomFilesPane is the FinalShell-style file dock: a directory tree on the
// left, the breadcrumb and actions on top of the remote file table on the
// right. A single click on a folder row opens it.
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

			wide := a.dockFileTableWide(c)
			cols := bottomFileColumns()
			if wide {
				cols = fileColumns()
			}
			opened := false
			table := ui.Table(c, &b.remoteState, cols, len(b.remoteEntries), func(row, col int) {
				if row < 0 || row >= len(b.remoteEntries) {
					return
				}
				f := b.remoteEntries[row]
				cell := ui.Box(c).Fill().OnClick(func() {
					a.clickRemoteRow(b, row)
				})
				cell.ContextMenu(func(m *ui.Menu) {
					b.remoteSel = row
					a.fileContextMenu(m, b, true)
				})
				if !f.IsDir() && cell.DoubleClicked() {
					b.remoteSel = row
					a.openRemoteSelected(b)
					opened = true
				}
				cell.Children(func() {
					if wide {
						remoteCell(c, b, row, col)
					} else {
						bottomRemoteCell(c, b, row, col)
					}
				})
			}).Grow(1)
			table.ContextMenu(func(m *ui.Menu) {
				a.fileBackgroundMenu(m, b, true)
			})
			// Double clicks are also Submitted; the cell already opened the
			// file, so only the keyboard's Enter opens it here.
			if table.Submitted() && !opened && !a.editorOpen {
				a.openRemoteSelected(b)
			}
		})
	})
}

// dockFileTableWide picks the full column set (type, mode, owner) when the
// window has room, and the compact name/size/date set when it does not.
func (a *App) dockFileTableWide(c *ui.Context) bool {
	w, _ := c.Size()
	return w >= 1100
}

// clickRemoteRow opens a folder on a single click and just chooses a file.
func (a *App) clickRemoteRow(b *fileBrowser, row int) {
	if row < 0 || row >= len(b.remoteEntries) {
		return
	}
	b.remoteSel = row
	if f := b.remoteEntries[row]; f.IsDir() {
		a.navigateRemote(b, path.Join(b.remoteDir, f.Name()))
	}
}

func (a *App) bottomPathBar(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	ui.Row(c).Height(32).Shrink(0).Padding(0, 8).Gap(2).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Background(colPanel).Children(func() {
		up := ui.ButtonBase(c.Key("bf-path-up")).Size(22, 22).Center().Radius(4).
			Tooltip("上级目录").Label("上级目录").Disabled(b.remoteDir == "/")
		if up.Hovered() && b.remoteDir != "/" {
			up.Background(t.SurfaceHover)
		}
		up.Children(func() {
			iconEl(c, iconFolderUp, 12, condColor(b.remoteDir == "/", colMuted2, colMuted))
		})
		if up.Clicked() {
			a.navigateRemote(b, path.Dir(b.remoteDir))
		}

		// The crumbs scroll sideways when a deep path outgrows the bar, so
		// the actions on the right keep their room.
		ui.ScrollHorizontal(c).Grow(1).Height(32).Children(func() {
			ui.Row(c).Height(32).Gap(2).AlignItems(ui.Center).Children(func() {
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
			})
		})

		ui.Text(c, fmt.Sprintf("%d 项", len(b.remoteEntries))).FontSize(8.5).
			Font("Cascadia Code").TextColor(colMuted2).NoWrap()

		ui.Box(c).Width(1).Height(14).Background(colLine).MarginX(5)

		if paneIconButton(c, t, "bf-act-refresh", iconRefresh, "刷新").Clicked() {
			b.loadRemote()
		}
		if paneIconButton(c, t, "bf-act-new", iconNewFolder, "新建文件夹").Clicked() {
			a.promptMkdir(b, true)
		}
		if paneIconButton(c, t, "bf-act-upload", iconUpload, "上传文件").Clicked() {
			a.uploadViaDialog(b)
		}
		if paneIconButton(c, t, "bf-act-download", iconDownload, "下载所选").Clicked() {
			a.downloadSelected(b)
		}
		more := paneIconButton(c, t, "bf-act-more", iconMore, "更多")
		more.Menu(func(m *ui.Menu) {
			if m.Item("新建文件").Chosen() {
				a.promptNewFile(b, true)
			}
			if m.Item("新建文件夹").Chosen() {
				a.promptMkdir(b, true)
			}
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
	ui.Column(c).Width(190).FillHeight().Background(colTreeBg).
		BorderWidth(0, 1, 0, 0).BorderColor(colLine).Children(func() {
		ui.Row(c).Height(32).Shrink(0).Padding(0, 9).Gap(6).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
			iconEl(c, iconFolder, 12, colMuted2)
			ui.Text(c, "目录").FontSize(9.5).FontWeight(650).LetterSpacing(0.4).
				TextColor(colMuted2).NoWrap()
			ui.Text(c, "").Grow(1)
			label := "SFTP"
			if ts := a.activeTerminal(); ts != nil {
				label = shortLabel(ts.Title, 16)
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
				Tooltip(condLabel(open, "收起目录", "展开目录"))
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

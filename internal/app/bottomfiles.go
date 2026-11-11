package app

import (
	"fmt"
	"path"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// bottomFilesPane is the FinalShell-style file dock: a full-width breadcrumb
// bar above a directory tree on the left and the remote file table on the
// right. A single click on a folder row opens it.
func (a *App) bottomFilesPane(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	a.bottomFileContent(c, t, b)
}

func (a *App) bottomFileContent(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	ui.Column(c).FillWidth().FillHeight().Children(func() {
		a.bottomPathBar(c, t, b)

		if a.transfer != nil {
			a.transferBar(c, t)
		}
		if b.remoteErr != "" {
			ui.Text(c, shortLabel(b.remoteErr, 100)).FontSize(9).TextColor(colRed).Padding(2, 12)
		}

		ui.Split(c, &a.fileSplit, func() {
			a.bottomFileTree(c, t, b)
		}, func() {
			a.bottomFileTable(c, t, b)
		}).Grow(1).FillWidth().SplitReset(260)
	})
}

func (a *App) bottomFileTable(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	ui.Column(c).FillWidth().FillHeight().Background(colTreeBg).Children(func() {
		wide := a.dockFileTableWide(c)
		cols := bottomFileColumns()
		if wide {
			cols = fileColumns()
		}
		opened := false
		tableTheme := fileTableTheme(t)
		tableTheme.FontSize = 11.5
		var table ui.Element
		ui.Themed(c, tableTheme).Grow(1).FillWidth().Background(ui.Transparent).Children(func() {
			table = ui.Table(c, &b.remoteState, cols, len(b.remoteEntries), func(row, col int) {
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
			})
			table.Grow(1)
		})
		table.ContextMenu(func(m *ui.Menu) {
			a.fileBackgroundMenu(m, b, true)
		})
		// Double clicks are also Submitted; the cell already opened the
		// file, so only the keyboard's Enter opens it here.
		if table.Submitted() && !opened && !a.editorOpen {
			a.openRemoteSelected(b)
		}
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
	b.syncRemotePathEdit(c)
	ui.Row(c).Height(30).Shrink(0).Padding(0, 9).Gap(2).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Background(colPanelTop).Children(func() {
		if b.remotePathEdit {
			a.remotePathField(c, b)
		} else {
			// The crumbs scroll sideways when a deep path outgrows the bar,
			// so the actions on the right keep their room. A click on the
			// empty part of the bar edits the whole path, as Explorer's
			// address bar does.
			ui.ScrollHorizontal(c).Grow(1).Height(30).
				OnClick(b.beginRemotePathEdit).Children(func() {
				ui.Row(c).Height(30).Gap(2).AlignItems(ui.Center).Children(func() {
					a.pathCrumb(c, t, b, "/", "/", b.remoteDir == "/")
					if b.remoteDir != "/" {
						acc := "/"
						segs := strings.Split(strings.Trim(b.remoteDir, "/"), "/")
						for i, seg := range segs {
							if seg == "" {
								continue
							}
							acc = path.Join(acc, seg)
							ui.Text(c, "›").FontSize(10).TextColor(colMuted2)
							a.pathCrumb(c, t, b, shortLabel(seg, 18), acc, i == len(segs)-1)
						}
					}
				})
			})
		}

		up := paneIconButton(c, t, "bf-act-up", iconFolderUp, "返回上一级")
		up.Disabled(b.remoteDir == "/")
		if up.Clicked() {
			a.navigateRemote(b, path.Dir(b.remoteDir))
		}

		ui.Text(c, fmt.Sprintf("%d 项", len(b.remoteEntries))).FontSize(9.2).
			Font("Cascadia Code").TextColor(colMuted2).NoWrap()

		ui.Box(c).Width(1).Height(15).Background(colLine).MarginX(5)

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

func (a *App) remotePathField(c *ui.Context, b *fileBrowser) {
	input := ui.TextInput(c, &b.remotePathInput).Grow(1).Height(22).
		Font("Cascadia Code").FontSize(9.8).Placeholder("输入远程路径，回车跳转").
		SelectionColor(colCyan.Alpha(0.65)).
		Bind(&b.remotePathFocus)
	input.OnTextKey(func(k ui.TextKey) bool {
		if k.Key != ui.KeyEscape || k.Mods != 0 {
			return false
		}
		b.endRemotePathEdit()
		return true
	})
	if b.remotePathPick {
		// Entering the editor selects the whole path, as Explorer does, so
		// typing replaces it and copying takes the full address.
		input.SetTextSelection(0, len([]rune(b.remotePathInput)))
		b.remotePathFocus.Focus(c)
		b.remotePathPick = false
		b.remotePathWait = true
	}
	if input.Submitted() {
		a.navigateRemote(b, b.remotePathInput)
		if b.remoteErr == "" {
			b.endRemotePathEdit()
		}
	}
}

// syncRemotePathEdit leaves the path editor once its focus has moved away,
// after waiting for the focus requested when it opened.
func (b *fileBrowser) syncRemotePathEdit(c *ui.Context) {
	if !b.remotePathEdit || b.remotePathPick {
		return
	}
	if b.remotePathWait {
		if b.remotePathFocus.Focused(c) {
			b.remotePathWait = false
		}
		return
	}
	if !b.remotePathFocus.Focused(c) {
		b.endRemotePathEdit()
	}
}

func (b *fileBrowser) beginRemotePathEdit() {
	b.remotePathEdit = true
	b.remotePathInput = b.remoteDir
	b.remotePathPick = true
	b.remotePathWait = false
}

func (b *fileBrowser) endRemotePathEdit() {
	b.remotePathEdit = false
	b.remotePathInput = b.remoteDir
	b.remotePathPick = false
	b.remotePathWait = false
}

func (a *App) pathCrumb(c *ui.Context, t *ui.Theme, b *fileBrowser, label, target string, active bool) {
	btn := ui.ButtonBase(c.Key("bf-crumb-"+target)).Height(22).Padding(0, 4).Radius(3).
		Center().Tooltip(target)
	if active {
		btn.Background(colCyan.Alpha(0.08))
	} else if btn.Hovered() {
		btn.Background(t.SurfaceHover)
	}
	btn.Children(func() {
		ui.Text(c, label).FontSize(10.2).Font("Cascadia Code").
			TextColor(condColor(active, colCyan, colMuted)).NoWrap()
	})
	if btn.Clicked() {
		if active {
			b.beginRemotePathEdit()
		} else {
			a.navigateRemote(b, target)
		}
	}
}

func (a *App) bottomFileTree(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	ui.Column(c).FillWidth().FillHeight().Background(colTreeBg).Children(func() {
		ui.Column(c).Grow(1).Padding(7).Children(func() {
			ui.Scroll(c).Grow(1).Gap(2).Children(func() {
				a.treeDirRow(c, t, b, "/", 0)
			})
		})
	})
}

func (a *App) treeDirRow(c *ui.Context, t *ui.Theme, b *fileBrowser, dir string, depth int) {
	a.dirTreeRow(c, t, b, dir, depth, false)
}

// dirTreeRow draws one folder of the dock's tree, for the remote filesystem
// or, with local set, for this machine's disk.
func (a *App) dirTreeRow(c *ui.Context, t *ui.Theme, b *fileBrowser, dir string, depth int, local bool) {
	kids := b.treeKids[dir]
	loaded := b.treeLoaded[dir]
	open := b.treeOpen[dir]

	prefix := "bf"
	current := b.remoteDir
	label := path.Base(dir)
	if dir == "/" {
		label = "/"
	}
	if local {
		prefix = "lf"
		current = b.localDir
		label = localTreeLabel(dir)
	}
	active := current == dir
	if local {
		active = strings.EqualFold(current, dir)
	}
	hasKids := !loaded || len(kids) > 0

	ui.Row(c).FillWidth().Height(29).Gap(1).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(float32(depth) * 15).Shrink(0)

		if hasKids {
			twisty := ui.ButtonBase(c.Key(prefix+"-tw-"+dir)).Size(19, 19).Center().Radius(4).
				Tooltip(condLabel(open, "收起目录", "展开目录"))
			if twisty.Hovered() {
				twisty.Background(t.SurfaceHover)
			}
			twisty.Children(func() {
				icon := iconChevronRight
				if open {
					icon = iconChevron
				}
				iconEl(c, icon, 11, colMuted2)
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
			ui.Box(c).Width(19).Shrink(0)
		}

		row := ui.ButtonBase(c.Key(prefix+"-tree-"+dir)).Grow(1).Height(26).
			Padding(0, 4).Radius(4).Gap(8).AlignItems(ui.Center).Justify(ui.Start)
		if active {
			row.Background(colCyan.Alpha(0.08))
		} else if row.Hovered() {
			row.Background(t.SurfaceHover)
		}
		row.Children(func() {
			iconEl(c, iconFolder, 17, condColor(active, colCyan, colMuted))
			ui.Text(c, shortLabel(label, 22)).FontSize(13).Font("Cascadia Code").
				TextColor(condColor(active, colText, colText2)).NoWrap()
		})
		if row.Clicked() {
			if local {
				a.navigateLocal(b, dir)
			} else {
				a.navigateRemote(b, dir)
			}
		}
	})

	if open {
		if msg := b.treeErr[dir]; msg != "" {
			ui.Text(c, shortLabel(msg, 34)).FontSize(9.6).TextColor(colRed).
				Padding(1, 8, 2, float32(depth)*15+28).NoWrap()
		}
		for _, kid := range kids {
			a.dirTreeRow(c, t, b, kid, depth+1, local)
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

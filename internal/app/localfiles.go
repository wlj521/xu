package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// localPathBar is the local dock's breadcrumb bar: the same shape as the
// remote one (crumbs, path editor, up, count, actions), so both panes read
// the same at a glance.
func (a *App) localPathBar(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	b.syncLocalPathEdit(c)
	ui.Row(c).Height(30).Shrink(0).Padding(0, 9).Gap(2).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Background(colPanelTop).Children(func() {
		if b.localPathEdit {
			a.localPathField(c, b)
		} else {
			// A click on the empty part of the bar edits the whole path, as
			// Explorer's address bar does; the crumbs scroll when deep.
			ui.ScrollHorizontal(c).Grow(1).Height(30).
				OnClick(b.beginLocalPathEdit).Children(func() {
				ui.Row(c).Height(30).Gap(2).AlignItems(ui.Center).Children(func() {
					root, segs := localPathParts(b.localDir)
					a.localPathCrumb(c, t, b, root, root, len(segs) == 0)
					acc := root
					for i, seg := range segs {
						acc = filepath.Join(acc, seg)
						ui.Text(c, "›").FontSize(10).TextColor(colMuted2)
						a.localPathCrumb(c, t, b, shortLabel(seg, 18), acc, i == len(segs)-1)
					}
				})
			})
		}

		parent := filepath.Dir(b.localDir)
		up := paneIconButton(c, t, "lf-act-up", iconFolderUp, "返回上一级")
		up.Disabled(parent == b.localDir)
		if up.Clicked() {
			a.navigateLocal(b, parent)
		}

		ui.Text(c, fmt.Sprintf("%d 项", len(b.localEntries))).FontSize(9.2).
			Font("Cascadia Code").TextColor(colMuted2).NoWrap()

		ui.Box(c).Width(1).Height(15).Background(colLine).MarginX(5)

		if paneIconButton(c, t, "lf-act-refresh", iconRefresh, "刷新").Clicked() {
			b.loadLocal()
		}
		if paneIconButton(c, t, "lf-act-new", iconNewFolder, "新建文件夹").Clicked() {
			a.promptMkdir(b, false)
		}
		if paneIconButton(c, t, "lf-act-upload", iconUpload, "上传文件").Clicked() {
			a.uploadSelected(b)
		}
		if paneIconButton(c, t, "lf-act-download", iconDownload, "下载所选").Clicked() {
			a.downloadSelected(b)
		}
		more := paneIconButton(c, t, "lf-act-more", iconMore, "更多")
		more.Menu(func(m *ui.Menu) {
			if m.Item("新建文件").Chosen() {
				a.promptNewFile(b, false)
			}
			if m.Item("新建文件夹").Chosen() {
				a.promptMkdir(b, false)
			}
			m.Separator()
			if m.Item("重命名").Chosen() {
				a.promptRename(b, false)
			}
			if m.Item("删除").Chosen() {
				a.deleteSelected(b, false)
			}
			m.Separator()
			if m.Item("刷新").Chosen() {
				b.loadLocal()
			}
		})
	})
}

func (a *App) localPathCrumb(c *ui.Context, t *ui.Theme, b *fileBrowser, label, target string, active bool) {
	btn := ui.ButtonBase(c.Key("lf-crumb-"+target)).Height(22).Padding(0, 4).Radius(3).
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
			b.beginLocalPathEdit()
		} else {
			a.navigateLocal(b, target)
		}
	}
}

func (a *App) localPathField(c *ui.Context, b *fileBrowser) {
	input := ui.TextInput(c, &b.localPathInput).Grow(1).Height(22).
		Font("Cascadia Code").FontSize(9.8).Placeholder("输入本地路径，回车跳转").
		SelectionColor(colCyan.Alpha(0.65)).
		Bind(&b.localPathFocus)
	input.OnTextKey(func(k ui.TextKey) bool {
		if k.Key != ui.KeyEscape || k.Mods != 0 {
			return false
		}
		b.endLocalPathEdit()
		return true
	})
	if b.localPathPick {
		// Entering the editor selects the whole path, so typing replaces it
		// and copying takes the full address.
		input.SetTextSelection(0, len([]rune(b.localPathInput)))
		b.localPathFocus.Focus(c)
		b.localPathPick = false
		b.localPathWait = true
	}
	if input.Submitted() {
		a.navigateLocal(b, b.localPathInput)
		if b.localErr == "" {
			b.endLocalPathEdit()
		}
	}
}

// syncLocalPathEdit leaves the path editor once its focus has moved away,
// after waiting for the focus requested when it opened.
func (b *fileBrowser) syncLocalPathEdit(c *ui.Context) {
	if !b.localPathEdit || b.localPathPick {
		return
	}
	if b.localPathWait {
		if b.localPathFocus.Focused(c) {
			b.localPathWait = false
		}
		return
	}
	if !b.localPathFocus.Focused(c) {
		b.endLocalPathEdit()
	}
}

func (b *fileBrowser) beginLocalPathEdit() {
	b.localPathEdit = true
	b.localPathInput = b.localDir
	b.localPathPick = true
	b.localPathWait = false
}

func (b *fileBrowser) endLocalPathEdit() {
	b.localPathEdit = false
	b.localPathInput = b.localDir
	b.localPathPick = false
	b.localPathWait = false
}

// localFileTree mirrors the remote tree: the volume root and the folders
// open down the current path.
func (a *App) localFileTree(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	root, _ := localPathParts(b.localDir)
	ui.Column(c).FillWidth().FillHeight().Background(colTreeBg).Children(func() {
		ui.Column(c).Grow(1).Padding(7).Children(func() {
			ui.Scroll(c).Grow(1).Gap(2).Children(func() {
				a.dirTreeRow(c, t, b, root, 0, true)
			})
		})
	})
}

// localFileTable is the local counterpart of bottomFileTable: the same
// columns, rows and click behaviour as the remote file list.
func (a *App) localFileTable(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	ui.Column(c).FillWidth().FillHeight().Children(func() {
		wide := a.dockFileTableWide(c)
		cols := bottomFileColumns()
		if wide {
			cols = fileColumns()
		}
		opened := false
		tableTheme := fileTableTheme(t)
		tableTheme.FontSize = 11.5
		var table ui.Element
		b.localState.CellMenu = func(m *ui.Menu, row, col int) {
			if row >= 0 && row < len(b.localEntries) {
				b.localSel = row
			}
			a.fileContextMenu(m, b, false)
		}
		ui.Themed(c, tableTheme).Grow(1).FillWidth().Background(ui.Transparent).Children(func() {
			table = ui.Table(c, &b.localState, cols, len(b.localEntries), func(row, col int) {
				if row < 0 || row >= len(b.localEntries) {
					return
				}
				f := b.localEntries[row]
				cell := ui.Box(c).Fill().OnClick(func() {
					a.clickLocalRow(b, row)
				})
				cell.ContextMenu(func(m *ui.Menu) {
					b.localSel = row
					a.fileContextMenu(m, b, false)
				})
				if !f.IsDir() && cell.DoubleClicked() {
					b.localSel = row
					a.openLocalSelected(b)
					opened = true
				}
				cell.Children(func() {
					if wide {
						localCell(c, b, row, col)
					} else {
						bottomLocalCell(c, b, row, col)
					}
				})
			})
			table.Grow(1)
		})
		table.ContextMenu(func(m *ui.Menu) { a.fileBackgroundMenu(m, b, false) })
		if table.Submitted() && !opened && !a.editorOpen {
			a.openLocalSelected(b)
		}
	})
}

func localCell(c *ui.Context, b *fileBrowser, row, col int) {
	if row < 0 || row >= len(b.localEntries) {
		return
	}
	// Windows has no POSIX owner pairs; the mode bits still tell read-only
	// from writable, which is what the 权限 column shows.
	fileCell(c, b.localEntries[row], "—", col)
}

func bottomLocalCell(c *ui.Context, b *fileBrowser, row, col int) {
	if row < 0 || row >= len(b.localEntries) {
		return
	}
	f := b.localEntries[row]
	switch col {
	case 0, 1:
		fileCell(c, f, "—", col)
	case 2:
		ui.Text(c, f.ModTime().Format("2006/01/02 15:04")).FontSize(10.2).
			Font("Cascadia Code").TextColor(colMuted).NoWrap()
	}
}

// clickLocalRow opens a folder on a single click, like the remote table.
func (a *App) clickLocalRow(b *fileBrowser, row int) {
	if row < 0 || row >= len(b.localEntries) {
		return
	}
	b.localSel = row
	if f := b.localEntries[row]; f.IsDir() {
		a.navigateLocal(b, filepath.Join(b.localDir, f.Name()))
	}
}

// localTreeLabel names a tree row: the volume root keeps its "C:\" style,
// everything else shows its folder name.
func localTreeLabel(dir string) string {
	root, _ := localPathParts(dir)
	if strings.EqualFold(filepath.Clean(dir), filepath.Clean(root)) {
		return root
	}
	return filepath.Base(dir)
}

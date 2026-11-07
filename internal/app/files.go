package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/pkg/sftp"
)

type fileBrowser struct {
	termID     int64
	client     *sftp.Client
	remoteHome string

	localDir       string
	localPathInput string
	localEntries   []os.FileInfo
	localSel       int
	localErr       string
	localState     ui.ListState

	remoteDir       string
	remotePathInput string
	remoteEntries   []os.FileInfo
	remoteSel       int
	remoteErr       string
	remoteState     ui.ListState

	// Remote tree used by the bottom dock. Children are loaded lazily and
	// cached per directory; treeOpen remembers which branches are expanded.
	treeOpen   map[string]bool
	treeKids   map[string][]string
	treeLoaded map[string]bool
	treeErr    map[string]string
}

type transferState struct {
	id     int64
	label  string
	done   int64
	total  int64
	cancel context.CancelFunc
}

type transferRecord struct {
	Label    string
	Done     int64
	Total    int64
	Err      string
	Canceled bool
}

func newFileBrowser(termID int64, client *sftp.Client, localDir, remoteHome string) *fileBrowser {
	b := &fileBrowser{
		termID:     termID,
		client:     client,
		remoteHome: remoteHome,
		localSel:   -1,
		remoteSel:  -1,
		treeOpen:   map[string]bool{"/": true},
		treeKids:   map[string][]string{},
		treeLoaded: map[string]bool{},
		treeErr:    map[string]string{},
	}

	b.localState.Selected = &b.localSel
	b.remoteState.Selected = &b.remoteSel
	b.localState.Key = func(i int) any { return "l:" + entryAt(b.localEntries, i) }
	b.remoteState.Key = func(i int) any { return "r:" + entryAt(b.remoteEntries, i) }
	b.localState.Label = func(i int) string { return entryAt(b.localEntries, i) }
	b.remoteState.Label = func(i int) string { return entryAt(b.remoteEntries, i) }

	if err := b.setLocalDir(localDir); err != nil {
		b.localErr = err.Error()
	}
	if client != nil {
		b.remoteDir = remoteHome
		b.remotePathInput = remoteHome
		b.loadRemote()
		b.loadTreeDir("/")
		b.revealTreePath(remoteHome)
	}
	return b
}

// ensureLocalBrowser returns a local-only browser for local terminals. It has
// no SFTP client, so only localPane may render it.
func (a *App) ensureLocalBrowser(ts *termSession) *fileBrowser {
	if b := a.browsers[ts.ID]; b != nil {
		return b
	}
	localDir, err := os.UserHomeDir()
	if err != nil || localDir == "" {
		localDir = "."
	}
	b := newFileBrowser(ts.ID, nil, localDir, "")
	a.browsers[ts.ID] = b
	return b
}

func (b *fileBrowser) Close() {
	if b.client != nil {
		b.client.Close()
	}
}

func (a *App) activeTerminal() *termSession {
	if a.activeTerm < 0 || a.activeTerm >= len(a.terms) {
		return nil
	}
	return a.terms[a.activeTerm]
}

func (a *App) openFiles() {
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	if a.finding {
		a.closeFind()
	}
	if a.activeTerminal() == nil {
		a.OpenLocalTerminal()
	}
	if a.activeTerminal() == nil {
		a.status = "请先打开一个终端"
		return
	}
	a.setDockTab(dockFiles)
	a.fileErr = ""
	a.status = ""
}

// ensureFileBrowser opens the SFTP subsystem for ts on first use.
func (a *App) ensureFileBrowser(ts *termSession) (*fileBrowser, error) {
	if b := a.browsers[ts.ID]; b != nil {
		return b, nil
	}
	if ts.client == nil {
		return nil, fmt.Errorf("SSH 连接不可用")
	}

	sc, err := sftp.NewClient(ts.client)
	if err != nil {
		return nil, err
	}

	localDir, err := os.UserHomeDir()
	if err != nil || localDir == "" {
		localDir = "."
	}
	remoteHome, err := sc.Getwd()
	if err != nil || remoteHome == "" {
		remoteHome = "/"
	}

	b := newFileBrowser(ts.ID, sc, localDir, remoteHome)
	a.browsers[ts.ID] = b
	return b, nil
}

func (a *App) filesView(c *ui.Context, t *ui.Theme) {
	ts := a.activeTerminal()
	if ts == nil {
		a.filesEmptyPage(c, t, "请先打开一个会话", "连接一台服务器后即可管理远端文件")
		return
	}
	if ts.local {
		a.filesEmptyPage(c, t, "本地终端不支持 SFTP", "切换到 SSH 会话后再打开文件管理")
		return
	}
	b := a.browsers[ts.ID]
	if b == nil {
		if _, err := a.ensureFileBrowser(ts); err != nil {
			a.filesEmptyPage(c, t, "打开 SFTP 失败", err.Error())
			return
		}
		b = a.browsers[ts.ID]
	}
	if b == nil {
		a.filesEmptyPage(c, t, "文件浏览器不可用", "请先连接一台 SSH 服务器")
		return
	}

	ui.Column(c).Grow(1).Fill().Padding(20, 24, 16, 24).Gap(12).Children(func() {
		a.filesPageHeader(c, t, b, ts)

		if a.promptOpen {
			a.promptBar(c)
		}
		if a.fileErr != "" {
			ui.Box(c).FillWidth().Padding(8, 11).Radius(5).
				Background(colRed.Alpha(0.07)).Border(1, colRed.Alpha(0.18)).Children(func() {
				ui.Text(c, a.fileErr).FontSize(10).TextColor(colRed)
			})
		}

		ui.Row(c).Grow(1).Gap(12).AlignItems(ui.Stretch).Children(func() {
			a.localPane(c, b, t)
			a.remotePane(c, b, t)
		})

		if a.transfer != nil {
			a.transferTray(c, t)
		}
	})
}

func (a *App) filesPageHeader(c *ui.Context, t *ui.Theme, b *fileBrowser, ts *termSession) {
	pageHeader(c, t, iconFolder, "文件管理",
		fmt.Sprintf("SFTP · %s · %s", ts.Title, shortLabel(b.remoteDir, 46)),
		func() {
			if ghostAction(c, t, "files-terminal", "终端", iconTerm).Clicked() {
				a.goNav(navWorkspace)
			}
			if ghostAction(c, t, "files-refresh", "刷新", iconRefresh).Clicked() {
				b.loadLocal()
				b.loadRemote()
				a.fileErr = ""
			}
			if ghostAction(c, t, "files-download", "下载", iconDownload).Clicked() {
				a.downloadSelected(b)
			}
			if primaryAction(c, "files-upload", "上传", iconUpload).Clicked() {
				a.uploadSelected(b)
			}
		})
}

func (a *App) filesEmptyPage(c *ui.Context, t *ui.Theme, title, sub string) {
	ui.Column(c).Grow(1).Fill().Padding(24, 28).Gap(16).Children(func() {
		pageHeader(c, t, iconFolder, "文件管理", "本地与远端 SFTP 文件", nil)
		ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
			ui.Box(c).Size(58, 58).Radius(10).
				Background(colPanel2).Border(1, colLine).Center().Children(func() {
				iconEl(c, iconFolder, 24, colMuted2)
			})
			ui.Text(c, title).FontSize(fsTitle).FontWeight(600).TextColor(colText)
			ui.Text(c, sub).FontSize(10.5).TextColor(colMuted).TextAlign(ui.Center)
			ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
				if primaryAction(c, "files-empty-connect", "打开连接", iconConnections).Clicked() {
					a.openConnectionManager()
				}
				if ghostAction(c, t, "files-empty-terminal", "返回终端", iconTerm).Clicked() {
					a.goNav(navWorkspace)
				}
			})
		})
	})
}

func (a *App) promptBar(c *ui.Context) {
	ui.Row(c).Padding(4, 12).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.promptTitle)
		ui.TextInput(c, &a.promptValue).Grow(1)
		if ui.PrimaryButton(c, "确定").Clicked() {
			a.submitPrompt()
		}
		if ui.Button(c, "取消").Clicked() {
			a.promptOpen = false
			a.promptAction = nil
		}
	})
}

func (a *App) transferBar(c *ui.Context, t *ui.Theme) {
	tr := a.transfer
	if tr == nil {
		return
	}
	pct := 0
	if tr.total > 0 {
		pct = int(tr.done * 100 / tr.total)
	}
	ui.Row(c).Padding(4, 12).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf("%s  %d%%  %s / %s", tr.label, pct, humanSize(tr.done), humanSize(tr.total))).TextColor(t.TextMuted)
		if ui.Button(c, "取消").Clicked() && tr.cancel != nil {
			tr.cancel()
		}
	})
}

func (a *App) localPane(c *ui.Context, b *fileBrowser, t *ui.Theme) {
	ui.Column(c).Grow(1).FillHeight().Background(colPanel2).
		Border(1, colLine).Radius(10).Children(func() {
		a.filePaneHead(c, t, "本地", "本机", iconTerm, colViolet, func() {
			if paneIconButton(c, t, "local-refresh", iconRefresh, "刷新").Clicked() {
				b.loadLocal()
			}
			if paneIconButton(c, t, "local-up", iconArrowUp, "上一级").Clicked() {
				a.navigateLocal(b, filepath.Dir(b.localDir))
			}
			if paneIconButton(c, t, "local-upload", iconUpload, "上传所选到远端").Clicked() {
				a.uploadSelected(b)
			}
			if paneIconButton(c, t, "local-download", iconDownload, "下载所选到本机").Clicked() {
				a.downloadSelected(b)
			}
			more := paneIconButton(c, t, "local-more", iconMore, "更多")
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
			})
		})

		a.localPathBar(c, t, b)

		if b.localErr != "" {
			ui.Text(c, shortLabel(b.localErr, 90)).FontSize(9).TextColor(colRed).Padding(2, 10)
		}

		tableTheme := *t
		tableTheme.FontSize = 9
		tableTheme.TextMuted = colMuted2
		tableTheme.Accent = colAccentSoft
		tableTheme.AccentText = colText
		tableTheme.Focus = colAccentLine
		b.localState.CellMenu = func(m *ui.Menu, row, col int) {
			if row >= 0 && row < len(b.localEntries) {
				b.localSel = row
			}
			a.fileContextMenu(m, b, false)
		}
		ui.Themed(c, &tableTheme).Grow(1).Background(ui.Transparent).
			ContextMenu(func(m *ui.Menu) { a.fileBackgroundMenu(m, b, false) }).Children(func() {
			if ui.Table(c, &b.localState, bottomFileColumns(), len(b.localEntries), func(row, col int) {
				if row >= 0 && row < len(b.localEntries) {
					compactFileCell(c, b.localEntries[row], col)
				}
			}).Grow(1).Submitted() {
				a.openLocalSelected(b)
			}
		})

		a.filePaneFoot(c, fmt.Sprintf("%d 项", len(b.localEntries)), "本机")
	})
}

func (a *App) remotePane(c *ui.Context, b *fileBrowser, t *ui.Theme) {
	zone := ui.Column(c).Grow(1).FillHeight().Background(colPanel2).
		Border(1, colLine).Radius(10)
	if zone.FileDragOver() {
		zone.Border(2, colCyan)
	}
	if files := zone.DroppedFiles(); files != nil {
		a.uploadDropped(b, files)
	}

	zone.Children(func() {
		a.filePaneHead(c, t, "远端", "SFTP", iconServer, colCyan, func() {
			if paneIconButton(c, t, "remote-refresh", iconRefresh, "刷新").Clicked() {
				b.loadRemote()
			}
			if paneIconButton(c, t, "remote-up", iconArrowUp, "上一级").Clicked() {
				a.navigateRemote(b, path.Dir(b.remoteDir))
			}
			if paneIconButton(c, t, "remote-upload", iconUpload, "上传文件").Clicked() {
				a.uploadViaDialog(b)
			}
			if paneIconButton(c, t, "remote-download", iconDownload, "下载所选").Clicked() {
				a.downloadSelected(b)
			}
			more := paneIconButton(c, t, "remote-more", iconMore, "更多")
			more.Menu(func(m *ui.Menu) {
				if m.Item("新建文件").Chosen() {
					a.promptNewFile(b, true)
				}
				if m.Item("新建文件夹").Chosen() {
					a.promptMkdir(b, true)
				}
				if m.Item("编辑所选").Chosen() {
					a.openSelectedRemoteForEdit(b)
				}
				m.Separator()
				if m.Item("重命名").Chosen() {
					a.promptRename(b, true)
				}
				if m.Item("删除").Chosen() {
					a.deleteSelected(b, true)
				}
			})
		})

		a.remotePathBar(c, t, b)

		if b.remoteErr != "" {
			ui.Text(c, shortLabel(b.remoteErr, 90)).FontSize(9).TextColor(colRed).Padding(2, 10)
		}

		tableTheme := *t
		tableTheme.FontSize = 9
		tableTheme.TextMuted = colMuted2
		tableTheme.Accent = colAccentSoft
		tableTheme.AccentText = colText
		tableTheme.Focus = colAccentLine
		b.remoteState.CellMenu = func(m *ui.Menu, row, col int) {
			if row >= 0 && row < len(b.remoteEntries) {
				b.remoteSel = row
			}
			a.fileContextMenu(m, b, true)
		}
		ui.Themed(c, &tableTheme).Grow(1).Background(ui.Transparent).
			ContextMenu(func(m *ui.Menu) { a.fileBackgroundMenu(m, b, true) }).Children(func() {
			if ui.Table(c, &b.remoteState, bottomFileColumns(), len(b.remoteEntries), func(row, col int) {
				if row >= 0 && row < len(b.remoteEntries) {
					compactFileCell(c, b.remoteEntries[row], col)
				}
			}).Grow(1).Submitted() {
				a.openRemoteSelected(b)
			}
		})

		a.filePaneFoot(c, fmt.Sprintf("%d 项", len(b.remoteEntries)), "SFTP")
	})
}

func paneIconButton(c *ui.Context, t *ui.Theme, key string, icon *ui.SVG, tip string) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(23, 23).Radius(5).Center().Tooltip(tip).Label(tip)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		iconEl(c, icon, 12, colMuted)
	})
	return b
}

func (a *App) fileContextMenu(m *ui.Menu, b *fileBrowser, remote bool) {
	f := selectedEntry(b.localEntries, b.localSel)
	if remote {
		f = selectedEntry(b.remoteEntries, b.remoteSel)
	}
	if f != nil {
		label := "打开"
		if remote && !f.IsDir() {
			label = "编辑"
		}
		if m.Item(label).Chosen() {
			if remote {
				a.openRemoteSelected(b)
			} else {
				a.openLocalSelected(b)
			}
		}
		if remote {
			if m.Item("下载到本机").Chosen() {
				a.downloadSelected(b)
			}
		} else {
			if m.Item("上传到远端").Chosen() {
				a.uploadSelected(b)
			}
			if m.Item("下载远端所选").Chosen() {
				a.downloadSelected(b)
			}
		}
		m.Separator()
	}
	if m.Item("新建文件").Chosen() {
		a.promptNewFile(b, remote)
	}
	if m.Item("新建文件夹").Chosen() {
		a.promptMkdir(b, remote)
	}
	if f != nil {
		m.Separator()
		if m.Item("重命名").Chosen() {
			a.promptRename(b, remote)
		}
		if m.Item("删除").Chosen() {
			a.deleteSelected(b, remote)
		}
	}
	m.Separator()
	if m.Item("刷新").Chosen() {
		if remote {
			b.loadRemote()
		} else {
			b.loadLocal()
		}
	}
}

func (a *App) fileBackgroundMenu(m *ui.Menu, b *fileBrowser, remote bool) {
	if m.Item("新建文件").Chosen() {
		a.promptNewFile(b, remote)
	}
	if m.Item("新建文件夹").Chosen() {
		a.promptMkdir(b, remote)
	}
	m.Separator()
	if remote && m.Item("上传文件…").Chosen() {
		a.uploadViaDialog(b)
	}
	if m.Item("刷新").Chosen() {
		if remote {
			b.loadRemote()
		} else {
			b.loadLocal()
		}
	}
}

func (a *App) filePaneHead(c *ui.Context, t *ui.Theme, title, badge string, icon *ui.SVG, color ui.Color, actions func()) {
	ui.Row(c).Height(33).Shrink(0).Padding(0, 10).Gap(6).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		iconEl(c, icon, 13, color)
		ui.Text(c, title).FontSize(11).FontWeight(600).TextColor(colText).NoWrap()
		ui.Box(c).Height(17).Padding(0, 5).Radius(4).Background(colPanelTop).
			Border(1, colLine).Center().Children(func() {
			ui.Text(c, shortLabel(badge, 16)).FontSize(8).Font("Cascadia Code").
				TextColor(colMuted).NoWrap()
		})
		ui.Text(c, "").Grow(1)
		actions()
	})
}

func (a *App) localPathBar(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	ui.Row(c).Height(27).Shrink(0).Padding(0, 8).Gap(3).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		up := ui.ButtonBase(c.Key("fp-local-up")).Size(21, 21).Center().Radius(4).
			Tooltip("上级目录")
		if up.Hovered() {
			up.Background(t.SurfaceHover)
		}
		up.Children(func() { iconEl(c, iconArrowUp, 11, colMuted) })
		if up.Clicked() {
			a.navigateLocal(b, filepath.Dir(b.localDir))
		}

		input := ui.TextInput(c, &b.localPathInput).Grow(1).Height(21).
			Font("Cascadia Code").FontSize(9.8).Placeholder("输入本地路径，回车跳转").
			Background(ui.Transparent).Border(0, ui.Transparent)
		if input.Submitted() {
			a.navigateLocal(b, b.localPathInput)
		}
		ui.Text(c, fmt.Sprintf("%d 项", len(b.localEntries))).FontSize(8.5).
			Font("Cascadia Code").TextColor(colMuted2).NoWrap()
	})
}

func (a *App) remotePathBar(c *ui.Context, t *ui.Theme, b *fileBrowser) {
	ui.Row(c).Height(27).Shrink(0).Padding(0, 8).Gap(3).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		up := ui.ButtonBase(c.Key("fp-remote-up")).Size(21, 21).Center().Radius(4).
			Tooltip("上级目录").Disabled(b.remoteDir == "/")
		if up.Hovered() && b.remoteDir != "/" {
			up.Background(t.SurfaceHover)
		}
		up.Children(func() { iconEl(c, iconArrowUp, 11, condColor(b.remoteDir == "/", colMuted2, colMuted)) })
		if up.Clicked() && b.remoteDir != "/" {
			a.navigateRemote(b, path.Dir(b.remoteDir))
		}

		input := ui.TextInput(c, &b.remotePathInput).Grow(1).Height(21).
			Font("Cascadia Code").FontSize(9.8).Placeholder("输入远端路径，回车跳转").
			Background(ui.Transparent).Border(0, ui.Transparent)
		if input.Submitted() {
			a.navigateRemote(b, b.remotePathInput)
		}
		ui.Text(c, fmt.Sprintf("%d 项", len(b.remoteEntries))).FontSize(8.5).
			Font("Cascadia Code").TextColor(colMuted2).NoWrap()
	})
}

func (a *App) fileCrumb(c *ui.Context, t *ui.Theme, b *fileBrowser, label, target string, active bool) {
	btn := ui.ButtonBase(c.Key("fp-crumb-"+target)).Height(20).Padding(0, 4).Radius(3).
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

func (a *App) filePaneFoot(c *ui.Context, left, right string) {
	ui.Row(c).Height(22).Shrink(0).Padding(0, 9).Gap(5).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		ui.Text(c, left).FontSize(8.3).Font("Cascadia Code").TextColor(colMuted2)
		ui.Text(c, "").Grow(1)
		ui.Text(c, "UTF-8").FontSize(8.3).Font("Cascadia Code").TextColor(colMuted2)
		ui.Text(c, right).FontSize(8.3).Font("Cascadia Code").TextColor(colMuted2)
	})
}

func compactFileCell(c *ui.Context, f os.FileInfo, col int) {
	switch col {
	case 0:
		fileCell(c, f, "—", 0)
	case 1:
		fileCell(c, f, "—", 1)
	case 2:
		ui.Text(c, f.ModTime().Format("01/02 15:04")).FontSize(9.1).
			Font("Cascadia Code").TextColor(colMuted).TextAlign(ui.End).NoWrap()
	}
}

func (a *App) transferTray(c *ui.Context, t *ui.Theme) {
	tr := a.transfer
	if tr == nil {
		return
	}
	pct := 0
	if tr.total > 0 {
		pct = int(tr.done * 100 / tr.total)
	}
	icon := iconDownload
	if strings.Contains(tr.label, "上传") {
		icon = iconUpload
	}
	ui.Column(c).FillWidth().Shrink(0).Padding(9, 12).Gap(7).
		Background(colPanel).Border(1, colLine).Radius(6).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			iconEl(c, icon, 13, colCyan)
			ui.Text(c, shortLabel(tr.label, 60)).FontSize(10.5).TextColor(colText).NoWrap()
			ui.Text(c, "").Grow(1)
			ui.Text(c, fmt.Sprintf("%d%% · %s / %s", pct, humanSize(tr.done), humanSize(tr.total))).
				FontSize(9).Font("Cascadia Code").TextColor(colMuted2).NoWrap()
			if ghostAction(c, t, "files-transfer-cancel", "取消", nil).Clicked() && tr.cancel != nil {
				tr.cancel()
			}
		})
		barFill(c, float64(pct)/100, colCyan, 4)
	})
}

func (a *App) uploadDropped(b *fileBrowser, paths []string) {
	if a.transfer != nil {
		a.fileErr = "已有传输在进行，请等待或取消"
		return
	}

	type item struct {
		local string
		size  int64
	}
	var items []item
	var total int64
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			a.fileErr = "读取拖入文件失败: " + err.Error()
			continue
		}
		if st.IsDir() {
			a.fileErr = "暂不支持拖拽目录上传"
			continue
		}
		items = append(items, item{local: p, size: st.Size()})
		total += st.Size()
	}
	if len(items) == 0 {
		return
	}

	a.startTransfer(fmt.Sprintf("上传 %d 个文件", len(items)), func(ctx context.Context, report func(int64, int64)) error {
		var done int64
		for _, it := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			remote := path.Join(b.remoteDir, filepath.Base(it.local))
			base := done
			if err := uploadFile(ctx, b.client, it.local, remote, func(d, _ int64) {
				report(base+d, total)
			}); err != nil {
				return err
			}
			done += it.size
			report(done, total)
		}
		return nil
	}, func() {
		b.loadRemote()
	})
}

func fileColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{Title: "文件名"},
		{Title: "大小", Width: 82, Align: ui.End},
		{Title: "类型", Width: 68},
		{Title: "修改时间", Width: 132},
		{Title: "权限", Width: 98},
		{Title: "用户/组", Width: 96},
	}
}

// bottomFileColumns is the compact set used by the dock: FinalShell-style
// name / size / modified layout, leaving the path and tree room to breathe.
func bottomFileColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{Title: "名称"},
		{Title: "大小", Width: 64, Align: ui.End},
		{Title: "修改时间", Width: 80, Align: ui.End},
	}
}

func bottomRemoteCell(c *ui.Context, b *fileBrowser, row, col int) {
	if row < 0 || row >= len(b.remoteEntries) {
		return
	}
	f := b.remoteEntries[row]
	switch col {
	case 0:
		fileCell(c, f, "—", 0)
	case 1:
		fileCell(c, f, "—", 1)
	case 2:
		ui.Text(c, f.ModTime().Format("2006/01/02 15:04")).FontSize(9.5).
			Font("Cascadia Code").TextColor(colMuted).NoWrap()
	}
}

func fileKind(f os.FileInfo) string {
	switch {
	case f.Mode()&os.ModeSymlink != 0:
		return "链接"
	case f.IsDir():
		return "文件夹"
	default:
		return "文件"
	}
}

func fileCell(c *ui.Context, f os.FileInfo, owner string, col int) {
	switch col {
	case 0:
		icon := iconFile
		color := colBlue
		if f.IsDir() {
			icon = iconFolder
			color = colAmber
		} else if f.Mode()&os.ModeSymlink != 0 {
			icon = iconLink
			color = colFileLink
		}
		name := f.Name()
		if f.IsDir() {
			name += "/"
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			iconEl(c, icon, 13, color)
			ui.Text(c, name).FontSize(11).Font("Cascadia Code").TextColor(colText2).NoWrap()
		})
	case 1:
		if !f.IsDir() {
			ui.Text(c, humanSize(f.Size())).FontSize(9.1).Font("Cascadia Code").
				TextColor(colMuted).TextAlign(ui.End).NoWrap()
		}
	case 2:
		ui.Text(c, fileKind(f)).FontSize(9.1).Font("Cascadia Code").TextColor(colMuted).NoWrap()
	case 3:
		ui.Text(c, f.ModTime().Format("2006/01/02 15:04")).FontSize(9.1).
			Font("Cascadia Code").TextColor(colMuted).NoWrap()
	case 4:
		ui.Text(c, f.Mode().Perm().String()).FontSize(9.1).Font("Cascadia Code").
			TextColor(colFilePerm).NoWrap()
	case 5:
		ui.Text(c, owner).FontSize(9.1).Font("Cascadia Code").TextColor(colMuted).NoWrap()
	}
}

func localCell(c *ui.Context, b *fileBrowser, row, col int) {
	if row < 0 || row >= len(b.localEntries) {
		return
	}
	fileCell(c, b.localEntries[row], "—", col)
}

func remoteCell(c *ui.Context, b *fileBrowser, row, col int) {
	if row < 0 || row >= len(b.remoteEntries) {
		return
	}
	f := b.remoteEntries[row]
	owner := "—"
	if st, ok := f.Sys().(*sftp.FileStat); ok {
		if st.UID == 0 && st.GID == 0 {
			owner = "root / root"
		} else {
			owner = fmt.Sprintf("%d / %d", st.UID, st.GID)
		}
	}
	fileCell(c, f, owner, col)
}

func (a *App) navigateLocal(b *fileBrowser, dir string) {
	if err := b.setLocalDir(dir); err != nil {
		b.localErr = err.Error()
		return
	}
	a.fileErr = ""
}

func (b *fileBrowser) setLocalDir(dir string) error {
	dir = filepath.Clean(expandLocal(dir))
	entries, err := readLocalDir(dir)
	if err != nil {
		return err
	}
	b.localDir = dir
	b.localPathInput = dir
	b.localEntries = entries
	b.localSel = -1
	b.localErr = ""
	return nil
}

func (a *App) navigateRemote(b *fileBrowser, dir string) {
	dir = b.expandRemote(dir)
	info, err := b.client.Stat(dir)
	if err != nil {
		b.remoteErr = err.Error()
		return
	}
	if !info.IsDir() {
		b.remoteErr = "不是目录: " + dir
		return
	}
	b.remoteDir = dir
	b.remotePathInput = dir
	b.remoteSel = -1
	b.remoteErr = ""
	b.loadRemote()
	b.revealTreePath(dir)
	a.fileErr = ""
}

func (b *fileBrowser) loadLocal() {
	if err := b.setLocalDir(b.localDir); err != nil {
		b.localErr = err.Error()
	}
}

func (b *fileBrowser) loadRemote() {
	entries, err := b.client.ReadDir(b.remoteDir)
	if err != nil {
		b.remoteErr = err.Error()
		return
	}
	sortEntries(entries)
	b.remoteEntries = entries
	b.remoteSel = -1
	b.remoteErr = ""

	// Keep the tree branch for the current directory in sync with the table
	// without issuing a second ReadDir.
	if b.treeKids != nil {
		kids := make([]string, 0, len(entries))
		for _, f := range entries {
			if !f.IsDir() {
				continue
			}
			name := f.Name()
			if name == "." || name == ".." || strings.HasPrefix(name, ".") {
				continue
			}
			kids = append(kids, path.Join(b.remoteDir, name))
		}
		sort.Strings(kids)
		b.treeKids[b.remoteDir] = kids
		b.treeLoaded[b.remoteDir] = true
		delete(b.treeErr, b.remoteDir)
	}
}

func (b *fileBrowser) loadTreeDir(dir string) {
	if b.treeKids == nil {
		b.treeOpen = map[string]bool{"/": true}
		b.treeKids = map[string][]string{}
		b.treeLoaded = map[string]bool{}
		b.treeErr = map[string]string{}
	}
	if b.treeLoaded[dir] {
		return
	}
	b.treeLoaded[dir] = true
	if b.client == nil {
		return
	}
	entries, err := b.client.ReadDir(dir)
	if err != nil {
		b.treeErr[dir] = err.Error()
		return
	}
	kids := make([]string, 0, len(entries))
	for _, f := range entries {
		if !f.IsDir() {
			continue
		}
		name := f.Name()
		if name == "." || name == ".." || strings.HasPrefix(name, ".") {
			continue
		}
		kids = append(kids, path.Join(dir, name))
	}
	sort.Strings(kids)
	b.treeKids[dir] = kids
	delete(b.treeErr, dir)
}

// revealTreePath expands the ancestors of dir so the active branch is visible
// in the remote tree.
func (b *fileBrowser) revealTreePath(dir string) {
	dir = path.Clean(dir)
	if !strings.HasPrefix(dir, "/") {
		return
	}
	b.loadTreeDir("/")
	b.treeOpen["/"] = true
	if dir == "/" {
		return
	}
	acc := "/"
	for _, seg := range strings.Split(strings.TrimPrefix(dir, "/"), "/") {
		if seg == "" {
			continue
		}
		acc = path.Join(acc, seg)
		b.treeOpen[acc] = true
		b.loadTreeDir(acc)
	}
}

func (a *App) openLocalSelected(b *fileBrowser) {
	f := selectedEntry(b.localEntries, b.localSel)
	if f == nil {
		return
	}
	p := filepath.Join(b.localDir, f.Name())
	if f.IsDir() {
		a.navigateLocal(b, p)
		return
	}
	_ = mygo.Shell.OpenPath(p)
}

func (a *App) openRemoteSelected(b *fileBrowser) {
	f := selectedEntry(b.remoteEntries, b.remoteSel)
	if f == nil {
		return
	}
	remote := path.Join(b.remoteDir, f.Name())
	if f.IsDir() {
		a.navigateRemote(b, remote)
		return
	}
	a.openRemoteEditor(b, remote)
}

func (a *App) uploadSelected(b *fileBrowser) {
	if b.client == nil {
		a.fileErr = "需要 SSH 连接才能上传"
		return
	}
	if a.transfer != nil {
		a.fileErr = "已有传输在进行，请等待或取消"
		return
	}
	f := selectedEntry(b.localEntries, b.localSel)
	if f == nil {
		a.fileErr = "请先在左侧选择文件"
		return
	}
	if f.IsDir() {
		a.fileErr = "暂不支持目录上传"
		return
	}

	local := filepath.Join(b.localDir, f.Name())
	remote := path.Join(b.remoteDir, f.Name())
	a.startTransfer("上传 "+f.Name(), func(ctx context.Context, report func(int64, int64)) error {
		return uploadFile(ctx, b.client, local, remote, report)
	}, func() {
		b.loadRemote()
	})
}

func (a *App) downloadSelected(b *fileBrowser) {
	if b.client == nil {
		a.fileErr = "需要 SSH 连接才能下载"
		return
	}
	if a.transfer != nil {
		a.fileErr = "已有传输在进行，请等待或取消"
		return
	}
	f := selectedEntry(b.remoteEntries, b.remoteSel)
	if f == nil {
		a.fileErr = "请先在右侧选择文件"
		return
	}
	if f.IsDir() {
		a.fileErr = "暂不支持目录下载"
		return
	}

	remote := path.Join(b.remoteDir, f.Name())
	local := filepath.Join(b.localDir, safeLocalName(f.Name()))
	a.startTransfer("下载 "+f.Name(), func(ctx context.Context, report func(int64, int64)) error {
		return downloadFile(ctx, b.client, remote, local, report)
	}, func() {
		b.loadLocal()
	})
}

func (a *App) startTransfer(label string, work func(context.Context, func(int64, int64)) error, done func()) {
	id := a.nextTransferID
	a.nextTransferID++
	ctx, cancel := context.WithCancel(a.ctx)
	a.transfer = &transferState{id: id, label: label, cancel: cancel}
	a.fileErr = ""

	go func() {
		err := work(ctx, func(done, total int64) {
			a.post(func() {
				if a.transfer != nil && a.transfer.id == id {
					a.transfer.done = done
					a.transfer.total = total
				}
			})
		})

		a.post(func() {
			var doneBytes, totalBytes int64
			if a.transfer != nil && a.transfer.id == id {
				doneBytes = a.transfer.done
				totalBytes = a.transfer.total
				a.transfer = nil
			}
			if err != nil {
				if errors.Is(err, context.Canceled) {
					a.status = "传输已取消"
				} else {
					a.fileErr = "传输失败: " + err.Error()
				}
				a.addTransferLog(transferRecord{
					Label: label, Done: doneBytes, Total: totalBytes, Err: err.Error(),
					Canceled: errors.Is(err, context.Canceled),
				})
				return
			}
			a.addTransferLog(transferRecord{Label: label, Done: doneBytes, Total: totalBytes})
			a.status = label + "完成"
			if done != nil {
				done()
			}
		})
	}()
}

func uploadFile(ctx context.Context, client *sftp.Client, local, remote string, report func(int64, int64)) error {
	src, err := os.Open(local)
	if err != nil {
		return err
	}
	defer src.Close()

	st, err := src.Stat()
	if err != nil {
		return err
	}

	tmp := remote + ".xu-part"
	dst, err := client.Create(tmp)
	if err != nil {
		return err
	}

	if err := copyWithProgress(ctx, dst, src, st.Size(), report); err != nil {
		dst.Close()
		client.Remove(tmp)
		return err
	}
	if err := dst.Close(); err != nil {
		client.Remove(tmp)
		return err
	}

	if err := client.PosixRename(tmp, remote); err != nil {
		_ = client.Remove(remote)
		if err2 := client.Rename(tmp, remote); err2 != nil {
			client.Remove(tmp)
			return err2
		}
	}
	return nil
}

func downloadFile(ctx context.Context, client *sftp.Client, remote, local string, report func(int64, int64)) error {
	src, err := client.Open(remote)
	if err != nil {
		return err
	}
	defer src.Close()

	st, err := src.Stat()
	if err != nil {
		return err
	}

	tmp := local + ".part"
	dst, err := os.Create(tmp)
	if err != nil {
		return err
	}

	if err := copyWithProgress(ctx, dst, src, st.Size(), report); err != nil {
		dst.Close()
		os.Remove(tmp)
		return err
	}
	if err := dst.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	if err := os.Rename(tmp, local); err != nil {
		// Windows refuses to replace an existing file with Rename.
		if removeErr := os.Remove(local); removeErr == nil {
			err = os.Rename(tmp, local)
		}
		if err != nil {
			os.Remove(tmp)
			return err
		}
	}
	return nil
}

func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, total int64, report func(int64, int64)) error {
	buf := make([]byte, 128<<10)
	var done int64
	var last time.Time

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			if report != nil && (time.Since(last) >= 100*time.Millisecond || (total > 0 && done >= total)) {
				last = time.Now()
				report(done, total)
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

func (a *App) promptNewFile(b *fileBrowser, remote bool) {
	title := "新建本地文件"
	if remote {
		title = "新建远程文件"
	}
	a.openPrompt(title, "", func(name string) {
		if !validName(name) {
			a.fileErr = "名称不能为空，且不能包含 / 或 \\"
			return
		}
		var err error
		if remote {
			if b.client == nil {
				a.fileErr = "SSH 连接不可用"
				return
			}
			var f interface{ Close() error }
			f, err = b.client.Create(path.Join(b.remoteDir, name))
			if err == nil && f != nil {
				err = f.Close()
			}
		} else {
			var f *os.File
			f, err = os.OpenFile(filepath.Join(b.localDir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err == nil && f != nil {
				err = f.Close()
			}
		}
		if err != nil {
			a.fileErr = "创建失败: " + err.Error()
			return
		}
		if remote {
			b.loadRemote()
		} else {
			b.loadLocal()
		}
	})
}

func (a *App) promptMkdir(b *fileBrowser, remote bool) {
	title := "新建本地文件夹"
	if remote {
		title = "新建远程文件夹"
	}
	a.openPrompt(title, "", func(name string) {
		if !validName(name) {
			a.fileErr = "名称不能为空，且不能包含 / 或 \\"
			return
		}
		var err error
		if remote {
			err = b.client.Mkdir(path.Join(b.remoteDir, name))
		} else {
			err = os.Mkdir(filepath.Join(b.localDir, name), 0o755)
		}
		if err != nil {
			a.fileErr = "创建失败: " + err.Error()
			return
		}
		if remote {
			b.loadRemote()
		} else {
			b.loadLocal()
		}
	})
}

func (a *App) promptRename(b *fileBrowser, remote bool) {
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

	oldName := f.Name()
	a.openPrompt("重命名为", oldName, func(name string) {
		if !validName(name) || name == oldName {
			a.fileErr = "名称无效或未改变"
			return
		}
		var err error
		if remote {
			err = b.client.Rename(path.Join(b.remoteDir, oldName), path.Join(b.remoteDir, name))
		} else {
			err = os.Rename(filepath.Join(b.localDir, oldName), filepath.Join(b.localDir, name))
		}
		if err != nil {
			a.fileErr = "重命名失败: " + err.Error()
			return
		}
		if remote {
			b.loadRemote()
		} else {
			b.loadLocal()
		}
	})
}

func (a *App) deleteSelected(b *fileBrowser, remote bool) {
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

	res, _ := mygo.Dialog.Message(mygo.MessageOptions{
		Type:               mygo.MessageWarning,
		Message:            "删除「" + f.Name() + "」？",
		Buttons:            []string{"取消", "删除"},
		DestructiveButtons: []int{1},
	})
	if res.Button != 1 {
		return
	}

	var err error
	if remote {
		if f.IsDir() {
			err = b.client.RemoveDirectory(path.Join(b.remoteDir, f.Name()))
		} else {
			err = b.client.Remove(path.Join(b.remoteDir, f.Name()))
		}
	} else {
		err = os.Remove(filepath.Join(b.localDir, f.Name()))
	}
	if err != nil {
		a.fileErr = "删除失败: " + err.Error()
		return
	}

	if remote {
		b.loadRemote()
	} else {
		b.loadLocal()
	}
}

func (a *App) openPrompt(title, initial string, action func(string)) {
	a.promptOpen = true
	a.promptTitle = title
	a.promptValue = initial
	a.promptAction = action
}

func (a *App) submitPrompt() {
	value := strings.TrimSpace(a.promptValue)
	action := a.promptAction
	a.promptOpen = false
	a.promptAction = nil
	if action != nil {
		action(value)
	}
}

func sortEntries(list []os.FileInfo) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].IsDir() != list[j].IsDir() {
			return list[i].IsDir()
		}
		return strings.ToLower(list[i].Name()) < strings.ToLower(list[j].Name())
	})
}

func readLocalDir(dir string) ([]os.FileInfo, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]os.FileInfo, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			continue
		}
		out = append(out, info)
	}
	sortEntries(out)
	return out, nil
}

func (b *fileBrowser) expandRemote(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return b.remoteDir
	}
	if p == "~" {
		return b.remoteHome
	}
	if strings.HasPrefix(p, "~/") {
		return path.Join(b.remoteHome, p[2:])
	}
	if !path.IsAbs(p) {
		return path.Join(b.remoteDir, p)
	}
	return path.Clean(p)
}

func expandLocal(p string) string {
	p = strings.TrimSpace(p)
	home, _ := os.UserHomeDir()
	if p == "" || p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}

func safeLocalName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimRight(name, ". ")
	if name == "" || name == "." || name == ".." {
		return "unnamed"
	}

	var b strings.Builder
	for _, r := range name {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	name = b.String()

	base := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	switch base {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		name = "_" + name
	}
	if name == "" {
		return "unnamed"
	}
	return name
}

func validName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" &&
		name != "." &&
		name != ".." &&
		!strings.ContainsAny(name, `/\`) &&
		!strings.ContainsRune(name, 0)
}

func selectedEntry(list []os.FileInfo, selected int) os.FileInfo {
	if selected < 0 || selected >= len(list) {
		return nil
	}
	return list[selected]
}

func entryAt(list []os.FileInfo, i int) string {
	if i < 0 || i >= len(list) {
		return ""
	}
	return list[i].Name()
}

func humanSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	v := float64(n)
	for _, unit := range units {
		v /= 1024
		if v < 1024 {
			return fmt.Sprintf("%.1f %s", v, unit)
		}
	}
	return fmt.Sprintf("%.1f EiB", v/1024)
}

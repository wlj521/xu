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
	"golang.org/x/crypto/ssh"
)

type fileBrowser struct {
	termID int64
	// ssh is the terminal's SSH connection, used to run rm on the server
	// itself. client is the SFTP subsystem opened over that same connection;
	// remote deletes fall back to it when the server does not offer exec.
	ssh        *ssh.Client
	client     *sftp.Client
	remoteHome string
	// localMode marks a browser that browses this machine's disk (a local
	// terminal). Its tree and paths use the local filesystem; client is nil.
	localMode bool

	localDir       string
	localPathInput string
	localPathEdit  bool
	localPathPick  bool
	localPathWait  bool
	localPathFocus ui.Handle
	localEntries   []os.FileInfo
	localSel       int
	localErr       string
	localState     ui.ListState

	remoteDir       string
	remotePathInput string
	remotePathEdit  bool
	remotePathPick  bool
	remotePathWait  bool
	remotePathFocus ui.Handle
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
	target string
	done   int64
	total  int64
	cancel context.CancelFunc
}

type transferRecord struct {
	Label    string
	Target   string
	Done     int64
	Total    int64
	Err      string
	Canceled bool
}

var saveDownloadDialog = func(opts mygo.SaveDialogOptions) (string, error) {
	return mygo.Dialog.Save(opts)
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
	// The remote pane always starts somewhere explicit, so a browser can be
	// inspected (and its breadcrumb rendered) before its first listing.
	if remoteHome == "" {
		remoteHome = "/"
	}
	b.remoteDir = remoteHome
	b.remotePathInput = remoteHome
	if client != nil {
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
	b.localMode = true
	b.revealLocalTreePath(localDir)
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
	ts := a.activeTerminal()
	if ts == nil {
		a.status = "请先连接一台 SSH 服务器"
		return
	}
	if ts.local {
		a.status = "本地终端不提供文件面板"
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
	// The far pane starts at the filesystem root, not the login home: the
	// dock shows the server's whole tree, not only /root.
	remoteHome := "/"

	b := newFileBrowser(ts.ID, sc, localDir, remoteHome)
	b.ssh = ts.client
	a.browsers[ts.ID] = b
	return b, nil
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

// localPane is the local terminal's file dock. It mirrors the remote dock
// (breadcrumb bar, directory tree, same table) so the two look the same.
func (a *App) localPane(c *ui.Context, b *fileBrowser, t *ui.Theme) {
	ui.Column(c).FillWidth().FillHeight().Children(func() {
		a.localPathBar(c, t, b)
		if a.transfer != nil {
			a.transferBar(c, t)
		}
		if b.localErr != "" {
			ui.Text(c, shortLabel(b.localErr, 100)).FontSize(9).TextColor(colRed).Padding(2, 12)
		}
		ui.Split(c, &a.fileSplit, func() {
			a.localFileTree(c, t, b)
		}, func() {
			a.localFileTable(c, t, b)
		}).Grow(1).FillWidth().SplitReset(260)
	})
}

func paneIconButton(c *ui.Context, t *ui.Theme, key string, icon *ui.SVG, tip string) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Size(25, 25).Radius(5).Center().Tooltip(tip).Label(tip)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		iconEl(c, icon, 13.5, colMuted)
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
		if remote && m.Item("修改权限…").Chosen() {
			a.openChmodDialog(b, true)
		}
		if m.Item("删除").Chosen() {
			a.deleteSelected(b, remote)
		}
		// Local entries keep the confirmed delete only; on a remote server the
		// quick path covers a folder as well, as rm -rf would.
		if remote && m.Item("快速删除（rm）").Chosen() {
			a.quickRemoveSelected(b, true)
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
		{Title: "大小", Width: 88, Align: ui.End},
		{Title: "类型", Width: 74},
		{Title: "修改时间", Width: 142},
		{Title: "权限", Width: 110},
		{Title: "用户/组", Width: 108},
	}
}

// bottomFileColumns is the compact set used by the dock: FinalShell-style
// name / size / modified layout, leaving the path and tree room to breathe.
func bottomFileColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{Title: "名称"},
		{Title: "大小", Width: 70, Align: ui.End},
		{Title: "修改时间", Width: 94, Align: ui.End},
	}
}

// fileTableTheme keeps row selection quiet: a translucent accent wash with
// normal text, instead of the bright accent fill used by primary controls.
func fileTableTheme(t *ui.Theme) *ui.Theme {
	tableTheme := *t
	tableTheme.Background = colTreeBg
	tableTheme.Surface = colTreeBg
	tableTheme.Accent = colAccentSoft
	tableTheme.AccentText = colText
	tableTheme.Focus = colAccentLine
	// Only a chosen row is painted: the pointer passing over the list on its
	// way to the toolbar must not light rows up.
	tableTheme.SurfaceHover = colTreeBg
	return &tableTheme
}

func bottomRemoteCell(c *ui.Context, b *fileBrowser, row, col int) {
	if row < 0 || row >= len(b.remoteEntries) {
		return
	}
	f := b.remoteEntries[row]
	chosen := b.remoteSel == row
	switch col {
	case 0:
		fileCell(c, f, "—", 0, chosen)
	case 1:
		fileCell(c, f, "—", 1, chosen)
	case 2:
		ui.Text(c, f.ModTime().Format("2006/01/02 15:04")).FontSize(10.2).
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

func fileVisual(f os.FileInfo) (*ui.SVG, ui.Color) {
	switch {
	case f.IsDir():
		return iconFolder, colCyan
	case f.Mode()&os.ModeSymlink != 0:
		return iconLink, colFileLink
	}

	name := strings.ToLower(f.Name())
	ext := path.Ext(name)
	switch {
	case strings.HasPrefix(name, ".env"):
		return iconFileConfig, colViolet
	case name == "makefile" || name == "dockerfile" || name == "containerfile" ||
		name == "cmakelists.txt" || name == ".gitignore" || name == ".dockerignore":
		return iconFileConfig, colViolet
	}
	switch ext {
	case ".sh", ".bash", ".zsh", ".fish", ".ps1", ".bat", ".cmd", ".py", ".pl", ".rb", ".lua":
		return iconFileScript, colGreen
	case ".go", ".rs", ".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".java", ".kt", ".kts",
		".swift", ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".vue", ".svelte", ".cs", ".php",
		".dart", ".scala", ".sql":
		return iconFileCode, colCyan
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".conf", ".cfg", ".config", ".env",
		".properties", ".xml", ".plist", ".lock", ".mod", ".sum", ".gradle", ".cmake":
		return iconFileConfig, colViolet
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".bmp", ".tif", ".tiff", ".avif":
		return iconFileImage, colBlue
	case ".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar", ".jar", ".war", ".ear",
		".deb", ".rpm", ".apk", ".iso":
		return iconFileArchive, colAmber
	case ".csv", ".tsv", ".xls", ".xlsx", ".ods":
		return iconFileTable, colGreen
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".m4a", ".opus", ".mp4", ".mkv", ".mov",
		".avi", ".webm", ".wmv", ".m4v":
		return iconFileMedia, colViolet
	case ".txt", ".md", ".markdown", ".log", ".rst", ".nfo", ".text", ".pdf",
		".doc", ".docx", ".odt", ".rtf":
		return iconFileText, colMuted
	}
	if f.Mode().Perm()&0o111 != 0 {
		return iconFileScript, colGreen
	}
	return iconFile, colMuted
}

// fileTypeIcon shows the entry's type, in the type's color only for the row
// chosen, as the folder tree colors only the folder it is in: the list stays
// quiet until something is selected.
func fileTypeIcon(c *ui.Context, f os.FileInfo, chosen bool) ui.Element {
	icon, color := fileVisual(f)
	if !chosen {
		color = colMuted
	}
	tile := ui.Box(c).Size(22, 22).Shrink(0).Radius(6).
		Background(color.Alpha(0.11)).Center()
	tile.Children(func() {
		iconEl(c, icon, 14.5, color)
	})
	return tile
}

func fileCell(c *ui.Context, f os.FileInfo, owner string, col int, chosen bool) {
	switch col {
	case 0:
		name := f.Name()
		if f.IsDir() {
			name += "/"
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			fileTypeIcon(c, f, chosen)
			ui.Text(c, name).FontSize(12.2).Font("Cascadia Code").
				TextColor(condColor(chosen, colText, colText2)).NoWrap()
		})
	case 1:
		if !f.IsDir() {
			ui.Text(c, humanSize(f.Size())).FontSize(10).Font("Cascadia Code").
				TextColor(colMuted).TextAlign(ui.End).NoWrap()
		}
	case 2:
		ui.Text(c, fileKind(f)).FontSize(10).Font("Cascadia Code").TextColor(colMuted).NoWrap()
	case 3:
		ui.Text(c, f.ModTime().Format("2006/01/02 15:04")).FontSize(10).
			Font("Cascadia Code").TextColor(colMuted).NoWrap()
	case 4:
		ui.Text(c, f.Mode().Perm().String()).FontSize(10).Font("Cascadia Code").
			TextColor(colFilePerm).NoWrap()
	case 5:
		ui.Text(c, owner).FontSize(10).Font("Cascadia Code").TextColor(colMuted).NoWrap()
	}
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
	fileCell(c, f, owner, col, b.remoteSel == row)
}

func (a *App) navigateLocal(b *fileBrowser, dir string) {
	if err := b.setLocalDir(dir); err != nil {
		b.localErr = err.Error()
		return
	}
	b.revealLocalTreePath(b.localDir)
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

	// Keep the open tree branch in sync with the table, like loadRemote does.
	if b.localMode && b.treeKids != nil {
		kids := make([]string, 0, len(entries))
		for _, f := range entries {
			if f.IsDir() {
				kids = append(kids, filepath.Join(dir, f.Name()))
			}
		}
		sort.Strings(kids)
		b.treeKids[dir] = kids
		b.treeLoaded[dir] = true
		delete(b.treeErr, dir)
	}
	return nil
}

func (a *App) navigateRemote(b *fileBrowser, dir string) {
	if b.client == nil {
		b.remoteErr = "SFTP 不可用"
		return
	}
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
	if b.localMode {
		entries, err := readLocalDir(dir)
		if err != nil {
			b.treeErr[dir] = err.Error()
			return
		}
		kids := make([]string, 0, len(entries))
		for _, f := range entries {
			if f.IsDir() {
				kids = append(kids, filepath.Join(dir, f.Name()))
			}
		}
		sort.Strings(kids)
		b.treeKids[dir] = kids
		delete(b.treeErr, dir)
		return
	}
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

// revealLocalTreePath loads and opens the local tree down to dir, so the
// active folder's branch is visible when the pane appears.
func (b *fileBrowser) revealLocalTreePath(dir string) {
	if !b.localMode {
		return
	}
	root, segs := localPathParts(dir)
	b.loadTreeDir(root)
	b.treeOpen[root] = true
	acc := root
	for _, seg := range segs {
		acc = filepath.Join(acc, seg)
		b.treeOpen[acc] = true
		b.loadTreeDir(acc)
	}
}

// localPathParts splits a local directory into its volume root (C:\ or a
// UNC share root) and the folders below it.
func localPathParts(dir string) (string, []string) {
	dir = filepath.Clean(dir)
	vol := filepath.VolumeName(dir)
	root := vol + string(filepath.Separator)
	if vol == "" {
		root = string(filepath.Separator)
	}
	rest := strings.TrimPrefix(dir, root)
	segs := make([]string, 0, 4)
	for _, seg := range strings.Split(rest, string(filepath.Separator)) {
		if seg != "" {
			segs = append(segs, seg)
		}
	}
	return root, segs
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
	if b.client == nil {
		b.remoteErr = "SFTP 不可用"
		return
	}
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

	local, err := a.chooseDownloadDestination(b, f)
	if err != nil {
		a.fileErr = "无法选择下载位置: " + err.Error()
		return
	}
	if local == "" {
		return
	}
	local = filepath.Clean(local)
	b.localDir = filepath.Dir(local)

	remote := path.Join(b.remoteDir, f.Name())
	a.startTransferTo("下载 "+f.Name(), local, func(ctx context.Context, report func(int64, int64)) error {
		return downloadFile(ctx, b.client, remote, local, report)
	}, func() {
		b.loadLocal()
	})
}

func (a *App) chooseDownloadDestination(b *fileBrowser, f os.FileInfo) (string, error) {
	return saveDownloadDialog(mygo.SaveDialogOptions{
		Parent:            a.win,
		Title:             "选择下载位置",
		DefaultPath:       filepath.Join(b.localDir, safeLocalName(f.Name())),
		ButtonLabel:       "保存",
		Message:           "选择远程文件保存到本机的位置",
		CreateDirectories: true,
	})
}

func (a *App) startTransfer(label string, work func(context.Context, func(int64, int64)) error, done func()) {
	a.startTransferTo(label, "", work, done)
}

func (a *App) startTransferTo(label, target string, work func(context.Context, func(int64, int64)) error, done func()) {
	id := a.nextTransferID
	a.nextTransferID++
	ctx, cancel := context.WithCancel(a.ctx)
	a.transfer = &transferState{id: id, label: label, target: target, cancel: cancel}
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
					Label: label, Target: target, Done: doneBytes, Total: totalBytes, Err: err.Error(),
					Canceled: errors.Is(err, context.Canceled),
				})
				return
			}
			a.addTransferLog(transferRecord{Label: label, Target: target, Done: doneBytes, Total: totalBytes})
			if target != "" {
				a.status = label + "完成 · " + shortLabel(target, 70)
			} else {
				a.status = label + "完成"
			}
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

func (a *App) quickRemoveSelected(b *fileBrowser, remote bool) {
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

	if !remote {
		if f.IsDir() {
			a.fileErr = "快速删除仅支持文件，目录请使用“删除”"
			return
		}
		if err := os.Remove(filepath.Join(b.localDir, f.Name())); err != nil {
			a.fileErr = "删除失败: " + err.Error()
			return
		}
		a.status = "已删除 " + f.Name()
		b.loadLocal()
		return
	}

	if b.client == nil {
		a.fileErr = "SSH 连接不可用"
		return
	}
	target := path.Join(b.remoteDir, f.Name())

	// A file is done in one command; a folder can take a while even on the
	// server, so its rm runs off the UI thread and the status line reports it.
	if !f.IsDir() {
		if err := remoteQuickRemove(a.ctx, b.ssh, b.client, target); err != nil {
			a.fileErr = "删除失败: " + err.Error()
			return
		}
		a.status = "已删除 " + f.Name()
		b.loadRemote()
		return
	}

	sshClient, client := b.ssh, b.client
	a.fileErr = ""
	a.status = "正在删除 " + f.Name() + "…"
	go func() {
		err := remoteQuickRemove(a.ctx, sshClient, client, target)
		a.post(func() {
			b.loadRemote()
			switch {
			case err == nil:
				a.status = "已删除 " + f.Name()
			case errors.Is(err, context.Canceled):
				a.status = "已停止删除"
			default:
				a.status = ""
				a.fileErr = "删除失败: " + err.Error()
			}
		})
	}()
}

// remoteQuickRemove is the menu's "快速删除（rm）" on a remote entry. With the
// terminal's SSH connection it runs rm -rf on the server, so a folder costs
// one round trip instead of one per entry; when the server refuses exec, or
// the path cannot be quoted for a shell, the SFTP tree walk finishes the job.
func remoteQuickRemove(ctx context.Context, sshClient *ssh.Client, sftpClient *sftp.Client, target string) error {
	if sshClient != nil {
		err := runRemoteRM(ctx, sshClient, target)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errRemoteRMUnavailable) {
			return err
		}
	}
	if sftpClient == nil {
		return errors.New("SSH 连接不可用")
	}
	_, err := removeTree(ctx, true, sftpClient, target, nil)
	return err
}

var errRemoteRMUnavailable = errors.New("remote rm unavailable")

// runRemoteRM deletes target with the server's own rm -rf. The path is quoted
// for a POSIX shell; 126/127 mean the shell could not run rm at all, which
// leaves the SFTP walk as a fallback. Any other failure is rm's own and is
// returned with its stderr, because the entry may be half gone.
func runRemoteRM(ctx context.Context, client *ssh.Client, target string) error {
	quoted, ok := shellQuote(target)
	if !ok {
		return errRemoteRMUnavailable
	}

	sess, err := client.NewSession()
	if err != nil {
		return errRemoteRMUnavailable
	}
	defer sess.Close()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = sess.Signal(ssh.SIGKILL)
			sess.Close()
		case <-done:
		}
	}()

	out, err := sess.CombinedOutput("rm -rf -- " + quoted)
	close(done)

	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		return nil
	}

	var exit *ssh.ExitError
	if errors.As(err, &exit) {
		if code := exit.ExitStatus(); code == 126 || code == 127 {
			return errRemoteRMUnavailable
		}
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return errRemoteRMUnavailable
}

// shellQuote wraps s in single quotes for a POSIX shell. Control characters
// that would let a path break the command line are refused, so the caller can
// fall back to SFTP instead of risking a different target.
func shellQuote(s string) (string, bool) {
	if s == "" || strings.ContainsAny(s, "\x00\n\r") {
		return "", false
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'", true
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

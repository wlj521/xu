package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/sshx"
)

func TestDockFilePanelKeepsTreeWithoutHeader(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{
		ID: 1, Title: "root@test",
		profile: sshx.Profile{Host: "test", Port: 22, Username: "root"},
	}}
	a.syncTabs()
	a.bottomFiles = true

	b := newFileBrowser(1, nil, t.TempDir(), "/")
	b.remoteDir = "/"
	b.remotePathInput = "/"
	b.remoteEntries = []os.FileInfo{fakeInfo{name: "etc", dir: true}}
	b.treeKids["/"] = []string{"/etc"}
	b.treeLoaded["/"] = true
	a.browsers[1] = b

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if _, ok := tt.Find("etc"); !ok {
		t.Fatalf("remote tree entry missing; texts = %q", tt.Texts())
	}
	if _, ok := tt.Find("etc/"); !ok {
		t.Fatalf("remote file row missing; texts = %q", tt.Texts())
	}
	if _, ok := tt.Find("目录"); ok {
		t.Fatalf("the tree header must be gone; texts = %q", tt.Texts())
	}
	if tt.HasText("本地") {
		t.Fatalf("the local pane must be gone from SSH sessions; texts = %q", tt.Texts())
	}
}

func TestDefaultRemoteDirIsRoot(t *testing.T) {
	b := newFileBrowser(1, nil, t.TempDir(), "/")
	if b.remoteDir != "/" || b.remotePathInput != "/" {
		t.Fatalf("remote starts at %q / %q, want /", b.remoteDir, b.remotePathInput)
	}
}

// clickText clicks the middle of the box showing s, which works for rows
// whose cell content sits under a clickable wrapper.
func clickText(tt *ui.Tester, s string) error {
	box, ok := tt.Find(s)
	if !ok {
		return fmt.Errorf("no element shows %q", s)
	}
	tt.ClickAt(box.X+box.W/2, box.Y+box.H/2)
	return nil
}

func newRemoteDockApp(t *testing.T) (*App, *fileBrowser, *ui.Tester) {
	t.Helper()
	a := New()
	a.stop()
	a.terms = []*termSession{{
		ID: 1, Title: "root@test",
		profile: sshx.Profile{Host: "test", Port: 22, Username: "root"},
	}}
	a.syncTabs()
	a.bottomFiles = true

	b := newFileBrowser(1, nil, t.TempDir(), "/")
	b.remoteDir = "/"
	b.remotePathInput = "/"
	b.remoteEntries = []os.FileInfo{
		fakeInfo{name: "etc", dir: true},
		fakeInfo{name: "notes.txt", size: 3},
	}
	b.treeKids["/"] = []string{"/etc"}
	b.treeLoaded["/"] = true
	a.browsers[1] = b

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	return a, b, tt
}

func TestFileDockKeepsTreeAndColumnsWithoutHeader(t *testing.T) {
	_, _, tt := newRemoteDockApp(t)
	for _, want := range []string{"文件名", "类型", "权限", "用户/组"} {
		if !tt.HasText(want) {
			t.Fatalf("file dock missing %q; texts = %q", want, tt.Texts())
		}
	}
	if _, ok := tt.Find("目录"); ok {
		t.Fatalf("the tree header must be gone; texts = %q", tt.Texts())
	}
	tree, ok := tt.Find("etc")
	if !ok {
		t.Fatalf("remote tree entry missing; texts = %q", tt.Texts())
	}
	table, ok := tt.Find("文件名")
	if !ok {
		t.Fatalf("file table missing; texts = %q", tt.Texts())
	}
	if table.X < tree.X+100 {
		t.Fatalf("tree at %.1f and table at %.1f do not form two columns", tree.X, table.X)
	}
}

func TestFileTableFillsDockWidth(t *testing.T) {
	_, _, tt := newRemoteDockApp(t)
	tt.SetSize(2100, 900)
	tt.Frame()

	last, ok := tt.Find("用户/组")
	if !ok {
		t.Fatalf("last column missing; texts = %q", tt.Texts())
	}
	if right := last.X + last.W; right < 1900 {
		t.Fatalf("the file table ends at %.1f in a 2100-wide window: the dock leaves the right side empty", right)
	}
}

func TestFileToolbarUpNavigatesToParent(t *testing.T) {
	_, b, tt := newRemoteDockApp(t)
	b.remoteDir = "/projects/jt808"
	b.remotePathInput = b.remoteDir
	tt.Frame()

	if err := tt.Click("返回上一级"); err != nil {
		t.Fatalf("up button: %v", err)
	}
	tt.Frame()
	if b.remoteErr != "SFTP 不可用" {
		t.Fatalf("up button did not navigate: err=%q", b.remoteErr)
	}
}

func TestFileErrorIsShownOnce(t *testing.T) {
	a, _, tt := newRemoteDockApp(t)
	a.fileErr = "测试提示"
	tt.Frame()

	count := 0
	for _, text := range tt.Texts() {
		if text == "测试提示" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("file error shown %d times, want once", count)
	}
}

func TestChooseDownloadDestination(t *testing.T) {
	a := New()
	b := newFileBrowser(1, nil, t.TempDir(), "")
	target := filepath.Join(t.TempDir(), "saved.sh")
	old := saveDownloadDialog
	defer func() { saveDownloadDialog = old }()

	var got mygo.SaveDialogOptions
	saveDownloadDialog = func(opts mygo.SaveDialogOptions) (string, error) {
		got = opts
		return target, nil
	}
	path, err := a.chooseDownloadDestination(b, fakeInfo{name: "consumer.sh"})
	if err != nil {
		t.Fatalf("choose destination: %v", err)
	}
	if path != target {
		t.Fatalf("chosen path = %q, want %q", path, target)
	}
	wantDefault := filepath.Join(b.localDir, "consumer.sh")
	if got.Title != "选择下载位置" || got.DefaultPath != wantDefault || got.ButtonLabel != "保存" {
		t.Fatalf("dialog options = %+v, want the download default", got)
	}
}

func TestTransferLogShowsDownloadPath(t *testing.T) {
	a := New()
	a.terms = []*termSession{{ID: 1, Title: "root@test"}}
	a.syncTabs()
	a.activeTerm = 0
	a.bottomFiles = true
	a.bottomFileTab = int(dockTransfers)
	a.transferLog = []transferRecord{{
		Label: "下载 consumer.sh", Target: `C:\Downloads\consumer.sh`, Done: 375, Total: 375,
	}}

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if !tt.HasText(`保存到 C:\Downloads\consumer.sh`) {
		t.Fatalf("download path missing from transfer log; texts = %q", tt.Texts())
	}
}

func TestPathBarBlankClickEditsAndSelectsPath(t *testing.T) {
	_, b, tt := newRemoteDockApp(t)
	b.remoteDir = "/projects/jt808/platform"
	b.remotePathInput = b.remoteDir
	tt.Frame()

	crumbs, ok := tt.Find("platform")
	if !ok {
		t.Fatalf("current crumb missing; texts = %q", tt.Texts())
	}
	tt.ClickAt(crumbs.X+crumbs.W+120, crumbs.Y+crumbs.H/2)
	tt.Frame()
	tt.Frame()
	if !b.remotePathEdit {
		t.Fatal("clicking the empty path bar did not open the path editor")
	}
	if active, _ := tt.TextInputActive(); !active {
		t.Fatal("the path editor does not have the keyboard focus")
	}

	// The whole path starts selected, so typing replaces it at once.
	tt.Type("x")
	tt.Frame()
	if b.remotePathInput != "x" {
		t.Fatalf("typing did not replace the selected path: %q", b.remotePathInput)
	}

	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if b.remotePathEdit || b.remotePathInput != b.remoteDir {
		t.Fatalf("Escape did not restore the crumbs: edit=%v path=%q", b.remotePathEdit, b.remotePathInput)
	}
}

func TestPathBarCurrentCrumbEditsPath(t *testing.T) {
	_, b, tt := newRemoteDockApp(t)
	b.remoteDir = "/projects/jt808/platform"
	b.remotePathInput = b.remoteDir
	tt.Frame()

	if err := clickText(tt, "platform"); err != nil {
		t.Fatalf("click current crumb: %v", err)
	}
	tt.Frame()
	if !b.remotePathEdit {
		t.Fatal("clicking the current crumb did not open the path editor")
	}
}

func TestPathBarEditorClosesWhenFocusLeaves(t *testing.T) {
	_, b, tt := newRemoteDockApp(t)
	b.remoteDir = "/projects/jt808/platform"
	b.remotePathInput = b.remoteDir
	tt.Frame()

	crumbs, ok := tt.Find("platform")
	if !ok {
		t.Fatalf("current crumb missing; texts = %q", tt.Texts())
	}
	tt.ClickAt(crumbs.X+crumbs.W+120, crumbs.Y+crumbs.H/2)
	tt.Frame()
	tt.Frame()
	if !b.remotePathEdit {
		t.Fatal("the path editor did not open")
	}

	if err := clickText(tt, "文件名"); err != nil {
		t.Fatalf("click table header: %v", err)
	}
	tt.Frame()
	if b.remotePathEdit {
		t.Fatal("the path editor stayed open after the focus left it")
	}
	if b.remotePathInput != b.remoteDir {
		t.Fatalf("leaving the editor left the path as %q", b.remotePathInput)
	}
}

func TestPathBarParentCrumbStillNavigates(t *testing.T) {
	_, b, tt := newRemoteDockApp(t)
	b.remoteDir = "/projects/jt808/platform"
	b.remotePathInput = b.remoteDir
	tt.Frame()

	if err := clickText(tt, "projects"); err != nil {
		t.Fatalf("click parent crumb: %v", err)
	}
	tt.Frame()
	if b.remotePathEdit {
		t.Fatal("clicking a parent crumb opened the editor instead of navigating")
	}
	if b.remoteErr != "SFTP 不可用" {
		t.Fatalf("parent crumb did not navigate: err=%q", b.remoteErr)
	}
}

func TestSingleClickFolderOpensNextLevel(t *testing.T) {
	_, b, tt := newRemoteDockApp(t)
	if err := clickText(tt, "etc/"); err != nil {
		t.Fatalf("click folder: %v", err)
	}
	if b.remoteSel != 0 {
		t.Fatalf("folder row not chosen: sel=%d", b.remoteSel)
	}
	// A browser without SFTP cannot open the folder: the attempt proves the
	// single click navigated instead of waiting for a double click.
	if b.remoteErr != "SFTP 不可用" {
		t.Fatalf("folder click did not try to open it: err=%q", b.remoteErr)
	}
}

func TestSingleClickFileOnlyChoosesIt(t *testing.T) {
	_, b, tt := newRemoteDockApp(t)
	if err := clickText(tt, "notes.txt"); err != nil {
		t.Fatalf("click file: %v", err)
	}
	if b.remoteSel != 1 {
		t.Fatalf("file row not chosen: sel=%d", b.remoteSel)
	}
	if b.remoteErr != "" {
		t.Fatalf("clicking a file must not navigate: err=%q", b.remoteErr)
	}
}

func TestFileTableSelectionUsesPaleBackground(t *testing.T) {
	a, b, tt := newRemoteDockApp(t)
	a.settings.Theme = 2
	tt.Frame()
	if err := clickText(tt, "notes.txt"); err != nil {
		t.Fatalf("click file: %v", err)
	}
	if b.remoteSel != 1 {
		t.Fatalf("file row not chosen: sel=%d", b.remoteSel)
	}

	label, ok := tt.Find("notes.txt")
	if !ok {
		t.Fatalf("selected row missing; texts = %q", tt.Texts())
	}
	// Sample the empty part of the selected row, clear of its icon and text.
	c := tt.Image().RGBAAt(int(label.X+200), int(label.Y+label.H/2))
	if c.R < 220 || c.G < 220 || c.B < 220 || c.B < c.R {
		t.Fatalf("selected row background = rgba(%d,%d,%d,%d), want a pale blue wash", c.R, c.G, c.B, c.A)
	}
}

func TestFileVisualsByType(t *testing.T) {
	cases := []struct {
		name string
		dir  bool
		mode os.FileMode
		want *ui.SVG
	}{
		{"projects", true, 0, iconFolder},
		{"deploy.sh", false, 0, iconFileScript},
		{"main.go", false, 0, iconFileCode},
		{"app.env", false, 0, iconFileConfig},
		{"cover.png", false, 0, iconFileImage},
		{"release.tar.gz", false, 0, iconFileArchive},
		{"report.csv", false, 0, iconFileTable},
		{"demo.mp4", false, 0, iconFileMedia},
		{"README.md", false, 0, iconFileText},
		{"current", false, os.ModeSymlink, iconLink},
		{"server", false, 0o755, iconFileScript},
		{"payload.bin", false, 0, iconFile},
	}
	for _, tc := range cases {
		got, _ := fileVisual(fakeInfo{name: tc.name, dir: tc.dir, mode: tc.mode})
		if got != tc.want {
			t.Errorf("fileVisual(%q) chose the wrong icon", tc.name)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:           "0 B",
		512:         "512 B",
		1024:        "1.0 KiB",
		1024 * 1024: "1.0 MiB",
	}
	for in, want := range cases {
		if got := humanSize(in); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestSafeLocalName(t *testing.T) {
	cases := map[string]string{
		"a:b.txt": "a_b.txt",
		"CON":     "_CON",
		"con.txt": "_con.txt",
		"..":      "unnamed",
		"a. ":     "a",
		"配置.txt":  "配置.txt",
	}
	for in, want := range cases {
		if got := safeLocalName(in); got != want {
			t.Errorf("safeLocalName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidName(t *testing.T) {
	for _, name := range []string{"a", "配置", "a b", ".hidden"} {
		if !validName(name) {
			t.Errorf("validName(%q) = false", name)
		}
	}
	for _, name := range []string{"", " ", ".", "..", "a/b", `a\b`, "a\x00b"} {
		if validName(name) {
			t.Errorf("validName(%q) = true", name)
		}
	}
}

type fakeInfo struct {
	name string
	dir  bool
	size int64
	mode os.FileMode
}

func (f fakeInfo) Name() string { return f.name }
func (f fakeInfo) Size() int64  { return f.size }
func (f fakeInfo) Mode() os.FileMode {
	if f.dir {
		return os.ModeDir | f.mode
	}
	return f.mode
}
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.dir }
func (f fakeInfo) Sys() any           { return nil }

func TestSortEntries(t *testing.T) {
	list := []os.FileInfo{
		fakeInfo{name: "b.txt"},
		fakeInfo{name: "adir", dir: true},
		fakeInfo{name: "A.txt"},
	}
	sortEntries(list)

	if got := []string{list[0].Name(), list[1].Name(), list[2].Name()}; got[0] != "adir" || got[1] != "A.txt" || got[2] != "b.txt" {
		t.Fatalf("sorted names = %v", got)
	}
}

func TestPromptNewLocalFile(t *testing.T) {
	dir := t.TempDir()
	b := newFileBrowser(1, nil, dir, "")
	a := New()
	a.promptNewFile(b, false)
	a.promptValue = "notes.txt"
	a.submitPrompt()
	if a.fileErr != "" {
		t.Fatalf("new file: %s", a.fileErr)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatalf("new file not created: %v", err)
	}
}

func TestFileRowContextMenu(t *testing.T) {
	a := New()
	a.terms = []*termSession{{ID: 1, Title: "root@test", profile: sshx.Profile{Host: "test", Port: 22, Username: "root"}}}
	a.activeTerm = 0
	a.bottomFiles = true
	b := newFileBrowser(1, nil, t.TempDir(), "/tmp")
	b.remoteDir = "/tmp"
	b.remotePathInput = "/tmp"
	b.remoteEntries = []os.FileInfo{fakeInfo{name: "config.yaml", size: 12}}
	a.browsers[1] = b

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	box, ok := tt.Find("config.yaml")
	if !ok {
		t.Fatalf("file row missing; texts = %q", tt.Texts())
	}
	tt.RightClickAt(box.X+box.W/2, box.Y+box.H/2)
	menu := tt.Menu()
	want := map[string]bool{"新建文件": false, "新建文件夹": false, "删除": false, "下载到本机": false}
	for _, item := range menu {
		if _, ok := want[item]; ok {
			want[item] = true
		}
	}
	for item, found := range want {
		if !found {
			t.Fatalf("context menu %q missing %q", menu, item)
		}
	}
}

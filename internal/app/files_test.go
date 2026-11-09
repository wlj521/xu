package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"xu/internal/sshx"
)

func TestDockFilePanelHasNoLocalPane(t *testing.T) {
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
	a.browsers[1] = b

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if !tt.HasText("目录") {
		t.Fatalf("remote tree pane missing; texts = %q", tt.Texts())
	}
	if !tt.HasText("etc") {
		t.Fatalf("remote entries missing; texts = %q", tt.Texts())
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
	a.browsers[1] = b

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	return a, b, tt
}

func TestFileDockShowsTreeAndFullColumns(t *testing.T) {
	_, _, tt := newRemoteDockApp(t)
	for _, want := range []string{"目录", "类型", "权限", "用户/组", "Divider"} {
		if !tt.HasText(want) {
			t.Fatalf("file dock missing %q; texts = %q", want, tt.Texts())
		}
	}
	if tt.HasText("本地") {
		t.Fatalf("the local pane must be gone from SSH sessions; texts = %q", tt.Texts())
	}
}

func TestFileTreeRowsAlignLeft(t *testing.T) {
	_, b, tt := newRemoteDockApp(t)
	b.treeKids["/"] = []string{"/etc"}
	b.treeLoaded["/"] = true
	b.treeOpen["/"] = true
	tt.Frame()

	label, ok := tt.Find("etc")
	if !ok {
		t.Fatalf("tree entry missing; texts = %q", tt.Texts())
	}
	head, ok := tt.Find("目录")
	if !ok {
		t.Fatalf("tree header missing; texts = %q", tt.Texts())
	}
	if label.X > head.X+70 {
		t.Fatalf("tree entry starts at %.1f, header at %.1f: the rows are not left aligned", label.X, head.X)
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

func TestFileTreeDividerResizes(t *testing.T) {
	a, _, tt := newRemoteDockApp(t)
	tt.SetSize(2100, 900)
	tt.Frame()

	head, ok := tt.Find("目录")
	if !ok {
		t.Fatalf("tree header missing; texts = %q", tt.Texts())
	}
	name, ok := tt.Find("文件名")
	if !ok {
		t.Fatalf("file header missing; texts = %q", tt.Texts())
	}

	// The tree panel starts after the header's 9 DIP padding, 12 DIP icon
	// and 6 DIP gap; its right edge is the split's divider.
	treeLeft := head.X - 27
	divider := treeLeft + a.fileSplit
	y := head.Y + 80
	tt.Press(divider+0.5, y)
	tt.Move(divider+60.5, y)
	tt.Release(divider+60.5, y)
	tt.Frame()

	if a.fileSplit < 265 || a.fileSplit > 275 {
		t.Fatalf("dragged the divider 60 DIPs right, tree width = %.1f", a.fileSplit)
	}
	moved, ok := tt.Find("文件名")
	if !ok {
		t.Fatalf("file header missing after drag; texts = %q", tt.Texts())
	}
	if moved.X < name.X+55 || moved.X > name.X+65 {
		t.Fatalf("the file pane moved from %.1f to %.1f, want 60 DIPs", name.X, moved.X)
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
}

func (f fakeInfo) Name() string { return f.name }
func (f fakeInfo) Size() int64  { return f.size }
func (f fakeInfo) Mode() os.FileMode {
	if f.dir {
		return os.ModeDir
	}
	return 0
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

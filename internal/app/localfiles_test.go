package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestLocalDockMatchesRemoteLayout pins the local file dock to the remote
// one: breadcrumbs, a folder tree, and the same full column set.
func TestLocalPathParts(t *testing.T) {
	cases := []struct {
		dir  string
		root string
		segs []string
	}{
		{`C:\`, `C:\`, nil},
		{`C:\Users\51237`, `C:\`, []string{"Users", "51237"}},
		{`\\server\share\dir`, `\\server\share\`, []string{"dir"}},
	}
	for _, c := range cases {
		root, segs := localPathParts(c.dir)
		if root != c.root {
			t.Fatalf("localPathParts(%q) root = %q, want %q", c.dir, root, c.root)
		}
		if len(segs) != len(c.segs) {
			t.Fatalf("localPathParts(%q) segs = %v, want %v", c.dir, segs, c.segs)
		}
		for i := range segs {
			if segs[i] != c.segs[i] {
				t.Fatalf("localPathParts(%q) segs = %v, want %v", c.dir, segs, c.segs)
			}
		}
	}
}

func TestLocalDockMatchesRemoteLayout(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "PowerShell 7", local: true}}
	a.syncTabs()
	a.bottomFiles = true

	b := newFileBrowser(1, nil, dir, "")
	b.localMode = true
	if err := b.setLocalDir(dir); err != nil {
		t.Fatalf("list %s: %v", dir, err)
	}
	b.revealLocalTreePath(dir)
	a.browsers[1] = b

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()

	for _, want := range []string{
		"文件名", "大小", "类型", "修改时间", "权限", "用户/组",
		"sub/", "notes.txt", "sub", "返回上一级",
	} {
		if !tt.HasText(want) {
			t.Fatalf("local file dock is missing %q; texts = %q", want, tt.Texts())
		}
	}

	// The old local-only header and footer are gone; the path is crumbs now.
	if _, ok := tt.Find("本机"); ok {
		t.Fatalf("the old local pane header is still there; texts = %q", tt.Texts())
	}
	root, segs := localPathParts(dir)
	if !tt.HasText(root) {
		t.Fatalf("the volume crumb %q is missing; texts = %q", root, tt.Texts())
	}
	if len(segs) > 0 && !tt.HasText(segs[len(segs)-1]) {
		t.Fatalf("the current folder crumb %q is missing; texts = %q", segs[len(segs)-1], tt.Texts())
	}

	// A folder row opens on a double click, like the remote table.
	box, ok := tt.Find("sub/")
	if !ok {
		t.Fatalf("folder row missing; texts = %q", tt.Texts())
	}
	x, y := box.X+40, box.Y+box.H/2
	tt.ClickAt(x, y)
	tt.Frame()
	if b.localDir == sub {
		t.Fatal("a single click opened the folder")
	}
	tt.ClickAt(x, y)
	tt.Frame()
	if b.localDir != sub {
		t.Fatalf("localDir = %q, want %q after double-clicking the folder", b.localDir, sub)
	}
}

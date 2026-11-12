package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

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

func TestLocalTerminalHidesBottomDock(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "本地终端", local: true}}
	a.syncTabs()
	a.bottomFiles = true

	b := newFileBrowser(1, nil, dir, "")
	b.localMode = true
	if err := b.setLocalDir(dir); err != nil {
		t.Fatalf("list %s: %v", dir, err)
	}
	a.browsers[1] = b

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()

	for _, unwanted := range []string{"文件名", "notes.txt", "传输", "隧道", "命令"} {
		if tt.HasText(unwanted) {
			t.Fatalf("local terminal shows bottom dock content %q; texts = %q", unwanted, tt.Texts())
		}
	}
	if !tt.HasText("本地终端") {
		t.Fatalf("local terminal is missing; texts = %q", tt.Texts())
	}
}

func TestLocalTerminalIgnoresDockAndFileActions(t *testing.T) {
	a := New()
	a.stop()
	a.terms = []*termSession{{ID: 1, Title: "本地终端", local: true}}
	a.syncTabs()
	a.activeTerm = 0
	a.bottomFiles = false

	a.toggleDock()
	if a.bottomFiles || a.status == "" {
		t.Fatalf("toggleDock local: bottomFiles=%v status=%q", a.bottomFiles, a.status)
	}

	a.openFiles()
	if a.bottomFiles || a.status == "" {
		t.Fatalf("openFiles local: bottomFiles=%v status=%q", a.bottomFiles, a.status)
	}
}

package app

import (
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"
)

func addTestEditor(t *testing.T, a *App, b *fileBrowser, name, body string) *editorSession {
	t.Helper()
	local := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(local, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	buf := editor.NewBuffer(body)
	a.nextEditorID++
	es := &editorSession{
		id:       a.nextEditorID,
		termID:   b.termID,
		browser:  b,
		name:     name,
		remote:   path.Join("/", name),
		local:    local,
		buffer:   buf,
		state:    &editor.State{Buffer: buf},
		baseSize: int64(len(body)),
		baseMod:  time.Now(),
	}
	a.editorSessions = append(a.editorSessions, es)
	a.editorIndex = len(a.editorSessions) - 1
	return es
}

func TestEditorWindowShowsMultipleFilesAsTabs(t *testing.T) {
	a, b, tt := newRemoteDockApp(t)
	addTestEditor(t, a, b, "notes.txt", "notes\n")
	addTestEditor(t, a, b, "config.yaml", "port: 22\n")
	tt.Frame()

	for _, want := range []string{"2 个文件", "编辑文件 notes.txt", "编辑文件 config.yaml"} {
		if !tt.HasText(want) {
			t.Fatalf("editor window missing %q; texts = %q", want, tt.Texts())
		}
	}
	if got := a.activeEditor(); got == nil || got.name != "config.yaml" {
		t.Fatalf("active editor = %#v, want config.yaml", got)
	}

	if err := tt.Click("编辑文件 notes.txt"); err != nil {
		t.Fatalf("switch editor tab: %v", err)
	}
	tt.Frame()
	if got := a.activeEditor(); got == nil || got.name != "notes.txt" {
		t.Fatalf("active editor after tab click = %#v, want notes.txt", got)
	}
	if !tt.HasText("/notes.txt") {
		t.Fatalf("active file path is not shown; texts = %q", tt.Texts())
	}
}

func TestEditorWindowKeepsFileDockInteractive(t *testing.T) {
	a, b, tt := newRemoteDockApp(t)
	addTestEditor(t, a, b, "config.yaml", "port: 22\n")
	tt.Frame()

	row, ok := tt.Find("notes.txt")
	if !ok {
		t.Fatalf("file row missing; texts = %q", tt.Texts())
	}
	title, ok := tt.Find("config.yaml")
	if !ok {
		t.Fatalf("editor title missing; texts = %q", tt.Texts())
	}
	if row.Y <= title.Y {
		t.Fatalf("file row at %.1f is not below the editor at %.1f", row.Y, title.Y)
	}

	if err := tt.Click("notes.txt"); err != nil {
		t.Fatalf("click file behind the editor: %v", err)
	}
	tt.Frame()
	if b.remoteSel != 1 {
		t.Fatalf("remote selection = %d, want notes.txt at row 1", b.remoteSel)
	}
}

func TestEditorTabCloseKeepsOtherFilesOpen(t *testing.T) {
	a, b, tt := newRemoteDockApp(t)
	addTestEditor(t, a, b, "notes.txt", "notes\n")
	addTestEditor(t, a, b, "config.yaml", "port: 22\n")
	tt.Frame()

	if err := tt.Click("关闭编辑文件 notes.txt"); err != nil {
		t.Fatalf("close editor tab: %v", err)
	}
	tt.Frame()
	if len(a.editorSessions) != 1 {
		t.Fatalf("editor sessions = %d, want 1", len(a.editorSessions))
	}
	if got := a.activeEditor(); got == nil || got.name != "config.yaml" {
		t.Fatalf("active editor after closing a tab = %#v, want config.yaml", got)
	}
	if !tt.HasText("config.yaml") {
		t.Fatalf("remaining file title missing; texts = %q", tt.Texts())
	}
}

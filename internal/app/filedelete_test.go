package app

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/pkg/sftp"
)

// writeTree lays out dir/a.txt, dir/sub/b.txt and dir/sub/deep/c.txt.
func writeTree(t *testing.T, dir string) {
	t.Helper()
	for _, p := range []string{"a.txt", "sub/b.txt", "sub/deep/c.txt", "sub/deep/d.txt"} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestRemoveTreeLocalFolderWithChildren is the regression for "删除文件夹不能
// 删除": a folder that still holds something used to come back as a failure
// instead of being removed.
func TestRemoveTreeLocalFolderWithChildren(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "admin.old-20261111-152628")
	writeTree(t, target)

	var seen int
	removed, err := removeTree(context.Background(), false, nil, target, func(n int, _ string) { seen = n })
	if err != nil {
		t.Fatalf("removeTree: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("folder still there: %v", err)
	}
	// Four files and three folders: the target itself, sub and sub/deep.
	if removed != 7 {
		t.Fatalf("removed %d entries, want 7", removed)
	}
	if seen == 0 {
		t.Fatalf("progress never reported a count")
	}
}

func TestRemoveTreeLocalFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err := removeTree(context.Background(), false, nil, target, nil)
	if err != nil {
		t.Fatalf("removeTree: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed %d entries, want 1", removed)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
}

func TestRemoveTreeStopsWhenCancelled(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "proj")
	writeTree(t, target)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := removeTree(ctx, false, nil, target, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled removeTree error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("a cancelled delete must not remove anything: %v", err)
	}
}

// newTestSFTP pairs a client with an in-process SFTP server rooted at dir, so
// the remote path of a delete runs against a real SFTP protocol.
func newTestSFTP(t *testing.T, dir string) *sftp.Client {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	srv, err := sftp.NewServer(serverConn, sftp.WithServerWorkingDirectory(dir))
	if err != nil {
		t.Fatalf("sftp server: %v", err)
	}
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = serverConn.Close()
	})

	client, err := sftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		t.Fatalf("sftp client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// TestRemoveTreeRemoteFolderWithChildren is the remote half of the same
// regression: SFTP's RMDIR refuses a folder that is not empty, so the delete
// has to walk the tree entry by entry.
func TestRemoveTreeRemoteFolderWithChildren(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, filepath.Join(dir, "admin.old-20261111-152628"))
	client := newTestSFTP(t, dir)

	removed, err := removeTree(context.Background(), true, client, "admin.old-20261111-152628", nil)
	if err != nil {
		t.Fatalf("removeTree: %v", err)
	}
	if removed != 7 {
		t.Fatalf("removed %d entries, want 7", removed)
	}
	if _, err := client.Stat("admin.old-20261111-152628"); !os.IsNotExist(err) {
		t.Fatalf("remote folder still there: %v", err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("temp dir not empty after delete: %v %v", entries, err)
	}
}

func TestRemoveTreeRemoteEmptyFolder(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := newTestSFTP(t, dir)

	if _, err := removeTree(context.Background(), true, client, "empty", nil); err != nil {
		t.Fatalf("removeTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "empty")); !os.IsNotExist(err) {
		t.Fatalf("folder still there: %v", err)
	}
}

func TestRemoveTreeRemoteFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := newTestSFTP(t, dir)

	removed, err := removeTree(context.Background(), true, client, "old.log", nil)
	if err != nil {
		t.Fatalf("removeTree: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed %d entries, want 1", removed)
	}
}

// TestQuickRemoveRemoteFolderFallback is the quick menu path when no SSH
// connection is at hand: the SFTP walk still removes the folder with
// everything inside it, because SFTP's RMDIR alone refuses a non-empty one.
func TestQuickRemoveRemoteFolderFallback(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, filepath.Join(dir, "admin.old-20261111-152628"))
	client := newTestSFTP(t, dir)

	if err := remoteQuickRemove(context.Background(), nil, client, "admin.old-20261111-152628"); err != nil {
		t.Fatalf("remoteQuickRemove folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "admin.old-20261111-152628")); !os.IsNotExist(err) {
		t.Fatalf("folder still there: %v", err)
	}
}

// TestShellQuote pins the quoting that keeps rm -rf pointed at the entry the
// user picked, including a path that itself contains a quote.
func TestShellQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"/var/log/app.log", `'/var/log/app.log'`, true},
		{"/home/o'neil", `'/home/o'\''neil'`, true},
		{"/tmp/a b", `'/tmp/a b'`, true},
		{"", "", false},
		{"/tmp/a\nb", "", false},
		{"/tmp/a\rb", "", false},
		{"/tmp/a\x00b", "", false},
	}
	for _, c := range cases {
		got, ok := shellQuote(c.in)
		if ok != c.ok || got != c.want {
			t.Fatalf("shellQuote(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestDeleteDialogAsksInAppLook pins the delete confirmation to the app's own
// dialog: a native message box cannot warn about a folder's contents, show
// progress or offer a way to stop.
func TestDeleteDialogAsksInAppLook(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "admin.old-20261111-152628")
	writeTree(t, folder)

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
	for i, f := range b.localEntries {
		if f.Name() == "admin.old-20261111-152628" {
			b.localSel = i
		}
	}
	if b.localSel < 0 {
		t.Fatalf("folder not selected; entries = %q", localNames(b))
	}

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()

	a.deleteSelected(b, false)
	tt.Frame()

	for _, want := range []string{
		"删除文件夹", "本机磁盘", "admin.old-20261111-152628",
		"文件夹及其中的全部内容都会被删除。",
		"此操作不可撤销", "取消", "删除",
	} {
		if !tt.HasText(want) {
			t.Fatalf("delete dialog is missing %q; texts = %q", want, tt.Texts())
		}
	}

	// A failure is explained in the dialog, and offers another try.
	a.delErr = `sftp: "Permission denied" (SSH_FX_PERMISSION_DENIED)`
	tt.Frame()
	for _, want := range []string{"删除失败", "sftp: \"Permission denied\" (SSH_FX_PERMISSION_DENIED)", "关闭", "重试"} {
		if !tt.HasText(want) {
			t.Fatalf("failed delete is missing %q; texts = %q", want, tt.Texts())
		}
	}
	a.delErr = ""
	tt.Frame()

	if err := clickText(tt, "取消"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.delOpen {
		t.Fatalf("cancel left the dialog open")
	}
	if tt.HasText("删除文件夹") || tt.HasText("此操作不可撤销") {
		t.Fatalf("the delete dialog is still drawn; texts = %q", tt.Texts())
	}
	// The panel is back in front, still listing the folder it never removed.
	if !tt.HasText("admin.old-20261111-152628/") {
		t.Fatalf("file panel did not come back; texts = %q", tt.Texts())
	}
	if _, err := os.Stat(folder); err != nil {
		t.Fatalf("cancel removed the folder: %v", err)
	}
}

func TestDeleteDialogShowsRemoteTarget(t *testing.T) {
	a, b, tt := newRemoteDockApp(t)
	b.client = newTestSFTP(t, t.TempDir())
	b.remoteEntries = []os.FileInfo{fakeInfo{name: "logs", dir: true}}
	b.remoteSel = 0
	tt.Frame()

	a.deleteSelected(b, true)
	tt.Frame()

	if !tt.HasText("删除文件夹") || !tt.HasText("远端 · SFTP") {
		t.Fatalf("remote delete dialog missing its header; texts = %q", tt.Texts())
	}
	// The dialog names the folder it is about and the directory it is in.
	if !tt.HasText("logs") || !tt.HasText("位于 "+b.remoteDir) {
		t.Fatalf("remote delete dialog missing the target or its folder; texts = %q", tt.Texts())
	}
	a.closeDelete()
	if a.delOpen || a.delTarget != nil {
		t.Fatalf("closeDelete left delete state behind")
	}
}

// localNames lists the local entries, for failure messages.
func localNames(b *fileBrowser) []string {
	names := make([]string, 0, len(b.localEntries))
	for _, f := range b.localEntries {
		names = append(names, f.Name())
	}
	return names
}

func TestDeleteStartWithoutTarget(t *testing.T) {
	a := New()
	a.stop()
	a.startDelete()
	if a.delErr == "" {
		t.Fatalf("delete without a target must report an error")
	}
}

func TestDescribeDeleteError(t *testing.T) {
	target := "/projects/jt808/admin.old-20261111-152628"
	same := &os.PathError{Op: "remove", Path: target, Err: errors.New(`sftp: "Failure" (SSH_FX_FAILURE)`)}
	if got, want := describeDeleteError(same, target), `sftp: "Failure" (SSH_FX_FAILURE)`; got != want {
		t.Fatalf("describeDeleteError(same) = %q, want %q", got, want)
	}

	child := &os.PathError{Op: "remove", Path: target + "/a.txt", Err: errors.New("denied")}
	if got := describeDeleteError(child, target); got != target+"/a.txt: denied" {
		t.Fatalf("describeDeleteError(child) = %q", got)
	}

	if got := describeDeleteError(errors.New("boom"), target); got != "boom" {
		t.Fatalf("describeDeleteError(plain) = %q", got)
	}
}

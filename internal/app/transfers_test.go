package app

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/pkg/sftp"
)

func newInMemorySFTP(t *testing.T) *sftp.Client {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	server := sftp.NewRequestServer(serverConn, sftp.InMemHandler())
	go func() { _ = server.Serve() }()

	client, err := sftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		_ = server.Close()
		_ = serverConn.Close()
		_ = clientConn.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	return client
}

func putRemoteFile(t *testing.T, client *sftp.Client, name, content string) {
	t.Helper()
	file, err := client.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(content)); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func readLocalFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDownloadDirectoryRecursively(t *testing.T) {
	client := newInMemorySFTP(t)
	if err := client.MkdirAll("/data/logs"); err != nil {
		t.Fatal(err)
	}
	putRemoteFile(t, client, "/data/root.txt", "root")
	putRemoteFile(t, client, "/data/logs/app.log", "hello")

	localRoot := filepath.Join(t.TempDir(), "data")
	var finalDone, finalTotal int64
	err := downloadDirectory(context.Background(), client, "/data", localRoot, transferConflictOverwrite,
		func(done, total int64) {
			finalDone, finalTotal = done, total
		})
	if err != nil {
		t.Fatal(err)
	}
	if got := readLocalFile(t, filepath.Join(localRoot, "root.txt")); got != "root" {
		t.Fatalf("root.txt = %q", got)
	}
	if got := readLocalFile(t, filepath.Join(localRoot, "logs", "app.log")); got != "hello" {
		t.Fatalf("app.log = %q", got)
	}
	if finalDone != finalTotal || finalDone != 9 {
		t.Fatalf("progress = %d/%d, want 9/9", finalDone, finalTotal)
	}
}

func TestDownloadDirectoryConflictPolicies(t *testing.T) {
	client := newInMemorySFTP(t)
	if err := client.MkdirAll("/data"); err != nil {
		t.Fatal(err)
	}
	putRemoteFile(t, client, "/data/app.log", "remote")

	t.Run("skip", func(t *testing.T) {
		localRoot := filepath.Join(t.TempDir(), "data")
		if err := os.MkdirAll(localRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		localFile := filepath.Join(localRoot, "app.log")
		if err := os.WriteFile(localFile, []byte("local"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := downloadDirectory(context.Background(), client, "/data", localRoot, transferConflictSkip, nil); err != nil {
			t.Fatal(err)
		}
		if got := readLocalFile(t, localFile); got != "local" {
			t.Fatalf("skip overwrote file: %q", got)
		}
	})

	t.Run("rename", func(t *testing.T) {
		parent := t.TempDir()
		localRoot := filepath.Join(parent, "data")
		if err := os.MkdirAll(localRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(localRoot, "keep.txt"), []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := downloadDirectory(context.Background(), client, "/data", localRoot, transferConflictRename, nil); err != nil {
			t.Fatal(err)
		}
		renamed := filepath.Join(parent, "data (1)")
		if got := readLocalFile(t, filepath.Join(renamed, "app.log")); got != "remote" {
			t.Fatalf("renamed download = %q", got)
		}
		if got := readLocalFile(t, filepath.Join(localRoot, "keep.txt")); got != "keep" {
			t.Fatalf("existing folder changed: %q", got)
		}
	})
}

func TestTransferQueueRespectsConcurrency(t *testing.T) {
	a := New()
	defer a.stop()
	a.settings.TransferConcurrency = 2
	release := make(chan struct{})
	work := func(ctx context.Context, _ func(int64, int64)) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	a.transferTasks = []*transferTask{
		{id: 1, label: "one", status: transferQueued, work: work},
		{id: 2, label: "two", status: transferQueued, work: work},
		{id: 3, label: "three", status: transferQueued, work: work},
	}
	a.pumpTransfers()

	if a.transferTasks[0].status != transferRunning || a.transferTasks[1].status != transferRunning {
		t.Fatalf("first two tasks are not running: %+v", a.transferTasks)
	}
	if a.transferTasks[2].status != transferQueued {
		t.Fatalf("third task = %v, want queued", a.transferTasks[2].status)
	}
	close(release)
}

func TestCancelQueuedTransfer(t *testing.T) {
	a := New()
	defer a.stop()
	a.settings.TransferConcurrency = 1
	block := make(chan struct{})
	work := func(ctx context.Context, _ func(int64, int64)) error {
		select {
		case <-block:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	a.transferTasks = []*transferTask{
		{id: 1, label: "running", status: transferQueued, work: work},
		{id: 2, label: "queued", status: transferQueued, work: work},
	}
	a.pumpTransfers()
	a.cancelTransfer(2)

	if len(a.transferTasks) != 1 || a.transferTasks[0].id != 1 {
		t.Fatalf("queue after cancel = %+v", a.transferTasks)
	}
	if len(a.transferLog) != 1 || a.transferLog[0].Status != transferCanceled {
		t.Fatalf("history after cancel = %+v", a.transferLog)
	}
	close(block)
}

func TestUniqueLocalPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "archive.tar.gz")
	if err := os.WriteFile(target, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	next := uniqueLocalPath(target)
	if next != filepath.Join(dir, "archive.tar (1).gz") {
		t.Fatalf("unique path = %q", next)
	}
	if err := os.WriteFile(next, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	third := uniqueLocalPath(target)
	if third != filepath.Join(dir, "archive.tar (2).gz") {
		t.Fatalf("third path = %q", third)
	}
}

func TestRemotePathExists(t *testing.T) {
	client := newInMemorySFTP(t)
	if remotePathExists(client, "/missing.txt") {
		t.Fatal("missing remote path reported as existing")
	}
	putRemoteFile(t, client, "/exists.txt", "x")
	if !remotePathExists(client, "/exists.txt") {
		t.Fatal("existing remote path reported as missing")
	}
}

func TestDownloadFileResumesPartialFile(t *testing.T) {
	client := newInMemorySFTP(t)
	putRemoteFile(t, client, "/data.bin", "0123456789")

	local := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(local+".part", []byte("0123"), 0o644); err != nil {
		t.Fatal(err)
	}
	var start int64
	if err := downloadFileResume(context.Background(), client, "/data.bin", local, func(done, _ int64) {
		if start == 0 {
			start = done
		}
	}); err != nil {
		t.Fatal(err)
	}
	if start != 4 {
		t.Fatalf("download resumed at %d, want 4", start)
	}
	if got := readLocalFile(t, local); got != "0123456789" {
		t.Fatalf("downloaded content = %q", got)
	}
	if _, err := os.Stat(local + ".part"); !os.IsNotExist(err) {
		t.Fatalf("partial file remained: %v", err)
	}
}

func TestUploadFileResumesPartialFile(t *testing.T) {
	client := newInMemorySFTP(t)
	putRemoteFile(t, client, "/data.bin.xu-part", "0123")

	local := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(local, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	var start int64
	if err := uploadFileResume(context.Background(), client, local, "/data.bin", func(done, _ int64) {
		if start == 0 {
			start = done
		}
	}); err != nil {
		t.Fatal(err)
	}
	if start != 4 {
		t.Fatalf("upload resumed at %d, want 4", start)
	}
	remote, err := client.Open("/data.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	data, err := io.ReadAll(remote)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "0123456789" {
		t.Fatalf("uploaded content = %q", data)
	}
}

func TestTransferPanelShowsQueueAndRetry(t *testing.T) {
	a := New()
	a.terms = []*termSession{{ID: 1, Title: "root@test"}}
	a.syncTabs()
	a.activeTerm = 0
	a.bottomFiles = true
	a.bottomFileTab = int(dockTransfers)
	a.transferTasks = []*transferTask{{
		id: 1, kind: transferUpload, label: "上传 build", target: "/srv/build",
		status: transferRunning, done: 4, total: 10, server: "prod · root@prod:22",
	}, {
		id: 2, kind: transferDownload, label: "下载 app.log", target: "C:\\tmp\\app.log",
		status: transferFailed, err: "network", done: 2, total: 10, server: "db · ops@db:2222",
		work: func(context.Context, func(int64, int64)) error {
			return nil
		},
	}}
	a.transferLog = []transferRecord{{
		Kind: transferDownload, Label: "下载 app.log", Target: "C:\\tmp\\app.log",
		Status: transferFailed, Err: "network", Done: 2, Total: 10, Server: "prod · root@prod:22",
	}}

	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	for _, want := range []string{
		"传输队列", "1 运行 · 0 等待", "1 待继续", "上传 build",
		"prod · root@prod:22", "db · ops@db:2222", "历史记录", "最近 7 天", "重试",
	} {
		if !tt.HasText(want) {
			t.Fatalf("transfer panel missing %q; texts = %q", want, tt.Texts())
		}
	}
	if err := tt.Click("重试"); err != nil {
		t.Fatal(err)
	}
	if got := a.transferTaskByID(2); got == nil || got.status != transferRunning {
		t.Fatalf("retry did not resume the failed task: %+v", got)
	}
}

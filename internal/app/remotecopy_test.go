package app

import (
	"context"
	"io"
	"testing"

	"github.com/pkg/sftp"
)

func readRemoteFile(t *testing.T, client *sftp.Client, name string) string {
	t.Helper()
	file, err := client.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRemoteCopyWithStateCopiesDirectory(t *testing.T) {
	source := newInMemorySFTP(t)
	target := newInMemorySFTP(t)
	if err := source.MkdirAll("/data/logs"); err != nil {
		t.Fatal(err)
	}
	putRemoteFile(t, source, "/data/root.txt", "root")
	putRemoteFile(t, source, "/data/logs/app.log", "hello")

	var done, total int64
	err := copyRemoteWithState(
		context.Background(), source, target,
		"/data", "/backup", transferConflictOverwrite, true,
		func(d, n int64) { done, total = d, n },
		newTransferResumeState(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := readRemoteFile(t, target, "/backup/data/root.txt"); got != "root" {
		t.Fatalf("root.txt = %q", got)
	}
	if got := readRemoteFile(t, target, "/backup/data/logs/app.log"); got != "hello" {
		t.Fatalf("app.log = %q", got)
	}
	if done != 9 || total != 9 {
		t.Fatalf("progress = %d/%d, want 9/9", done, total)
	}
}

func TestRemoteCopyConflictPolicies(t *testing.T) {
	tests := []struct {
		name     string
		conflict transferConflict
		want     string
	}{
		{"overwrite", transferConflictOverwrite, "remote"},
		{"skip", transferConflictSkip, "local"},
		{"rename", transferConflictRename, "remote"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := newInMemorySFTP(t)
			target := newInMemorySFTP(t)
			putRemoteFile(t, source, "/app.txt", "remote")
			putRemoteFile(t, target, "/app.txt", "local")

			if err := copyRemoteWithState(
				context.Background(), source, target,
				"/app.txt", "/", tc.conflict, false, nil, newTransferResumeState(),
			); err != nil {
				t.Fatal(err)
			}
			path := "/app.txt"
			if tc.conflict == transferConflictRename {
				path = "/app (1).txt"
			}
			if got := readRemoteFile(t, target, path); got != tc.want {
				t.Fatalf("%s content = %q, want %q", path, got, tc.want)
			}
		})
	}
}

func TestRemoteCopyRenamesExistingDirectory(t *testing.T) {
	source := newInMemorySFTP(t)
	target := newInMemorySFTP(t)
	if err := source.MkdirAll("/data"); err != nil {
		t.Fatal(err)
	}
	putRemoteFile(t, source, "/data/app.log", "new")
	if err := target.MkdirAll("/backup/data"); err != nil {
		t.Fatal(err)
	}
	putRemoteFile(t, target, "/backup/data/keep.txt", "keep")

	if err := copyRemoteWithState(
		context.Background(), source, target,
		"/data", "/backup", transferConflictRename, true,
		nil, newTransferResumeState(),
	); err != nil {
		t.Fatal(err)
	}
	if got := readRemoteFile(t, target, "/backup/data (1)/app.log"); got != "new" {
		t.Fatalf("renamed directory content = %q", got)
	}
	if got := readRemoteFile(t, target, "/backup/data/keep.txt"); got != "keep" {
		t.Fatalf("existing directory changed: %q", got)
	}
}

func TestRemoteCopySkipMergesDirectoryAndSkipsExistingFiles(t *testing.T) {
	source := newInMemorySFTP(t)
	target := newInMemorySFTP(t)
	if err := source.MkdirAll("/data"); err != nil {
		t.Fatal(err)
	}
	putRemoteFile(t, source, "/data/app.log", "new")
	putRemoteFile(t, source, "/data/new.txt", "added")
	if err := target.MkdirAll("/backup/data"); err != nil {
		t.Fatal(err)
	}
	putRemoteFile(t, target, "/backup/data/app.log", "old")

	if err := copyRemoteWithState(
		context.Background(), source, target,
		"/data", "/backup", transferConflictSkip, true,
		nil, newTransferResumeState(),
	); err != nil {
		t.Fatal(err)
	}
	if got := readRemoteFile(t, target, "/backup/data/app.log"); got != "old" {
		t.Fatalf("skip overwrote an existing file: %q", got)
	}
	if got := readRemoteFile(t, target, "/backup/data/new.txt"); got != "added" {
		t.Fatalf("directory merge missed a new file: %q", got)
	}
}

func TestRemoteCopyFileResumesPartialTarget(t *testing.T) {
	source := newInMemorySFTP(t)
	target := newInMemorySFTP(t)
	putRemoteFile(t, source, "/data.bin", "0123456789")
	putRemoteFile(t, target, "/data.bin.xu-part", "0123")

	var start int64
	err := copyRemoteFileResume(
		context.Background(), source, target, "/data.bin", "/data.bin",
		func(done, _ int64) {
			if start == 0 {
				start = done
			}
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if start != 4 {
		t.Fatalf("remote copy resumed at %d, want 4", start)
	}
	if got := readRemoteFile(t, target, "/data.bin"); got != "0123456789" {
		t.Fatalf("copied content = %q", got)
	}
	if remotePathExists(target, "/data.bin.xu-part") {
		t.Fatal("partial target file remained")
	}
}

func TestRemoteCopyRejectsTargetTypeMismatch(t *testing.T) {
	source := newInMemorySFTP(t)
	target := newInMemorySFTP(t)
	putRemoteFile(t, source, "/data.bin", "file")
	if err := target.MkdirAll("/data.bin"); err != nil {
		t.Fatal(err)
	}
	err := copyRemoteWithState(
		context.Background(), source, target,
		"/data.bin", "/", transferConflictOverwrite, false,
		nil, newTransferResumeState(),
	)
	if err == nil {
		t.Fatal("file was copied over a directory")
	}
}

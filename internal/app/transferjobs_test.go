package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"xu/internal/sshx"
	"xu/internal/store"
)

func newTransferPersistenceApp(t *testing.T) (*App, *store.Store, store.Profile) {
	t.Helper()
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	st, err := store.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	profile := store.Profile{Name: "prod", Host: "prod.example.com", Port: 22, Username: "root", AuthType: "key"}
	if err := st.SaveProfile(context.Background(), &profile); err != nil {
		t.Fatal(err)
	}
	a := New()
	a.SetStore(st)
	return a, st, profile
}

func TestTransferTaskPersistsAndLoadsPaused(t *testing.T) {
	a, st, profile := newTransferPersistenceApp(t)
	task := &transferTask{
		id: 1, kind: transferUpload, label: "上传 build", target: "/srv/build",
		profileID: profile.ID, status: transferPaused, done: 20, total: 100,
		server:    "prod · root@prod.example.com:22",
		createdAt: time.Now(),
		payload: transferPayload{
			Conflict: int(transferConflictRename),
			Upload:   &transferUploadPayload{Paths: []string{"C:\\build"}, RemoteDir: "/srv"},
			Resume: &transferResumeSnapshot{
				Completed: map[string]bool{"/srv/build/a.txt": true},
				Targets:   map[string]string{"upload:/srv/build/a.txt": "/srv/build/a (1).txt"},
			},
		},
	}
	a.transferTasks = append(a.transferTasks, task)
	a.persistTransferTask(task)
	if task.jobID == 0 {
		t.Fatal("task did not get a job ID")
	}

	b := New()
	b.SetStore(st)
	if len(b.transferTasks) != 1 {
		t.Fatalf("loaded tasks = %+v", b.transferTasks)
	}
	loaded := b.transferTasks[0]
	if loaded.status != transferPaused || loaded.profileID != profile.ID ||
		loaded.done != 20 || loaded.total != 100 ||
		loaded.server != task.server ||
		loaded.payload.Upload == nil || loaded.payload.Upload.RemoteDir != "/srv" ||
		loaded.payload.Resume == nil ||
		!loaded.payload.Resume.Completed["/srv/build/a.txt"] {
		t.Fatalf("loaded task = %+v", loaded)
	}
	if b.nextTransferID <= loaded.id {
		t.Fatalf("next transfer ID = %d, loaded ID = %d", b.nextTransferID, loaded.id)
	}
}

func TestTransferTaskIdentityIncludesServer(t *testing.T) {
	a, _, profile := newTransferPersistenceApp(t)
	a.terms = []*termSession{{
		ID: 9, profileID: profile.ID,
		profile: sshx.Profile{Host: profile.Host, Port: profile.Port, Username: profile.Username},
	}}
	profileID, server := a.transferTaskIdentity(9)
	if profileID != profile.ID || server != "prod · root@prod.example.com:22" {
		t.Fatalf("identity = %d %q", profileID, server)
	}

	a.terms = append(a.terms, &termSession{
		ID:      10,
		profile: sshx.Profile{Host: "quick.example.com", Port: 2222, Username: "ops"},
	})
	profileID, server = a.transferTaskIdentity(10)
	if profileID != 0 || server != "ops@quick.example.com:2222" {
		t.Fatalf("unsaved identity = %d %q", profileID, server)
	}
}

func TestRemoteTransferTaskPersistsAndLoads(t *testing.T) {
	a, st, source := newTransferPersistenceApp(t)
	target := store.Profile{Name: "backup", Host: "backup.example.com", Port: 2222, Username: "ops", AuthType: "key"}
	if err := st.SaveProfile(context.Background(), &target); err != nil {
		t.Fatal(err)
	}
	task := &transferTask{
		id: 1, kind: transferRemote, label: "发送文件夹 logs",
		target:          "backup · ops@backup.example.com:2222 · /srv/logs",
		profileID:       source.ID,
		server:          "prod · root@prod.example.com:22",
		targetSessionID: 9,
		status:          transferPaused,
		done:            12,
		total:           40,
		createdAt:       time.Now(),
		payload: transferPayload{
			Conflict: int(transferConflictRename),
			Remote: &transferRemotePayload{
				SourcePath:      "/var/log",
				TargetDir:       "/srv",
				TargetProfileID: target.ID,
				TargetServer:    "backup · ops@backup.example.com:2222",
				Directory:       true,
				TargetSessionID: 9,
			},
		},
	}
	a.transferTasks = append(a.transferTasks, task)
	a.persistTransferTask(task)
	if task.jobID == 0 {
		t.Fatal("remote task did not get a job ID")
	}

	b := New()
	b.SetStore(st)
	if len(b.transferTasks) != 1 {
		t.Fatalf("loaded tasks = %+v", b.transferTasks)
	}
	loaded := b.transferTasks[0]
	if loaded.kind != transferRemote || loaded.profileID != source.ID ||
		loaded.targetSessionID != 0 || loaded.payload.Remote == nil {
		t.Fatalf("loaded remote task = %+v", loaded)
	}
	if loaded.payload.Remote.TargetProfileID != target.ID ||
		loaded.payload.Remote.TargetDir != "/srv" ||
		!loaded.payload.Remote.Directory {
		t.Fatalf("loaded remote payload = %+v", loaded.payload.Remote)
	}
	jobs, err := st.TransferJobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Kind != "remote" || jobs[0].Status != "paused" {
		t.Fatalf("stored job = %+v", jobs)
	}
}

func TestResumeRestoredTransferWaitsForSession(t *testing.T) {
	_, st, profile := newTransferPersistenceApp(t)
	task := &transferTask{
		id: 1, kind: transferDownload, label: "下载 app.log", target: "C:\\tmp\\app.log",
		profileID: profile.ID, status: transferPaused,
		payload: transferPayload{
			Download: &transferDownloadPayload{Remote: "/app.log", Local: "C:\\tmp\\app.log"},
		},
	}
	job := store.TransferJob{
		Kind: "download", Label: task.label, Target: task.target, ProfileID: profile.ID,
		Status: "paused", Payload: `{"download":{"remote":"/app.log","local":"C:\\tmp\\app.log"}}`,
	}
	if err := st.SaveTransferJob(context.Background(), &job); err != nil {
		t.Fatal(err)
	}

	b := New()
	b.SetStore(st)
	if len(b.transferTasks) != 1 {
		t.Fatalf("loaded tasks = %+v", b.transferTasks)
	}
	loaded := b.transferTasks[0]
	b.resumeTransfer(loaded.id)
	if loaded.status != transferPaused || !strings.Contains(loaded.err, "请先连接服务器") {
		t.Fatalf("resume without a session = status:%v err:%q", loaded.status, loaded.err)
	}
	jobs, err := st.TransferJobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || !strings.Contains(jobs[0].Err, "请先连接服务器") {
		t.Fatalf("persisted job = %+v", jobs)
	}
}

func TestRunningTransferJobLoadsAsPaused(t *testing.T) {
	_, st, profile := newTransferPersistenceApp(t)
	job := store.TransferJob{
		Kind: "upload", Label: "上传 build", Target: "/srv", ProfileID: profile.ID,
		Status: "running", Done: 10, Total: 50,
		Payload: `{"upload":{"paths":["C:\\build"],"remote_dir":"/srv"}}`,
	}
	if err := st.SaveTransferJob(context.Background(), &job); err != nil {
		t.Fatal(err)
	}

	a := New()
	a.SetStore(st)
	if len(a.transferTasks) != 1 {
		t.Fatalf("loaded tasks = %+v", a.transferTasks)
	}
	task := a.transferTasks[0]
	if task.status != transferPaused || !strings.Contains(task.err, "应用重启后已暂停") {
		t.Fatalf("loaded running job = %+v", task)
	}
	jobs, err := st.TransferJobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Status != "paused" {
		t.Fatalf("normalised job = %+v", jobs)
	}
}

func TestTransferFinishPersistsPausedAndFailedTasks(t *testing.T) {
	a, st, profile := newTransferPersistenceApp(t)
	ctx := context.Background()

	paused := &transferTask{
		id: 1, kind: transferUpload, label: "上传 build", target: "/srv/build",
		profileID: profile.ID, status: transferRunning, done: 10, total: 20,
		createdAt: time.Now(), cancelRequested: true,
		payload: transferPayload{Conflict: 1, Upload: &transferUploadPayload{Paths: []string{"build"}, RemoteDir: "/srv"}},
	}
	a.transferTasks = append(a.transferTasks, paused)
	a.persistTransferTask(paused)
	a.finishTransferTask(paused.id, context.Canceled)
	if paused.status != transferPaused || !containsTask(a.transferTasks, paused.id) {
		t.Fatalf("paused task = %+v", paused)
	}

	failed := &transferTask{
		id: 2, kind: transferDownload, label: "下载 app.log", target: "/tmp/app.log",
		profileID: profile.ID, status: transferRunning, done: 4, total: 10,
		createdAt: time.Now(),
		payload:   transferPayload{Download: &transferDownloadPayload{Remote: "/app.log", Local: "/tmp/app.log"}},
	}
	a.transferTasks = append(a.transferTasks, failed)
	a.persistTransferTask(failed)
	a.finishTransferTask(failed.id, errors.New("network"))
	if failed.status != transferFailed || failed.err != "network" || !containsTask(a.transferTasks, failed.id) {
		t.Fatalf("failed task = %+v", failed)
	}

	jobs, err := st.TransferJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v", jobs)
	}
	statuses := map[int64]string{jobs[0].ID: jobs[0].Status, jobs[1].ID: jobs[1].Status}
	if statuses[paused.jobID] != "paused" || statuses[failed.jobID] != "failed" {
		t.Fatalf("job statuses = %+v", statuses)
	}
	history, err := st.TransferHistory(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("history = %+v", history)
	}
}

func TestTransferSuccessDeletesJob(t *testing.T) {
	a, st, profile := newTransferPersistenceApp(t)
	task := &transferTask{
		id: 1, kind: transferUpload, label: "上传 build", target: "/srv/build",
		profileID: profile.ID, status: transferRunning, done: 20, total: 20, createdAt: time.Now(),
		payload: transferPayload{Upload: &transferUploadPayload{Paths: []string{"build"}, RemoteDir: "/srv"}},
	}
	a.transferTasks = append(a.transferTasks, task)
	a.persistTransferTask(task)
	jobID := task.jobID
	a.finishTransferTask(task.id, nil)

	if containsTask(a.transferTasks, task.id) {
		t.Fatal("completed task stayed in the queue")
	}
	jobs, err := st.TransferJobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if job.ID == jobID {
			t.Fatalf("completed job remained: %+v", job)
		}
	}
	history, err := st.TransferHistory(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Status != "done" {
		t.Fatalf("history = %+v", history)
	}
}

func TestTransferResumeSnapshotRoundTrip(t *testing.T) {
	state := newTransferResumeState()
	state.target("download-root:/data", func() string { return "C:\\data (1)" })
	state.complete("/data/a.txt")
	snapshot := state.snapshot()

	restored := newTransferResumeStateFrom(snapshot)
	if !restored.done("/data/a.txt") ||
		restored.target("download-root:/data", func() string { return "other" }) != "C:\\data (1)" {
		t.Fatalf("restored resume state = %+v", restored)
	}
}

func TestUploadTaskSkipsPersistedCompletedFile(t *testing.T) {
	client := newInMemorySFTP(t)
	local := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(local, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := newTransferResumeState()
	state.complete("/upload/data.bin")
	var done, total int64
	if err := uploadTaskWithClient(
		context.Background(), client, []string{local}, "/upload",
		transferConflictOverwrite, state,
		func(d, t int64) { done, total = d, t },
	); err != nil {
		t.Fatal(err)
	}
	if done != 10 || total != 10 {
		t.Fatalf("progress = %d/%d, want 10/10", done, total)
	}
	if remotePathExists(client, "/upload/data.bin") {
		t.Fatal("a previously completed file was uploaded again")
	}
}

func containsTask(tasks []*transferTask, id int64) bool {
	for _, task := range tasks {
		if task.id == id {
			return true
		}
	}
	return false
}

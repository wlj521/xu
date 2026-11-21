package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"

	"xu/internal/sshx"
	"xu/internal/store"
)

type transferPayload struct {
	Conflict int                      `json:"conflict"`
	Upload   *transferUploadPayload   `json:"upload,omitempty"`
	Download *transferDownloadPayload `json:"download,omitempty"`
	Remote   *transferRemotePayload   `json:"remote,omitempty"`
	Resume   *transferResumeSnapshot  `json:"resume,omitempty"`
}

type transferUploadPayload struct {
	Paths     []string `json:"paths"`
	RemoteDir string   `json:"remote_dir"`
}

type transferDownloadPayload struct {
	Remote    string `json:"remote"`
	Local     string `json:"local"`
	Directory bool   `json:"directory"`
}

type transferRemotePayload struct {
	SourcePath      string `json:"source_path"`
	TargetDir       string `json:"target_dir"`
	TargetProfileID int64  `json:"target_profile_id,omitempty"`
	TargetServer    string `json:"target_server"`
	Directory       bool   `json:"directory"`
	TargetSessionID int64  `json:"-"`
}

func (a *App) enqueueTransferJob(kind transferKind, label, target string, sessionID int64, payload transferPayload) int64 {
	id := a.nextTransferID
	a.nextTransferID++
	profileID, server := a.transferTaskIdentity(sessionID)
	task := &transferTask{
		id:        id,
		kind:      kind,
		label:     label,
		target:    target,
		sessionID: sessionID,
		profileID: profileID,
		server:    server,
		status:    transferQueued,
		payload:   payload,
		createdAt: time.Now(),
	}
	if payload.Remote != nil {
		task.targetSessionID = payload.Remote.TargetSessionID
	}
	a.transferTasks = append(a.transferTasks, task)
	a.persistTransferTask(task)
	a.fileErr = ""
	a.pumpTransfers()
	return id
}

func (a *App) transferTaskIdentity(sessionID int64) (int64, string) {
	ts := a.termSessionByID(sessionID)
	if ts == nil {
		return 0, ""
	}
	if ts.profileID != 0 {
		return ts.profileID, a.transferServerLabel(ts.profileID)
	}
	return 0, transferServerFromSSHProfile(ts.profile)
}

func (a *App) transferServerLabel(profileID int64) string {
	if profile, ok := a.profileByID(profileID); ok {
		endpoint := fmt.Sprintf("%s@%s:%d", profile.Username, profile.Host, profile.Port)
		name := strings.TrimSpace(profile.Name)
		if name == "" || strings.EqualFold(name, endpoint) {
			return endpoint
		}
		return name + " · " + endpoint
	}
	return fmt.Sprintf("服务器 #%d", profileID)
}

func transferServerFromSSHProfile(profile sshx.Profile) string {
	if strings.TrimSpace(profile.Host) == "" {
		return ""
	}
	return fmt.Sprintf("%s@%s:%d", profile.Username, profile.Host, profile.Port)
}

func (a *App) termSessionByID(id int64) *termSession {
	if id == 0 {
		return nil
	}
	for _, ts := range a.terms {
		if ts.ID == id {
			return ts
		}
	}
	return nil
}

func (a *App) transferSession(task *transferTask) (*termSession, error) {
	if task == nil {
		return nil, errors.New("传输任务不存在")
	}
	if ts := a.termSessionByID(task.sessionID); ts != nil && !ts.local && ts.client != nil && !ts.exited {
		return ts, nil
	}
	if task.profileID != 0 {
		if active := a.activeTerminal(); active != nil && active.profileID == task.profileID &&
			!active.local && active.client != nil && !active.exited {
			return active, nil
		}
		for _, ts := range a.terms {
			if ts.profileID == task.profileID && !ts.local && ts.client != nil && !ts.exited {
				return ts, nil
			}
		}
		name := fmt.Sprintf("#%d", task.profileID)
		if profile, ok := a.profileByID(task.profileID); ok {
			name = profileDisplayName(profile)
		}
		return nil, fmt.Errorf("请先连接服务器 %s 后再继续传输", name)
	}
	return nil, errors.New("原 SSH 会话已关闭；未保存的连接无法在重启后恢复传输")
}

func (a *App) remoteTransferTargetSession(task *transferTask, payload *transferRemotePayload) (*termSession, error) {
	if task == nil || payload == nil {
		return nil, errors.New("目标服务器参数缺失")
	}
	if ts := a.termSessionByID(task.targetSessionID); ts != nil &&
		!ts.local && ts.client != nil && !ts.exited &&
		(payload.TargetProfileID == 0 || ts.profileID == payload.TargetProfileID) {
		return ts, nil
	}
	if payload.TargetProfileID != 0 {
		if active := a.activeTerminal(); active != nil && active.profileID == payload.TargetProfileID &&
			!active.local && active.client != nil && !active.exited {
			return active, nil
		}
		for _, ts := range a.terms {
			if ts.profileID == payload.TargetProfileID && !ts.local && ts.client != nil && !ts.exited {
				return ts, nil
			}
		}
		name := payload.TargetServer
		if name == "" {
			name = fmt.Sprintf("#%d", payload.TargetProfileID)
		}
		return nil, fmt.Errorf("请先连接目标服务器 %s 后再继续传输", name)
	}
	return nil, errors.New("目标服务器会话已关闭；未保存的连接无法在重启后恢复传输")
}

func (a *App) buildTransferWork(task *transferTask) (func(context.Context, func(int64, int64)) error, error) {
	if task == nil {
		return nil, errors.New("传输任务不存在")
	}
	payload := task.payload
	if payload.Upload == nil && payload.Download == nil && payload.Remote == nil {
		return nil, errors.New("传输任务缺少可恢复参数")
	}
	ts, err := a.transferSession(task)
	if err != nil {
		return nil, err
	}
	task.sessionID = ts.ID
	if task.kind == transferRemote {
		target, err := a.remoteTransferTargetSession(task, payload.Remote)
		if err != nil {
			return nil, err
		}
		task.targetSessionID = target.ID
	}
	state := newTransferResumeStateFrom(task.payload.Resume)
	state.onChange = func() {
		snapshot := state.snapshot()
		a.post(func() {
			if a.transferTaskByID(task.id) == nil {
				return
			}
			task.payload.Resume = snapshot
			a.persistTransferTask(task)
		})
	}
	return func(ctx context.Context, report func(int64, int64)) error {
		defer func() {
			snapshot := state.snapshot()
			a.post(func() {
				if a.transferTaskByID(task.id) == nil {
					return
				}
				task.payload.Resume = snapshot
				a.persistTransferTask(task)
			})
		}()
		ts, err := a.transferSession(task)
		if err != nil {
			return err
		}
		client, err := sftp.NewClient(ts.client)
		if err != nil {
			return fmt.Errorf("打开 SFTP 会话: %w", err)
		}
		defer client.Close()

		conflict := transferConflict(payload.Conflict)
		switch task.kind {
		case transferUpload:
			if payload.Upload == nil {
				return errors.New("上传任务缺少源路径")
			}
			return uploadTaskWithClient(ctx, client, payload.Upload.Paths, payload.Upload.RemoteDir, conflict, state, report)
		case transferDownload:
			if payload.Download == nil {
				return errors.New("下载任务缺少目标路径")
			}
			if payload.Download.Directory {
				return downloadDirectoryWithState(
					ctx, client, payload.Download.Remote, payload.Download.Local,
					conflict, report, state,
				)
			}
			return downloadFileResume(ctx, client, payload.Download.Remote, payload.Download.Local, report)
		case transferRemote:
			if payload.Remote == nil {
				return errors.New("远程传输任务缺少目标服务器")
			}
			target, err := a.remoteTransferTargetSession(task, payload.Remote)
			if err != nil {
				return err
			}
			targetClient, err := sftp.NewClient(target.client)
			if err != nil {
				return fmt.Errorf("打开目标服务器 SFTP 会话: %w", err)
			}
			defer targetClient.Close()
			return copyRemoteWithState(
				ctx, client, targetClient,
				payload.Remote.SourcePath, payload.Remote.TargetDir,
				transferConflict(payload.Conflict), payload.Remote.Directory,
				report, state,
			)
		default:
			return errors.New("未知传输类型")
		}
	}, nil
}

func uploadTaskWithClient(ctx context.Context, client *sftp.Client, paths []string, remoteDir string, conflict transferConflict, state *transferResumeState, report func(int64, int64)) error {
	plan, err := buildUploadPlan(paths, remoteDir)
	if err != nil {
		return err
	}
	var done int64
	for _, item := range plan.items {
		if !item.dir && state.done(item.remote) {
			done += item.size
		}
	}
	report(done, plan.total)
	for _, item := range plan.items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if item.dir {
			if err := client.MkdirAll(item.remote); err != nil {
				return fmt.Errorf("创建远端目录 %s: %w", item.remote, err)
			}
			continue
		}
		if state.done(item.remote) {
			continue
		}
		target := state.target("upload:"+item.remote, func() string {
			if !remotePathExists(client, item.remote) {
				return item.remote
			}
			switch conflict {
			case transferConflictSkip:
				return ""
			case transferConflictRename:
				return uniqueRemotePath(client, item.remote)
			default:
				return item.remote
			}
		})
		if target == "" {
			state.complete(item.remote)
			done += item.size
			report(done, plan.total)
			continue
		}
		base := done
		if err := uploadFileResume(ctx, client, item.local, target, func(n, _ int64) {
			report(base+n, plan.total)
		}); err != nil {
			return fmt.Errorf("上传 %s: %w", filepath.Base(item.local), err)
		}
		state.complete(item.remote)
		done += item.size
		report(done, plan.total)
	}
	return nil
}

func (a *App) persistTransferTask(task *transferTask) {
	if a.store == nil || task == nil || task.profileID == 0 {
		return
	}
	payload, err := json.Marshal(task.payload)
	if err != nil {
		a.status = "保存传输任务失败: " + err.Error()
		return
	}
	job := store.TransferJob{
		ID:        task.jobID,
		Kind:      task.kind.text(),
		Label:     task.label,
		Target:    task.target,
		ProfileID: task.profileID,
		Server:    task.server,
		Status:    transferStatusText(task.status),
		Done:      task.done,
		Total:     task.total,
		Err:       task.err,
		Conflict:  task.payload.Conflict,
		Payload:   string(payload),
		CreatedAt: task.createdAt.Unix(),
	}
	if job.CreatedAt <= 0 {
		job.CreatedAt = time.Now().Unix()
	}
	if err := a.store.SaveTransferJob(a.ctx, &job); err != nil {
		a.status = "保存传输任务失败: " + err.Error()
		return
	}
	task.jobID = job.ID
	if task.createdAt.IsZero() {
		task.createdAt = time.Unix(job.CreatedAt, 0)
	}
}

func (a *App) deleteTransferJob(task *transferTask) {
	if a.store == nil || task == nil || task.jobID == 0 {
		return
	}
	if err := a.store.DeleteTransferJob(a.ctx, task.jobID); err != nil {
		a.status = "删除传输任务失败: " + err.Error()
	}
	task.jobID = 0
}

func (a *App) loadTransferJobs() {
	if a.store == nil {
		return
	}
	jobs, err := a.store.TransferJobs(a.ctx)
	if err != nil {
		a.status = "读取传输任务失败: " + err.Error()
		return
	}
	for _, job := range jobs {
		if job.Status == "done" || job.Status == "canceled" {
			_ = a.store.DeleteTransferJob(a.ctx, job.ID)
			continue
		}
		if a.transferTaskByJobID(job.ID) != nil {
			continue
		}
		kind := transferKindFromText(job.Kind)
		var payload transferPayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil ||
			(payload.Upload == nil && payload.Download == nil && payload.Remote == nil) {
			_ = a.store.DeleteTransferJob(a.ctx, job.ID)
			continue
		}
		server := job.Server
		if server == "" && job.ProfileID != 0 {
			server = a.transferServerLabel(job.ProfileID)
		}
		status := transferPaused
		errText := job.Err
		switch transferStatusFromText(job.Status) {
		case transferFailed:
			status = transferFailed
		case transferPaused:
			status = transferPaused
		default:
			status = transferPaused
			if errText == "" {
				errText = "应用重启后已暂停"
			}
		}
		task := &transferTask{
			id:        job.ID,
			jobID:     job.ID,
			kind:      kind,
			label:     job.Label,
			target:    job.Target,
			profileID: job.ProfileID,
			server:    server,
			status:    status,
			done:      job.Done,
			total:     job.Total,
			err:       errText,
			payload:   payload,
			restored:  true,
			createdAt: time.Unix(job.CreatedAt, 0),
		}
		a.transferTasks = append(a.transferTasks, task)
		a.persistTransferTask(task)
		if task.id >= a.nextTransferID {
			a.nextTransferID = task.id + 1
		}
	}
}

func (a *App) transferTaskByJobID(jobID int64) *transferTask {
	for _, task := range a.transferTasks {
		if task.jobID == jobID {
			return task
		}
	}
	return nil
}

func (a *App) resumeTransfer(id int64) {
	task := a.transferTaskByID(id)
	if task == nil || (task.status != transferPaused && task.status != transferFailed) {
		return
	}
	if task.work == nil {
		_, err := a.transferSession(task)
		if err == nil && task.kind == transferRemote {
			_, err = a.remoteTransferTargetSession(task, task.payload.Remote)
		}
		if err != nil {
			task.err = err.Error()
			a.status = err.Error()
			a.persistTransferTask(task)
			return
		}
	}
	task.status = transferQueued
	task.err = ""
	task.cancelRequested = false
	a.persistTransferTask(task)
	a.status = "已重新加入传输队列"
	a.pumpTransfers()
}

func (a *App) removeTransfer(id int64) {
	for i, task := range a.transferTasks {
		if task.id != id {
			continue
		}
		if task.status == transferRunning && task.cancel != nil {
			task.cancelRequested = true
			task.cancel()
			return
		}
		a.transferTasks = append(a.transferTasks[:i], a.transferTasks[i+1:]...)
		a.addTransferRecord(task, transferCanceled)
		a.deleteTransferJob(task)
		a.pumpTransfers()
		return
	}
}

func (a *App) pauseTransfersForShutdown() {
	for _, task := range a.transferTasks {
		if task.status == transferRunning {
			task.status = transferPaused
			task.cancelRequested = true
			if task.err == "" {
				task.err = "应用退出，任务已暂停"
			}
			if task.cancel != nil {
				task.cancel()
			}
		}
		a.persistTransferTask(task)
	}
}

func (a *App) refreshTransferViews(task *transferTask) {
	if task == nil {
		return
	}
	if task.kind == transferRemote {
		target, err := a.remoteTransferTargetSession(task, task.payload.Remote)
		if err == nil {
			if targetBrowser := a.browsers[target.ID]; targetBrowser != nil {
				targetBrowser.loadRemote()
			}
		}
		return
	}
	ts, err := a.transferSession(task)
	if err != nil {
		return
	}
	b := a.browsers[ts.ID]
	if b == nil {
		return
	}
	if task.kind == transferDownload {
		b.loadLocal()
	} else {
		b.loadRemote()
	}
}

func transferStatusLabel(status transferStatus) string {
	switch status {
	case transferQueued:
		return "等待中"
	case transferRunning:
		return "传输中"
	case transferPaused:
		return "已暂停"
	case transferFailed:
		return "失败"
	case transferDone:
		return "完成"
	default:
		return "已取消"
	}
}

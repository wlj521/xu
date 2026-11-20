package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"

	"xu/internal/store"
)

type transferKind int

const (
	transferUpload transferKind = iota
	transferDownload
)

func (k transferKind) text() string {
	if k == transferDownload {
		return "download"
	}
	return "upload"
}

func transferTargetLabel(kind transferKind, target string) string {
	prefix := "目标 "
	if kind == transferDownload {
		prefix = "保存到 "
	}
	return prefix + target
}

type transferStatus int

const (
	transferQueued transferStatus = iota
	transferRunning
	transferDone
	transferFailed
	transferCanceled
)

func transferStatusText(status transferStatus) string {
	switch status {
	case transferQueued:
		return "queued"
	case transferRunning:
		return "running"
	case transferDone:
		return "done"
	case transferCanceled:
		return "canceled"
	default:
		return "failed"
	}
}

func transferStatusFromText(text string) transferStatus {
	switch text {
	case "queued":
		return transferQueued
	case "running":
		return transferRunning
	case "done":
		return transferDone
	case "canceled":
		return transferCanceled
	default:
		return transferFailed
	}
}

type transferTask struct {
	id              int64
	kind            transferKind
	label           string
	target          string
	sessionID       int64
	status          transferStatus
	done            int64
	total           int64
	err             string
	cancel          context.CancelFunc
	cancelRequested bool
	work            func(context.Context, func(int64, int64)) error
	onDone          func()
	retry           func()
}

type transferResumeState struct {
	completed map[string]bool
	targets   map[string]string
}

func newTransferResumeState() *transferResumeState {
	return &transferResumeState{
		completed: make(map[string]bool),
		targets:   make(map[string]string),
	}
}

func (s *transferResumeState) done(key string) bool {
	return s != nil && s.completed[key]
}

func (s *transferResumeState) complete(key string) {
	if s != nil {
		s.completed[key] = true
	}
}

func (s *transferResumeState) target(key string, resolve func() string) string {
	if s == nil {
		return resolve()
	}
	if target, ok := s.targets[key]; ok {
		return target
	}
	target := resolve()
	s.targets[key] = target
	return target
}

type transferRecord struct {
	ID        int64
	Kind      transferKind
	Label     string
	Target    string
	Done      int64
	Total     int64
	Status    transferStatus
	Err       string
	CreatedAt time.Time
	retry     func()
}

type transferConflict int

const (
	transferConflictOverwrite transferConflict = iota
	transferConflictSkip
	transferConflictRename
)

func transferConflictLabel(conflict transferConflict) string {
	switch conflict {
	case transferConflictSkip:
		return "跳过"
	case transferConflictRename:
		return "重命名"
	default:
		return "覆盖"
	}
}

func transferConflictValue(label string) transferConflict {
	switch strings.TrimSpace(label) {
	case "跳过":
		return transferConflictSkip
	case "重命名":
		return transferConflictRename
	default:
		return transferConflictOverwrite
	}
}

func transferConcurrencyOptions() []string {
	return []string{"1 个", "2 个", "3 个", "4 个"}
}

func transferConcurrencyValue(label string) int {
	switch strings.TrimSpace(label) {
	case "1 个":
		return 1
	case "3 个":
		return 3
	case "4 个":
		return 4
	default:
		return 2
	}
}

func transferConcurrencyLabel(limit int) string {
	if limit < 1 || limit > 4 {
		limit = 2
	}
	return fmt.Sprintf("%d 个", limit)
}

func (a *App) transferConflictPolicy() transferConflict {
	switch a.settings.TransferConflict {
	case int(transferConflictSkip):
		return transferConflictSkip
	case int(transferConflictRename):
		return transferConflictRename
	default:
		return transferConflictOverwrite
	}
}

func (a *App) enqueueTransfer(kind transferKind, label, target string, sessionID int64, work func(context.Context, func(int64, int64)) error, onDone func()) int64 {
	return a.enqueueTransferFactory(kind, label, target, sessionID, func() func(context.Context, func(int64, int64)) error {
		return work
	}, onDone)
}

func (a *App) enqueueTransferFactory(kind transferKind, label, target string, sessionID int64, factory func() func(context.Context, func(int64, int64)) error, onDone func()) int64 {
	id := a.nextTransferID
	a.nextTransferID++
	task := &transferTask{
		id:        id,
		kind:      kind,
		label:     label,
		target:    target,
		sessionID: sessionID,
		status:    transferQueued,
		work:      factory(),
		onDone:    onDone,
	}
	task.retry = func() {
		a.enqueueTransferFactory(kind, label, target, sessionID, factory, onDone)
	}
	a.transferTasks = append(a.transferTasks, task)
	a.fileErr = ""
	a.pumpTransfers()
	return id
}

func (a *App) pumpTransfers() {
	limit := a.settings.TransferConcurrency
	if limit < 1 || limit > 4 {
		limit = 2
	}
	running := 0
	for _, task := range a.transferTasks {
		if task.status == transferRunning {
			running++
		}
	}
	for _, task := range a.transferTasks {
		if running >= limit {
			break
		}
		if task.status != transferQueued {
			continue
		}
		a.startTransferTask(task)
		running++
	}
}

func (a *App) startTransferTask(task *transferTask) {
	if task == nil || task.status != transferQueued {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	task.cancel = cancel
	task.status = transferRunning
	id := task.id
	go func() {
		err := task.work(ctx, func(done, total int64) {
			a.post(func() {
				if current := a.transferTaskByID(id); current != nil {
					current.done = done
					current.total = total
				}
			})
		})
		a.post(func() { a.finishTransferTask(id, err) })
	}()
}

func (a *App) finishTransferTask(id int64, err error) {
	index := -1
	for i, task := range a.transferTasks {
		if task.id == id {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	task := a.transferTasks[index]
	a.transferTasks = append(a.transferTasks[:index], a.transferTasks[index+1:]...)
	task.cancel = nil

	status := transferDone
	if err != nil {
		if task.cancelRequested || errors.Is(err, context.Canceled) {
			status = transferCanceled
		} else {
			status = transferFailed
			task.err = err.Error()
		}
	}
	a.addTransferRecord(task, status)

	switch status {
	case transferCanceled:
		a.status = "传输已暂停"
	case transferFailed:
		a.status = "传输失败: " + shortLabel(err.Error(), 70)
	default:
		if task.target != "" {
			a.status = task.label + "完成 · " + shortLabel(task.target, 70)
		} else {
			a.status = task.label + "完成"
		}
		if task.onDone != nil {
			task.onDone()
		}
	}
	a.pumpTransfers()
}

func (a *App) transferTaskByID(id int64) *transferTask {
	for _, task := range a.transferTasks {
		if task.id == id {
			return task
		}
	}
	return nil
}

func (a *App) cancelTransfer(id int64) {
	for i, task := range a.transferTasks {
		if task.id != id {
			continue
		}
		if task.status == transferQueued {
			a.transferTasks = append(a.transferTasks[:i], a.transferTasks[i+1:]...)
			a.addTransferRecord(task, transferCanceled)
			a.pumpTransfers()
			return
		}
		if task.cancel != nil {
			task.cancelRequested = true
			task.cancel()
		}
		return
	}
}

func (a *App) cancelTransfersForSession(sessionID int64) {
	for i := len(a.transferTasks) - 1; i >= 0; i-- {
		task := a.transferTasks[i]
		if task.sessionID != sessionID {
			continue
		}
		if task.status == transferQueued {
			a.transferTasks = append(a.transferTasks[:i], a.transferTasks[i+1:]...)
			a.addTransferRecord(task, transferCanceled)
			continue
		}
		if task.cancel != nil {
			task.cancelRequested = true
			task.cancel()
		}
	}
}

func (a *App) addTransferRecord(task *transferTask, status transferStatus) {
	rec := transferRecord{
		Kind:      task.kind,
		Label:     task.label,
		Target:    task.target,
		Done:      task.done,
		Total:     task.total,
		Status:    status,
		Err:       task.err,
		CreatedAt: time.Now(),
	}
	if status == transferFailed || status == transferCanceled {
		rec.retry = task.retry
	}
	a.transferLog = append([]transferRecord{rec}, a.transferLog...)
	if len(a.transferLog) > 100 {
		a.transferLog = a.transferLog[:100]
	}
	a.persistTransferRecord(rec)
}

func (a *App) persistTransferRecord(rec transferRecord) {
	if a.store == nil {
		return
	}
	stored := store.TransferRecord{
		Kind:      rec.Kind.text(),
		Label:     rec.Label,
		Target:    rec.Target,
		Done:      rec.Done,
		Total:     rec.Total,
		Status:    transferStatusText(rec.Status),
		Err:       rec.Err,
		CreatedAt: rec.CreatedAt.Unix(),
	}
	if err := a.store.AddTransferRecord(a.ctx, &stored); err != nil {
		a.status = "保存传输记录失败: " + err.Error()
	}
}

func (a *App) loadTransferHistory() {
	if a.store == nil {
		return
	}
	_ = a.store.PruneTransferHistory(a.ctx)
	history, err := a.store.TransferHistory(a.ctx, 100)
	if err != nil {
		a.status = "读取传输记录失败: " + err.Error()
		return
	}
	records := make([]transferRecord, 0, len(history))
	for _, rec := range history {
		kind := transferUpload
		if rec.Kind == "download" {
			kind = transferDownload
		}
		records = append(records, transferRecord{
			ID:        rec.ID,
			Kind:      kind,
			Label:     rec.Label,
			Target:    rec.Target,
			Done:      rec.Done,
			Total:     rec.Total,
			Status:    transferStatusFromText(rec.Status),
			Err:       rec.Err,
			CreatedAt: time.Unix(rec.CreatedAt, 0),
		})
	}
	a.transferLog = records
}

func (a *App) clearTransferHistory() {
	a.transferLog = nil
	if a.store != nil {
		_ = a.store.ClearTransferHistory(a.ctx)
	}
}

func (a *App) retryTransfer(index int) {
	if index < 0 || index >= len(a.transferLog) {
		return
	}
	rec := a.transferLog[index]
	if rec.retry == nil {
		a.status = "重启前的传输记录无法重试"
		return
	}
	a.transferLog = append(a.transferLog[:index], a.transferLog[index+1:]...)
	rec.retry()
	a.status = "已重新加入传输队列"
}

func (a *App) transferQueueStats() (active, queued int, done, total int64) {
	for _, task := range a.transferTasks {
		switch task.status {
		case transferRunning:
			active++
			done += task.done
			total += task.total
		case transferQueued:
			queued++
		}
	}
	return
}

func (a *App) hasTransfers() bool {
	return len(a.transferTasks) > 0
}

func remotePathExists(client *sftp.Client, target string) bool {
	if client == nil {
		return false
	}
	if _, err := client.Stat(target); err == nil {
		return true
	} else {
		return !os.IsNotExist(err)
	}
}

func localPathExists(target string) bool {
	_, err := os.Stat(target)
	return err == nil
}

func uniqueRemotePath(client *sftp.Client, target string) string {
	if !remotePathExists(client, target) {
		return target
	}
	ext := path.Ext(target)
	base := strings.TrimSuffix(target, ext)
	for i := 1; i < 10000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if !remotePathExists(client, candidate) {
			return candidate
		}
	}
	return fmt.Sprintf("%s (%d)%s", base, time.Now().Unix(), ext)
}

func uniqueLocalPath(target string) string {
	if !localPathExists(target) {
		return target
	}
	ext := filepath.Ext(target)
	base := strings.TrimSuffix(target, ext)
	for i := 1; i < 10000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if !localPathExists(candidate) {
			return candidate
		}
	}
	return fmt.Sprintf("%s (%d)%s", base, time.Now().Unix(), ext)
}

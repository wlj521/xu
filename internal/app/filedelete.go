package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/pkg/sftp"
)

// deleteTarget is the entry a delete confirmation was opened for: which
// browser shows it, and where it lives.
type deleteTarget struct {
	browser *fileBrowser
	remote  bool
	name    string
	dir     string // the folder the entry sits in
	isDir   bool
}

// path is the absolute location of the entry, in the browser's flavour.
func (dt *deleteTarget) path() string {
	if dt.remote {
		return path.Join(dt.dir, dt.name)
	}
	return filepath.Join(dt.dir, dt.name)
}

// deleteSelected asks before removing the selected entry. The question is the
// app's own dialog rather than a native message box: a folder is deleted with
// everything inside it, which needs a warning, a progress line and a way to
// stop, none of which a message box can show.
func (a *App) deleteSelected(b *fileBrowser, remote bool) {
	var f os.FileInfo
	if remote {
		f = selectedEntry(b.remoteEntries, b.remoteSel)
	} else {
		f = selectedEntry(b.localEntries, b.localSel)
	}
	if f == nil {
		a.fileErr = "请先选择一个文件"
		return
	}
	if remote && b.client == nil {
		a.fileErr = "SSH 连接不可用"
		return
	}

	dir := b.localDir
	if remote {
		dir = b.remoteDir
	}

	// One prompt at a time: a delete question replaces the inline prompt bar.
	a.promptOpen = false
	a.promptAction = nil
	a.delTarget = &deleteTarget{
		browser: b,
		remote:  remote,
		name:    f.Name(),
		dir:     dir,
		isDir:   f.IsDir(),
	}
	a.delErr = ""
	a.delRemoved = 0
	a.delCurrent = ""
	a.delRunning = false
	a.delOpen = true
}

// closeDelete drops the dialog and any delete still running behind it.
func (a *App) closeDelete() {
	if a.delCancel != nil {
		a.delCancel()
		a.delCancel = nil
	}
	// A job that is still unwinding must not paint into the closed dialog.
	a.delID++
	a.delOpen = false
	a.delRunning = false
	a.delTarget = nil
	a.delErr = ""
	a.delRemoved = 0
	a.delCurrent = ""
}

// startDelete removes the confirmed entry on a background goroutine: a folder
// with thousands of children takes one round trip per entry, which would
// freeze the window if it ran while the frame was being built. Progress is
// posted back so the dialog can count what is gone and offer to stop.
func (a *App) startDelete() {
	dt := a.delTarget
	if dt == nil || dt.browser == nil {
		a.delErr = "文件面板已关闭，请重新选择"
		return
	}
	if a.delRunning {
		return
	}
	if dt.remote && dt.browser.client == nil {
		a.delErr = "SSH 连接不可用"
		return
	}

	a.delID++
	id := a.delID
	target, remote := dt.path(), dt.remote
	b, client := dt.browser, dt.browser.client
	ctx, cancel := context.WithCancel(a.ctx)

	a.delCancel = cancel
	a.delRunning = true
	a.delErr = ""
	a.delRemoved = 0
	a.delCurrent = target

	go func() {
		removed, err := removeTree(ctx, remote, client, target, func(n int, current string) {
			a.post(func() {
				if a.delID != id {
					return
				}
				a.delRemoved = n
				a.delCurrent = current
			})
		})
		cancel()

		a.post(func() {
			if a.delID != id {
				return
			}
			a.delCancel = nil
			a.delRunning = false
			if remote {
				b.loadRemote()
			} else {
				b.loadLocal()
			}

			switch {
			case err == nil:
				a.delOpen = false
				a.delTarget = nil
				a.delErr = ""
				a.delRemoved = 0
				a.delCurrent = ""
				a.fileErr = ""
				if removed > 1 {
					a.status = fmt.Sprintf("已删除 %s · %d 项", dt.name, removed)
				} else {
					a.status = "已删除 " + dt.name
				}
			case errors.Is(err, context.Canceled):
				a.delOpen = false
				a.delTarget = nil
				a.delRemoved = 0
				a.delCurrent = ""
				if removed > 0 {
					a.status = fmt.Sprintf("已停止删除 · 已删除 %d 项", removed)
				} else {
					a.status = "已停止删除"
				}
			default:
				a.delRemoved = removed
				a.delErr = describeDeleteError(err, target)
			}
		})
	}()
}

// describeDeleteError keeps the reason a delete failed without repeating the
// path the dialog already shows.
func describeDeleteError(err error, target string) string {
	var pe *os.PathError
	if errors.As(err, &pe) && pe.Path != "" {
		if pe.Path == target {
			return pe.Err.Error()
		}
		return pe.Path + ": " + pe.Err.Error()
	}
	return err.Error()
}

// shortenPath keeps the end of a path, which is the part that says what is
// being deleted: a long one is cut at the front instead of at the back, and
// never runs off the dialog.
func shortenPath(s string, max int) string {
	r := []rune(s)
	if len(r) <= max || max < 2 {
		return s
	}
	return "…" + string(r[len(r)-(max-1):])
}

// shortenMiddle keeps both ends of a path: the root says where it lives and
// the tail names the folder, the two parts a path cut at one end loses. Both
// cuts land on a separator, so neither end breaks a folder name in half.
func shortenMiddle(s string, max int) string {
	r := []rune(s)
	if len(r) <= max || max < 10 {
		return s
	}

	head := max / 3
	for head > 0 && !isPathSep(r[head]) {
		head--
	}
	tail := len(r) - (max - 2 - head)
	if head < 1 || tail <= head+1 || tail >= len(r) {
		return shortenPath(s, max)
	}
	for tail < len(r) && !isPathSep(r[tail]) {
		tail++
	}
	if tail >= len(r) {
		return shortenPath(s, max)
	}
	return string(r[:head+1]) + "…" + string(r[tail:])
}

func isPathSep(r rune) bool { return r == '/' || r == '\\' }

// deleteModal shows the confirmation in the app's own look, over the window.
func (a *App) deleteModal(c *ui.Context, t *ui.Theme) {
	if !a.delOpen {
		return
	}
	dt := a.delTarget
	if dt == nil {
		a.closeDelete()
		return
	}

	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}

	panel := ui.DialogBase(c, &a.delOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(fitDialogWidth(c, 520)).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			a.deleteHeader(c, dt)
			a.deleteBody(c, dt)
			a.deleteFooter(c, t)
		})
	})

	// Escape and a press on the backdrop cancel the question, but never a
	// delete that is already running: that one ends through its stop button.
	if panel.Dismissed() {
		if a.delRunning {
			a.delOpen = true
			return
		}
		a.closeDelete()
	}
}

func (a *App) deleteHeader(c *ui.Context, dt *deleteTarget) {
	title, where := "删除文件", "本机磁盘"
	if dt.isDir {
		title = "删除文件夹"
	}
	if dt.remote {
		where = "远端 · SFTP"
	}

	ui.Row(c).Height(62).Shrink(0).Padding(0, 18).Gap(11).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Box(c).Size(30, 30).Radius(8).
			Background(colRed.Alpha(0.1)).Border(1, colRed.Alpha(0.26)).Center().Children(func() {
			iconEl(c, iconTrash, 15, colRed)
		})
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(14.5).FontWeight(650)
			ui.Text(c, where).FontSize(9.5).TextColor(colMuted).NoWrap()
		})
		if a.delRunning {
			return
		}
		close := ui.ButtonBase(c.Key("delete-close")).Size(26, 26).Radius(6).
			Center().Tooltip("关闭").Label("关闭")
		if close.Hovered() {
			close.Background(colPanel2)
		}
		close.Children(func() {
			iconEl(c, iconClose, 13, colMuted)
		})
		if close.Clicked() {
			a.closeDelete()
		}
	})
}

func (a *App) deleteBody(c *ui.Context, dt *deleteTarget) {
	ui.Column(c).Padding(16, 18).Gap(12).Children(func() {
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, dt.name).FontSize(13).FontWeight(600).
				Font("Cascadia Code").TextColor(colText).NoWrap()
			// The folder the entry sits in, which is what "删除" cannot show
			// on its own.
			ui.Text(c, "位于 "+shortenMiddle(dt.dir, 58)).FontSize(9.5).
				Font("Cascadia Code").TextColor(colMuted2).NoWrap()
		})

		switch {
		case a.delRunning:
			a.deleteProgressLine(c)
		case a.delErr != "":
			a.deleteErrorBox(c)
		case dt.isDir:
			// A file just goes; a folder takes its contents with it, which is
			// the part the prompt has to say out loud.
			a.deleteWarning(c)
		}
	})
}

func (a *App) deleteWarning(c *ui.Context) {
	ui.Box(c).FillWidth().Padding(9, 10).Radius(7).
		Background(colRed.Alpha(0.08)).Border(1, colRed.Alpha(0.22)).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Start).Children(func() {
			iconEl(c, iconAlert, 12, colRed)
			ui.Text(c, "文件夹及其中的全部内容都会被删除。").
				FontSize(10).TextColor(colRed).LineHeight(1.45)
		})
	})
}

func (a *App) deleteProgressLine(c *ui.Context) {
	ui.Box(c).FillWidth().Padding(10, 12).Radius(8).
		Background(colPanel2.Alpha(0.72)).Border(1, colLine).Children(func() {
		ui.Column(c).Gap(5).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "正在删除…").FontSize(10.5).FontWeight(600).TextColor(colText2).NoWrap()
				ui.Text(c, "").Grow(1)
				ui.Text(c, fmt.Sprintf("已删除 %d 项", a.delRemoved)).FontSize(9.5).
					Font("Cascadia Code").TextColor(colMuted).NoWrap()
			})
			ui.Text(c, shortenPath(a.delCurrent, 62)).FontSize(9.2).
				Font("Cascadia Code").TextColor(colMuted2).NoWrap()
		})
	})
}

func (a *App) deleteErrorBox(c *ui.Context) {
	ui.Box(c).FillWidth().Padding(9, 10).Radius(7).
		Background(colRed.Alpha(0.08)).Border(1, colRed.Alpha(0.22)).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				iconEl(c, iconAlert, 12, colRed)
				ui.Text(c, "删除失败").FontSize(10).FontWeight(650).TextColor(colRed)
			})
			ui.Text(c, a.delErr).FontSize(9.3).TextColor(colRed).LineHeight(1.45)
		})
	})
}

func (a *App) deleteFooter(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(56).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		if a.delRunning {
			ui.Text(c, "可随时停止，已删除的内容不会恢复").FontSize(9).TextColor(colMuted2).NoWrap()
			ui.Text(c, "").Grow(1)
			if ghostAction(c, t, "delete-stop", "停止删除", nil).Clicked() && a.delCancel != nil {
				a.delCancel()
			}
			return
		}

		iconEl(c, iconAlert, 11, colMuted2)
		ui.Text(c, a.deleteFooterNote()).FontSize(9).TextColor(colMuted2).NoWrap()
		ui.Text(c, "").Grow(1)

		if a.delErr != "" {
			// A delete that stopped halfway can be picked up again from the
			// entries that are left.
			if ghostAction(c, t, "delete-cancel", "关闭", nil).Clicked() {
				a.closeDelete()
			}
			if dangerPrimaryAction(c, "delete-retry", "重试", iconRefresh).Clicked() {
				a.startDelete()
			}
			return
		}

		if ghostAction(c, t, "delete-cancel", "取消", nil).Clicked() {
			a.closeDelete()
		}
		if dangerPrimaryAction(c, "delete-confirm", "删除", iconTrash).Clicked() {
			a.startDelete()
		}
	})
}

// deleteFooterNote says what the dialog's buttons cannot: once something is
// gone it stays gone, and a delete that failed part way left a count behind.
func (a *App) deleteFooterNote() string {
	if a.delErr != "" && a.delRemoved > 0 {
		return fmt.Sprintf("已删除 %d 项，其余未完成", a.delRemoved)
	}
	return "此操作不可撤销"
}

// dangerPrimaryAction is primaryAction in the destructive color, so the
// button that deletes reads as the dangerous one instead of the safe default.
func dangerPrimaryAction(c *ui.Context, key, label string, icon *ui.SVG) ui.Element {
	onAccent := ui.Hex("#FFFFFF")
	b := ui.ButtonBase(c.Key(key)).Height(32).Padding(0, 14).Gap(7).
		Radius(6).AlignItems(ui.Center)
	b.Background(colRed).Border(1, colRed)
	if b.Hovered() {
		b.Background(colRed.Alpha(0.86))
	}
	b.Children(func() {
		if icon != nil {
			iconEl(c, icon, 14, onAccent)
		}
		ui.Text(c, label).FontSize(fsSmall).FontWeight(650).TextColor(onAccent).NoWrap()
	})
	return b
}

// removeTree deletes target and, when it is a folder, everything inside it.
//
// SFTP's RMDIR refuses a folder that still has children, which is why a plain
// "删除" on any folder that was not empty came back as SSH_FX_FAILURE. The
// tree is walked here instead, so one click deletes a file and a folder alike.
// A symlink is removed as a link: the walk never follows one, so a delete
// cannot reach anything outside the entry the user picked.
func removeTree(ctx context.Context, remote bool, client *sftp.Client, target string, report func(removed int, current string)) (int, error) {
	r := &treeRemover{ctx: ctx, remote: remote, client: client, report: report}
	err := r.remove(target)
	return r.removed, err
}

type treeRemover struct {
	ctx     context.Context
	remote  bool
	client  *sftp.Client // nil when the tree is on this machine's disk
	report  func(removed int, current string)
	removed int
	last    time.Time
}

func (r *treeRemover) remove(target string) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if r.remote {
		return r.removeRemote(target)
	}
	return r.removeLocal(target)
}

func (r *treeRemover) removeRemote(target string) error {
	info, err := r.client.Lstat(target)
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := r.client.ReadDir(target)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := r.remove(path.Join(target, e.Name())); err != nil {
				return err
			}
		}
		if err := r.client.RemoveDirectory(target); err != nil {
			return err
		}
		r.record(target)
		return nil
	}
	if err := r.client.Remove(target); err != nil {
		return err
	}
	r.record(target)
	return nil
}

func (r *treeRemover) removeLocal(target string) error {
	info, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(target)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := r.remove(filepath.Join(target, e.Name())); err != nil {
				return err
			}
		}
	}
	if err := os.Remove(target); err != nil {
		return err
	}
	r.record(target)
	return nil
}

// record counts one removed entry and, at a readable pace, tells the dialog
// which one it was.
func (r *treeRemover) record(target string) {
	r.removed++
	if r.report == nil || time.Since(r.last) < 120*time.Millisecond {
		return
	}
	r.last = time.Now()
	r.report(r.removed, target)
}

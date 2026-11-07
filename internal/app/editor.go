package app

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type editorSession struct {
	termID   int64
	browser  *fileBrowser
	name     string
	remote   string
	local    string
	buffer   *editor.Buffer
	state    *editor.State
	baseSize int64
	baseMod  time.Time
	saving   bool
	err      string
}

const maxEditorSize = 5 << 20 // 5 MB

func (a *App) openRemoteEditor(b *fileBrowser, remote string) {
	if a.editorOpen {
		a.status = "请先保存或关闭当前编辑器"
		return
	}

	st, err := b.client.Stat(remote)
	if err != nil {
		a.fileErr = "读取远端文件失败: " + err.Error()
		return
	}
	if st.Size() > maxEditorSize {
		a.fileErr = "文件超过 5 MB，暂不支持内置编辑"
		return
	}

	dataDir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		a.fileErr = "读取数据目录失败: " + err.Error()
		return
	}
	tmpDir := filepath.Join(dataDir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		a.fileErr = "创建临时目录失败: " + err.Error()
		return
	}

	name := path.Base(remote)
	local := filepath.Join(tmpDir, fmt.Sprintf("edit-%d-%s", time.Now().UnixNano(), safeLocalName(name)))
	if err := downloadFile(a.ctx, b.client, remote, local, nil); err != nil {
		a.fileErr = "下载文件失败: " + err.Error()
		return
	}

	data, err := os.ReadFile(local)
	if err != nil {
		os.Remove(local)
		a.fileErr = "读取临时文件失败: " + err.Error()
		return
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		os.Remove(local)
		a.fileErr = "这看起来是二进制文件，暂不支持内置编辑"
		return
	}

	buf := editor.NewBuffer(string(data))
	if lang := editor.LanguageFor(name); lang != nil {
		buf.SetLanguage(lang)
	}

	a.editor = &editorSession{
		termID:   b.termID,
		browser:  b,
		name:     name,
		remote:   remote,
		local:    local,
		buffer:   buf,
		state:    &editor.State{Buffer: buf},
		baseSize: st.Size(),
		baseMod:  st.ModTime(),
	}
	a.editorOpen = true
	a.fileErr = ""
}

func (a *App) editorView(c *ui.Context, t *ui.Theme) {
	es := a.editor
	if es == nil {
		a.editorOpen = false
		return
	}

	ui.Column(c).Grow(1).Children(func() {
		ui.Row(c).Padding(18, 24, 14, 24).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(40, 40).Radius(11).
				Background(colCyan.Alpha(0.08)).Border(1, colCyan.Alpha(0.22)).Center().Children(func() {
				iconEl(c, iconFile, 18, colCyan)
			})
			title := es.name
			if es.buffer.Modified() {
				title += " *"
			}
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, title).FontSize(17).FontWeight(650).NoWrap()
				ui.Text(c, es.remote).FontSize(10).Font("Cascadia Code").TextColor(colMuted).NoWrap()
			})

			if ghostAction(c, t, "editor-back", "返回文件", iconFolder).Clicked() {
				a.closeEditor()
			}
			if primaryAction(c, "editor-save", "保存", iconSave).Disabled(es.saving).Clicked() {
				a.saveEditor()
			}
		})

		if es.err != "" {
			ui.Box(c).FillWidth().Padding(9, 12).Margin(0, 24, 12, 24).Radius(8).
				Background(colRed.Alpha(0.07)).Border(1, colRed.Alpha(0.18)).Children(func() {
				ui.Text(c, es.err).FontSize(10.5).TextColor(colRed)
			})
		}

		if c.Shortcut(ui.Cmd, ui.KeyS) {
			a.saveEditor()
		}

		editor.View(c, es.state).Grow(1).FillWidth().AutoFocus()
	})
}

func (a *App) saveEditor() {
	es := a.editor
	if es == nil || es.saving {
		return
	}

	if st, err := es.browser.client.Stat(es.remote); err == nil {
		if st.Size() != es.baseSize || !st.ModTime().Equal(es.baseMod) {
			res, _ := mygo.Dialog.Message(mygo.MessageOptions{
				Type:               mygo.MessageWarning,
				Message:            "远端文件已被修改，仍要覆盖？",
				Buttons:            []string{"取消", "覆盖"},
				DestructiveButtons: []int{1},
			})
			if res.Button != 1 {
				return
			}
		}
	}

	f, err := os.Create(es.local)
	if err != nil {
		es.err = "写入临时文件失败: " + err.Error()
		return
	}
	if _, err := es.buffer.WriteTo(f); err != nil {
		f.Close()
		es.err = "写入临时文件失败: " + err.Error()
		return
	}
	if err := f.Close(); err != nil {
		es.err = "写入临时文件失败: " + err.Error()
		return
	}

	es.saving = true
	es.err = ""
	go func() {
		err := uploadFile(a.ctx, es.browser.client, es.local, es.remote, nil)
		a.post(func() {
			es.saving = false
			if err != nil {
				es.err = "保存失败: " + err.Error()
				return
			}

			es.buffer.MarkSaved()
			if st, err := es.browser.client.Stat(es.remote); err == nil {
				es.baseSize = st.Size()
				es.baseMod = st.ModTime()
			}
			es.browser.loadRemote()
			a.status = "已保存 " + es.name
		})
	}()
}

func (a *App) closeEditor() {
	es := a.editor
	if es == nil {
		a.editorOpen = false
		return
	}
	if es.saving {
		a.status = "正在保存，请稍后"
		return
	}

	if es.buffer.Modified() {
		res, _ := mygo.Dialog.Message(mygo.MessageOptions{
			Type:               mygo.MessageWarning,
			Message:            "有未保存的修改，放弃并关闭？",
			Buttons:            []string{"取消", "放弃并关闭"},
			DestructiveButtons: []int{1},
		})
		if res.Button != 1 {
			return
		}
	}

	a.discardEditor()
	a.status = ""
}

func (a *App) discardEditor() {
	if a.editor != nil {
		os.Remove(a.editor.local)
		a.editor = nil
	}
	a.editorOpen = false
}

func (a *App) openSelectedRemoteForEdit(b *fileBrowser) {
	f := selectedEntry(b.remoteEntries, b.remoteSel)
	if f == nil {
		a.fileErr = "请先在右侧选择文件"
		return
	}
	if f.IsDir() {
		a.fileErr = "目录不能编辑"
		return
	}
	a.openRemoteEditor(b, path.Join(b.remoteDir, f.Name()))
}

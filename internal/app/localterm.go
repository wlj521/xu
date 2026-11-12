package app

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
)

func localShell() ([]string, string, error) {
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("pwsh"); err == nil {
			return []string{p}, "PowerShell 7", nil
		}
		if p, err := exec.LookPath("powershell"); err == nil {
			return []string{p}, "Windows PowerShell", nil
		}
		if p, err := exec.LookPath("cmd.exe"); err == nil {
			return []string{p}, "Command Prompt", nil
		}
		return nil, "", fmt.Errorf("未找到 pwsh、powershell 或 cmd.exe")
	}

	return posixShellCommand(os.Getenv("SHELL")), "本地终端", nil
}

func posixShellCommand(shell string) []string {
	if shell == "" {
		shell = "/bin/sh"
	}
	return []string{shell, "-l"}
}

func newLocalTerminal(post func(func()), onSearch func(int64, terminal.SearchStatus), display termDisplay) (*termSession, error) {
	command, title, err := localShell()
	if err != nil {
		return nil, err
	}

	ts := &termSession{
		ID:    time.Now().UnixNano(),
		Title: title,
		local: true,
	}

	term, err := terminal.New(terminal.Options{
		Command:      command,
		Scrollback:   display.Scrollback,
		Font:         display.Font,
		CopyOnSelect: copyOnSelect(display),
		Theme:        display.Light,
		DarkTheme:    display.Dark,
		OnTitle: func(title string) {
			title = strings.TrimSpace(title)
			if title == "" || strings.ContainsAny(title, `\/`) || strings.HasSuffix(strings.ToLower(title), ".exe") {
				return
			}
			post(func() { ts.Title = title })
		},
		OnExit: func(code int) {
			post(func() {
				if !ts.closing {
					ts.exited = true
				}
			})
		},
		OnSearch: func(s terminal.SearchStatus) {
			if onSearch != nil {
				post(func() { onSearch(ts.ID, s) })
			}
		},
	})
	if err != nil {
		return nil, err
	}

	ts.Term = term
	return ts, nil
}

// OpenLocalTerminal activates the running local shell, or opens one.
func (a *App) OpenLocalTerminal() {
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	a.pickerOpen = false

	for i, ts := range a.terms {
		if ts.local && !ts.exited {
			a.activateTerm(i)
			return
		}
	}

	for i := len(a.terms) - 1; i >= 0; i-- {
		ts := a.terms[i]
		if !ts.local {
			continue
		}
		if idx := a.termTabIndex(ts); idx >= 0 {
			a.tabs = append(a.tabs[:idx], a.tabs[idx+1:]...)
			if a.activeTab > idx {
				a.activeTab--
			}
		}
		ts.Close()
		a.terms = append(a.terms[:i], a.terms[i+1:]...)
	}
	if a.activeTab >= len(a.tabs) {
		a.activeTab = len(a.tabs) - 1
	}
	if a.activeTab < 0 {
		a.activeTab = 0
	}
	if a.activeTab >= len(a.tabs) {
		a.activeTerm = -1
	}

	ts, err := newLocalTerminal(a.post, a.onTermSearch, a.termDisplay())
	if err != nil {
		a.status = "打开本地终端失败: " + err.Error()
		a.formOpen = true
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}

	a.terms = append(a.terms, ts)
	a.openTermTab(ts)
	a.formOpen = false
	a.batchOpen = false
	a.settingsOpen = false
	a.filesOpen = false
	a.monitorOpen = false
	a.forwardOpen = false
	a.finding = false
	a.resetFind()
	if a.win != nil {
		a.win.Invalidate()
	}
}

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"

	"xu/internal/sshx"
	"xu/internal/store"
	"xu/internal/vault"
)

type batchChoice struct {
	id       int64
	name     string
	selected bool
}

type batchResult struct {
	profileID int64
	name      string
	status    string
	output    string
	err       string
}

func (a *App) openBatch() {
	if a.editorOpen {
		a.status = "请先保存或关闭编辑器"
		return
	}
	if a.finding {
		a.closeFind()
	}
	if a.activeTerminal() == nil {
		a.OpenLocalTerminal()
	}
	if a.activeTerminal() == nil {
		a.status = "请先打开一个终端"
		return
	}
	a.setDockTab(dockCommands)
	a.batchErr = ""
	a.batchChoices = make([]batchChoice, 0, len(a.profiles))
	for _, p := range a.profiles {
		a.batchChoices = append(a.batchChoices, batchChoice{id: p.ID, name: p.Name, selected: true})
	}
	a.batchState.Selected = &a.batchSel
}

func (a *App) closeBatch() {
	if a.batchRunning {
		a.batchErr = "请先取消当前批量任务"
		return
	}
	a.batchOpen = false
}

func (a *App) batchView(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Padding(24, 28, 20, 28).Gap(14).Children(func() {
		pageHeader(c, t, iconCommand, "批量命令", "在多个已保存会话中执行同一条命令", func() {
			if ghostAction(c, t, "batch-close", "关闭", nil).Clicked() {
				a.closeBatch()
			}
		})

		if len(a.profiles) == 0 {
			cardPanel(c, func() {
				ui.Column(c).FillWidth().Center().Gap(8).Padding(24, 0).Children(func() {
					iconEl(c, iconServer, 28, colMuted2)
					ui.Text(c, "还没有保存的会话").FontSize(fsTitle).FontWeight(600)
					ui.Text(c, "先连接一次并勾选“连接成功后保存会话”").FontSize(10.5).TextColor(colMuted)
				})
			})
			return
		}

		cardPanel(c, func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				sectionLabel(c, "目标会话")
				miniTag(c, fmt.Sprintf("已选 %d / %d", a.selectedBatchCount(), len(a.batchChoices)), colCyan)
				ui.Text(c, "").Grow(1)
				if quietAction(c, t, "batch-select-all", "全选", nil, false).Clicked() {
					for i := range a.batchChoices {
						a.batchChoices[i].selected = true
					}
				}
				if quietAction(c, t, "batch-select-none", "全不选", nil, false).Clicked() {
					for i := range a.batchChoices {
						a.batchChoices[i].selected = false
					}
				}
			})

			ui.Scroll(c).Height(140).Children(func() {
				for i := range a.batchChoices {
					ch := &a.batchChoices[i]
					ui.Checkbox(c.Key(ch.id), &ch.selected, ch.name).FontSize(11.5)
				}
			})
		})

		cardPanel(c, func() {
			ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
				fieldColumn(c, "命令", func() {
					ui.TextInput(c, &a.batchCmd).Label("命令").Placeholder("例如 uptime")
				}).Grow(1)
				execute := accentAction(c, "batch-run", "执行", iconCommand).Height(32)
				if execute.Clicked() && !a.batchRunning {
					a.runBatch()
				}
				if a.batchRunning {
					if ghostAction(c, t, "batch-cancel", "取消", nil).Clicked() {
						a.cancelBatch()
					}
				}
			})

			if a.batchErr != "" {
				ui.Box(c).FillWidth().Padding(9, 11).Radius(8).
					Background(colRed.Alpha(0.07)).Border(1, colRed.Alpha(0.18)).Children(func() {
					ui.Text(c, a.batchErr).FontSize(10.5).TextColor(colRed)
				})
			}
		})

		if len(a.batchResults) == 0 {
			return
		}

		if a.batchSel < 0 || a.batchSel >= len(a.batchResults) {
			a.batchSel = 0
		}

		ui.SplitVertical(c, &a.batchSplit, func() {
			ui.Column(c).Grow(1).Background(colPanel).Border(1, colLine).
				Radius(8).Children(func() {
				cols := []ui.TableColumn{
					{Title: "会话"},
					{Title: "状态", Width: 100},
				}
				ui.Table(c, &a.batchState, cols, len(a.batchResults), func(row, col int) {
					r := a.batchResults[row]
					switch col {
					case 0:
						ui.Text(c, r.name).FontSize(11)
					case 1:
						color := colMuted
						switch r.status {
						case "成功":
							color = colGreen
						case "失败":
							color = colRed
						case "运行中":
							color = colBlue
						}
						ui.Text(c, r.status).FontSize(10.5).TextColor(color)
					}
				}).Grow(1)
			})
		}, func() {
			ui.Scroll(c).Grow(1).Padding(14).Background(colPanel).
				Border(1, colLine).Radius(8).Children(func() {
				ui.Text(c, a.batchOutput()).FontSize(11.5).Font("Cascadia Code").LineHeight(1.55)
			})
		}).Grow(1)
	})
}

func (a *App) selectedBatchCount() int {
	n := 0
	for _, ch := range a.batchChoices {
		if ch.selected {
			n++
		}
	}
	return n
}

func (a *App) batchOutput() string {
	if a.batchSel < 0 || a.batchSel >= len(a.batchResults) {
		return ""
	}
	r := a.batchResults[a.batchSel]
	out := r.output
	if out == "" {
		switch r.status {
		case "等待":
			out = "等待执行…"
		case "运行中":
			out = "正在执行…"
		default:
			out = "（无输出）"
		}
	}
	if r.err != "" {
		out += "\n\n错误: " + r.err
	}
	return out
}

func (a *App) runBatch() {
	cmd := strings.TrimSpace(a.batchCmd)
	if cmd == "" {
		a.batchErr = "请填写要执行的命令"
		return
	}

	selected := make([]batchChoice, 0, len(a.batchChoices))
	for _, ch := range a.batchChoices {
		if ch.selected {
			selected = append(selected, ch)
		}
	}
	if len(selected) == 0 {
		a.batchErr = "请选择至少一个会话"
		return
	}

	res, _ := mygo.Dialog.Message(mygo.MessageOptions{
		Type:    mygo.MessageWarning,
		Message: fmt.Sprintf("将在 %d 台服务器上执行：\n\n%s", len(selected), cmd),
		Buttons: []string{"取消", "执行"},
	})
	if res.Button != 1 {
		return
	}

	byID := make(map[int64]store.Profile, len(a.profiles))
	for _, p := range a.profiles {
		byID[p.ID] = p
	}

	type job struct {
		profile  store.Profile
		password string
	}

	jobs := make([]job, 0, len(selected))
	a.batchResults = make([]batchResult, 0, len(selected))
	for _, ch := range selected {
		p, ok := byID[ch.id]
		if !ok {
			continue
		}
		pw, _ := vault.Get(p.ID, "password")
		jobs = append(jobs, job{profile: p, password: pw})
		a.batchResults = append(a.batchResults, batchResult{
			profileID: p.ID,
			name:      p.Name,
			status:    "等待",
		})
	}
	if len(jobs) == 0 {
		a.batchErr = "选中的会话已不存在"
		return
	}

	a.batchSel = 0
	a.batchState.Selected = &a.batchSel
	a.batchErr = ""
	a.batchRunning = true

	ctx, cancel := context.WithCancel(a.ctx)
	a.batchCancel = cancel

	go func() {
		sem := make(chan struct{}, 4)
		var wg sync.WaitGroup

		for i, j := range jobs {
			i, j := i, j
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				a.post(func() {
					if i < len(a.batchResults) {
						a.batchResults[i].status = "运行中"
					}
				})

				out, err := a.runBatchOne(ctx, j.profile, j.password, cmd)

				a.post(func() {
					if i >= len(a.batchResults) {
						return
					}
					if err != nil {
						a.batchResults[i].status = "失败"
						a.batchResults[i].err = err.Error()
					} else {
						a.batchResults[i].status = "成功"
					}
					a.batchResults[i].output = out
				})
			}()
		}

		wg.Wait()
		a.post(func() {
			a.batchRunning = false
			a.batchCancel = nil
		})
	}()
}

func (a *App) runBatchOne(ctx context.Context, p store.Profile, password, cmd string) (string, error) {
	sp := sshx.Profile{
		Host:     p.Host,
		Port:     p.Port,
		Username: p.Username,
		Password: password,
	}

	client, err := sshx.Dial(ctx, sp, a.trustHost)
	if err != nil {
		var hk *sshx.HostKeyError
		if errors.As(err, &hk) {
			return "", errors.New("主机密钥未信任，请先单独连接一次")
		}
		return "", err
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()

	cmdCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		select {
		case <-cmdCtx.Done():
			_ = sess.Signal(ssh.SIGKILL)
			sess.Close()
		case <-done:
		}
	}()

	out, err := sess.CombinedOutput(cmd)
	close(done)

	text := string(out)
	if len(text) > 64<<10 {
		text = text[:64<<10] + "\n…（输出已截断）"
	}
	return text, err
}

func (a *App) cancelBatch() {
	if a.batchCancel != nil {
		a.batchCancel()
		a.batchErr = "正在取消…"
	}
}

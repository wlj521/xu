package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"xu/internal/sshx"
)

type authPromptField struct {
	Label string
	Value string
	Echo  bool
}

type authPromptResult struct {
	Answers []string
	OK      bool
}

// authPromptAsk serialises passphrase and keyboard-interactive prompts. Only
// one handshake asks the user at a time, even when batch commands or reconnects
// run in parallel.
var authPromptAsk sync.Mutex

func (a *App) attachAuthCallbacks(p sshx.Profile) sshx.Profile {
	p.Passphrase = a.askPrivateKeyPassphrase
	p.Interactive = a.askKeyboardInteractive
	if p.Jump != nil {
		jump := a.attachAuthCallbacks(*p.Jump)
		p.Jump = &jump
	}
	return p
}

func (a *App) askPrivateKeyPassphrase(ctx context.Context, host string) (string, error) {
	answers, err := a.askAuthFields(
		ctx,
		host,
		"解锁私钥",
		"这台连接的私钥已加密。口令只用于本次 SSH 握手，不会写入数据库、日志或命令行。",
		[]string{"私钥口令"},
		[]bool{false},
	)
	if err != nil {
		return "", err
	}
	return answers[0], nil
}

func (a *App) askKeyboardInteractive(
	ctx context.Context,
	host, name, instruction string,
	questions []string,
	echos []bool,
) ([]string, error) {
	if len(questions) == 0 {
		return []string{}, nil
	}
	title := strings.TrimSpace(name)
	if title == "" {
		title = "服务器认证"
	}
	title = shortLabel(title, 70)
	message := strings.TrimSpace(instruction)
	if message == "" {
		message = "服务器要求完成交互式认证。输入内容只用于本次连接。"
	}
	message = shortLabel(message, 500)
	labels := make([]string, len(questions))
	for i, question := range questions {
		question = strings.TrimSpace(question)
		question = strings.TrimSuffix(question, ":")
		question = strings.TrimSuffix(question, "：")
		question = strings.TrimSpace(question)
		if question == "" {
			question = fmt.Sprintf("输入 %d", i+1)
		}
		labels[i] = shortLabel(question, 120)
	}
	if len(echos) != len(questions) {
		echos = make([]bool, len(questions))
	}
	return a.askAuthFields(ctx, host, title, message, labels, echos)
}

func (a *App) askAuthFields(
	ctx context.Context,
	host, title, message string,
	labels []string,
	echos []bool,
) ([]string, error) {
	if a.win == nil || a.ctx.Err() != nil {
		return nil, errors.New("需要交互式认证，但当前窗口不可用")
	}

	authPromptAsk.Lock()
	defer authPromptAsk.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	fields := make([]authPromptField, len(labels))
	for i := range labels {
		fields[i] = authPromptField{
			Label: labels[i],
			Echo:  i >= len(echos) || echos[i],
		}
	}
	ack := make(chan authPromptResult, 1)
	a.post(func() {
		a.authPromptHost = host
		a.authPromptTitle = title
		a.authPromptMessage = message
		a.authPromptFields = fields
		a.authPromptAck = ack
		a.authPromptErr = ""
		a.authPromptOpen = true
	})

	defer a.post(func() {
		if a.authPromptAck != ack {
			return
		}
		a.authPromptAck = nil
		a.authPromptOpen = false
		a.authPromptHost = ""
		a.authPromptTitle = ""
		a.authPromptMessage = ""
		a.authPromptFields = nil
		a.authPromptErr = ""
	})

	select {
	case result := <-ack:
		if !result.OK {
			return nil, errors.New("已取消认证输入")
		}
		return result.Answers, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-a.ctx.Done():
		return nil, a.ctx.Err()
	case <-time.After(2 * time.Minute):
		return nil, errors.New("等待认证输入超时")
	}
}

func (a *App) answerAuthPrompt(ok bool) {
	ack := a.authPromptAck
	if ack == nil {
		return
	}
	if ok {
		answers := make([]string, len(a.authPromptFields))
		for i := range a.authPromptFields {
			if strings.TrimSpace(a.authPromptFields[i].Value) == "" {
				a.authPromptErr = "请填写全部认证信息"
				return
			}
			answers[i] = a.authPromptFields[i].Value
		}
		a.authPromptAck = nil
		a.authPromptOpen = false
		a.authPromptFields = nil
		a.authPromptErr = ""
		select {
		case ack <- authPromptResult{Answers: answers, OK: true}:
		default:
		}
		return
	}

	a.authPromptAck = nil
	a.authPromptOpen = false
	a.authPromptFields = nil
	a.authPromptErr = ""
	select {
	case ack <- authPromptResult{OK: false}:
	default:
	}
}

func (a *App) authPromptModal(c *ui.Context, t *ui.Theme) {
	if !a.authPromptOpen {
		return
	}
	shade := ui.Hex("#000000").Alpha(0.6)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.32)
	}

	panel := ui.DialogBase(c, &a.authPromptOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(fitDialogWidth(c, 520)).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			a.authPromptHeader(c)
			a.authPromptBody(c)
			a.authPromptFooter(c, t)
		})
	})
	if panel.Dismissed() || !a.authPromptOpen {
		a.answerAuthPrompt(false)
	}
}

func (a *App) authPromptHeader(c *ui.Context) {
	ui.Row(c).Height(64).Shrink(0).Padding(0, 18).Gap(11).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Box(c).Size(30, 30).Radius(8).
			Background(colCyan.Alpha(0.1)).Border(1, colCyan.Alpha(0.26)).Center().Children(func() {
			iconEl(c, iconLock, 15, colCyan)
		})
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, a.authPromptTitle).FontSize(14.5).FontWeight(650)
			ui.Text(c, a.authPromptHost).FontSize(9.5).Font("Cascadia Code").TextColor(colMuted).NoWrap()
		})
	})
}

func (a *App) authPromptBody(c *ui.Context) {
	ui.Column(c).Padding(16, 18).Gap(10).Children(func() {
		ui.Text(c, a.authPromptMessage).FontSize(10.2).TextColor(colMuted).LineHeight(1.5)
		submitted := false
		for i := range a.authPromptFields {
			i := i
			field := &a.authPromptFields[i]
			fieldColumn(c, field.Label, func() {
				input := ui.TextInput(c, &field.Value).Label(field.Label).
					Placeholder("请输入").FillWidth()
				if !field.Echo {
					input.Password()
				}
				if i == 0 {
					input.AutoFocus()
				}
				if input.Submitted() {
					submitted = true
				}
			})
		}
		if submitted {
			a.answerAuthPrompt(true)
		}
		if a.authPromptErr != "" {
			ui.Box(c).FillWidth().Padding(8, 10).Radius(7).
				Background(colRed.Alpha(0.08)).Border(1, colRed.Alpha(0.2)).Children(func() {
				ui.Text(c, a.authPromptErr).FontSize(9.5).TextColor(colRed)
			})
		}
	})
}

func (a *App) authPromptFooter(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(56).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		iconEl(c, iconShield, 11, colGreen)
		ui.Text(c, "输入只用于本次认证，不会保存。").FontSize(9).TextColor(colMuted2).NoWrap()
		ui.Text(c, "").Grow(1)
		if ghostAction(c, t, "auth-prompt-cancel", "取消", nil).Clicked() {
			a.answerAuthPrompt(false)
		}
		if primaryAction(c, "auth-prompt-submit", "继续", iconCheck).Clicked() {
			a.answerAuthPrompt(true)
		}
	})
}

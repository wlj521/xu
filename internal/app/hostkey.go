package app

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"xu/internal/sshx"
)

// hostKeyAsk serialises the host key prompt: one dialog answers one dial at a
// time, even when several background connections need confirmation at once.
var hostKeyAsk sync.Mutex

// hostKeyWarnIcon is the shield-alert badge of the prompt: the host key is a
// security decision, not a plain warning.
var hostKeyWarnIcon = svgIcon(`<path d="M12 3 5 6v5.5c0 4.3 3 7.4 7 9.5 4-2.1 7-5.2 7-9.5V6l-7-3Z"/><path d="M12 9v4.2"/><path d="M12 16.6h.01"/>`)

// hostKeyLineWidth is the longest run of a fingerprint one line shows, and
// hostKeyFingerprintWidth what one line holds whole: an SSH SHA256 fingerprint
// runs to about 50 characters, and showing it on one line keeps reading and
// copying it exact. Longer keys are split into even lines instead of being
// clipped.
const (
	hostKeyLineWidth        = 46
	hostKeyFingerprintWidth = 64
)

// wrapFingerprint keeps a fingerprint whole: one that fits stays as it is, and
// a longer one is split into even lines no wider than hostKeyLineWidth, so no
// line ends with a handful of characters on its own.
func wrapFingerprint(fp string) string {
	runes := []rune(fp)
	if len(runes) <= hostKeyFingerprintWidth {
		return fp
	}

	lines := (len(runes) + hostKeyLineWidth - 1) / hostKeyLineWidth
	width := (len(runes) + lines - 1) / lines

	var b strings.Builder
	b.Grow(len(fp) + lines)
	for i, r := range runes {
		if i > 0 && i%width == 0 {
			b.WriteByte('\n')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// hostKeyFieldColor is the surface of the fingerprint block: a lifted field in
// the dark theme, a tinted code block in the light one.
func (a *App) hostKeyFieldColor() ui.Color {
	if a.uiDark() {
		return colPanel2
	}
	return colPanelTop
}

// askHostKey asks the user whether to trust the server's host key. It runs on
// the background dial goroutine: it opens the in-app modal through a.post,
// waits up to two minutes for the answer, and returns false when the window is
// gone, the app closes, the dial is cancelled (its tab was closed), or the
// user cancels.
//
// The dialog itself runs on the main thread, so all host key state lives
// there; the goroutine only waits on the ack channel it created.
func (a *App) askHostKey(ctx context.Context, hk *sshx.HostKeyError) bool {
	if a.win == nil || hk == nil || a.ctx.Err() != nil {
		return false
	}

	hostKeyAsk.Lock()
	defer hostKeyAsk.Unlock()

	// A dial that waited for another prompt may find the app closing.
	if a.ctx.Err() != nil {
		return false
	}

	ack := make(chan bool, 1)
	a.post(func() {
		a.hostKeyHost = hk.Host
		a.hostKeyFingerprint = hk.Fingerprint
		a.hostKeyAck = ack
		a.hostKeyOpen = true
	})

	// Whatever ends the wait, the prompt leaves the dialog closed and the
	// ack channel empty again; a late click then answers nothing.
	defer a.post(func() {
		if a.hostKeyAck != ack {
			return
		}
		a.hostKeyAck = nil
		a.hostKeyOpen = false
		a.hostKeyHost = ""
		a.hostKeyFingerprint = ""
	})

	select {
	case ok := <-ack:
		return ok
	case <-ctx.Done():
		return false
	case <-a.ctx.Done():
		return false
	case <-time.After(2 * time.Minute):
		return false
	}
}

// answerHostKey sends the user's answer to the waiting dial and closes the
// modal. Every way of closing the dialog goes through it, so one prompt is
// answered once and only once: the channel is cleared before the send, which
// never blocks, and a click after the first one is ignored.
func (a *App) answerHostKey(ok bool) {
	ack := a.hostKeyAck
	if ack == nil {
		return
	}
	a.hostKeyAck = nil
	a.hostKeyOpen = false
	select {
	case ack <- ok:
	default:
	}
}

// hostKeyModal shows the host key confirmation in the app's own look, as a
// dialog over the window: it appears when a background dial meets a server
// whose key is not known yet, and answerHostKey closes it.
func (a *App) hostKeyModal(c *ui.Context, t *ui.Theme) {
	if !a.hostKeyOpen {
		return
	}

	shade := ui.Hex("#000000").Alpha(0.58)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}

	fingerprint := wrapFingerprint(a.hostKeyFingerprint)

	panel := ui.DialogBase(c, &a.hostKeyOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(fitDialogWidth(c, 600)).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			a.hostKeyHeader(c)
			a.hostKeyBody(c, fingerprint)
			a.hostKeyFooter(c, t)
		})
	})

	// Escape and a press on the backdrop close the dialog as a cancel does:
	// both answer the waiting dial, once, whichever closed it.
	if panel.Dismissed() || !a.hostKeyOpen {
		a.answerHostKey(false)
	}
}

func (a *App) hostKeyHeader(c *ui.Context) {
	ui.Row(c).Height(64).Shrink(0).Padding(0, 18).Gap(11).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
		ui.Box(c).Size(30, 30).Radius(8).
			Background(colAmber.Alpha(0.12)).Border(1, colAmber.Alpha(0.3)).Center().Children(func() {
			iconEl(c, hostKeyWarnIcon, 15, colAmber)
		})
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, "首次连接这台服务器").FontSize(14.5).FontWeight(650)
			ui.Text(c, "主机密钥校验 · SSH").FontSize(9.5).TextColor(colMuted).NoWrap()
		})
	})
}

func (a *App) hostKeyBody(c *ui.Context, fingerprint string) {
	ui.Column(c).Padding(16, 18).Gap(12).Children(func() {
		fieldColumn(c, "主机", func() {
			ui.Text(c, a.hostKeyHost).FontSize(12.5).FontWeight(600).
				Font("Cascadia Code").TextColor(colText)
		})

		fieldColumn(c, "主机密钥指纹", func() {
			ui.Box(c).FillWidth().Padding(10, 12).Radius(8).
				Background(a.hostKeyFieldColor()).Border(1, colLine).Children(func() {
				ui.Text(c, fingerprint).FontSize(12).Font("Cascadia Code").
					LineHeight(1.6).TextColor(colText2).Selectable()
			})
		})

		ui.Box(c).FillWidth().Padding(9, 10).Radius(7).
			Background(colAmber.Alpha(0.08)).Border(1, colAmber.Alpha(0.22)).Children(func() {
			ui.Row(c).Gap(7).AlignItems(ui.Start).Children(func() {
				iconEl(c, hostKeyWarnIcon, 12, colAmber)
				ui.Text(c, "如果不是你预期的服务器，请取消。").
					FontSize(10).TextColor(colAmber).LineHeight(1.4)
			})
		})

		ui.Row(c).Gap(7).AlignItems(ui.Start).Children(func() {
			iconEl(c, iconShield, 12, colGreen)
			ui.Text(c, "确认后会写入 known_hosts；下次密钥变化将直接阻断连接。").
				FontSize(9.5).TextColor(colMuted).LineHeight(1.45)
		})
	})
}

func (a *App) hostKeyFooter(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(56).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
		iconEl(c, iconShield, 11, colGreen)
		ui.Text(c, "请与服务器管理员核对指纹").FontSize(9).TextColor(colMuted2).NoWrap()
		ui.Text(c, "").Grow(1)
		if ghostAction(c, t, "hostkey-cancel", "取消", nil).Clicked() {
			a.answerHostKey(false)
		}
		if primaryAction(c, "hostkey-trust", "信任并连接", iconShield).Clicked() {
			a.answerHostKey(true)
		}
	})
}

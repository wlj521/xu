package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"xu/internal/sshx"
)

// hostKeyTestFingerprint is a real SSH SHA256 fingerprint, the shape the modal
// has to show whole: 50 characters, "SHA256:" and 43 of base64.
const hostKeyTestFingerprint = "SHA256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU"

func newHostKeyPrompt() (*App, chan bool) {
	a := New()
	ack := make(chan bool, 1)
	a.hostKeyOpen = true
	a.hostKeyHost = "10.0.0.12:22"
	a.hostKeyFingerprint = hostKeyTestFingerprint
	a.hostKeyAck = ack
	return a, ack
}

// hostKeyTester renders the modal alone, as the app's View does over its
// content.
func hostKeyTester(a *App) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		theme := a.applyTheme(c)
		a.hostKeyModal(c, theme)
	}, 1024, 720)
}

func hostKeyAnswer(ack chan bool) (bool, bool) {
	select {
	case ok := <-ack:
		return ok, true
	default:
		return false, false
	}
}

func TestWrapFingerprint(t *testing.T) {
	if got := wrapFingerprint("MD5:aa:bb:cc"); got != "MD5:aa:bb:cc" {
		t.Fatalf("a short fingerprint was changed: %q", got)
	}
	// A SHA256 fingerprint stays on one line, whole: that is what the modal
	// has to show, and copying it copies the fingerprint exactly.
	if got := wrapFingerprint(hostKeyTestFingerprint); got != hostKeyTestFingerprint {
		t.Fatalf("a SHA256 fingerprint was wrapped: %q", got)
	}

	// A longer key is split into even lines, none wider than the block, and
	// none of them shortened.
	long := "SHA256:" + strings.Repeat("Ab3+/", 22)
	lines := strings.Split(wrapFingerprint(long), "\n")
	if len(lines) < 2 {
		t.Fatalf("a %d-character fingerprint was not wrapped: %q", len(long), lines)
	}
	for i, line := range lines {
		if n := len([]rune(line)); n > hostKeyLineWidth {
			t.Fatalf("line %d is %d characters wide: %q", i, n, line)
		}
		if n := len([]rune(line)); n < len([]rune(lines[0]))-1 {
			t.Fatalf("line %d is a short orphan: %q", i, lines)
		}
	}
	if joined := strings.Join(lines, ""); joined != long {
		t.Fatalf("wrapping changed the fingerprint:\n got %q\nwant %q", joined, long)
	}
}

func TestHostKeyModalRendersFingerprintInFull(t *testing.T) {
	a, _ := newHostKeyPrompt()
	tt := hostKeyTester(a)
	tt.Frame()

	for _, want := range []string{
		"首次连接这台服务器",
		"主机密钥校验 · SSH",
		"10.0.0.12:22",
		"主机密钥指纹",
		"如果不是你预期的服务器，请取消。",
		"确认后会写入 known_hosts；下次密钥变化将直接阻断连接。",
		"请与服务器管理员核对指纹",
		"取消",
		"信任并连接",
	} {
		if !tt.HasText(want) {
			t.Fatalf("the modal does not show %q; texts = %q", want, tt.Texts())
		}
	}

	// The whole fingerprint shows: the modal must not shorten it, which a
	// truncated label would.
	if !tt.HasText(hostKeyTestFingerprint) {
		t.Fatalf("the full fingerprint is not shown; texts = %q", tt.Texts())
	}
}

func TestHostKeyModalTrustsOnClick(t *testing.T) {
	a, ack := newHostKeyPrompt()
	tt := hostKeyTester(a)
	tt.Frame()

	if err := tt.Click("信任并连接"); err != nil {
		t.Fatalf("click 信任并连接: %v", err)
	}

	ok, answered := hostKeyAnswer(ack)
	if !answered {
		t.Fatal("clicking 信任并连接 did not answer the dial")
	}
	if !ok {
		t.Fatal("信任并连接 answered false")
	}
	if a.hostKeyOpen {
		t.Fatal("the modal stayed open after trusting")
	}
	if a.hostKeyAck != nil {
		t.Fatal("the ack channel was left behind")
	}

	tt.Frame()
	if tt.HasText("信任并连接") || tt.HasText("首次连接这台服务器") {
		t.Fatalf("the modal was rendered again after trusting; texts = %q", tt.Texts())
	}
}

func TestHostKeyModalCancelsOnClick(t *testing.T) {
	a, ack := newHostKeyPrompt()
	tt := hostKeyTester(a)
	tt.Frame()

	if err := tt.Click("取消"); err != nil {
		t.Fatalf("click 取消: %v", err)
	}

	ok, answered := hostKeyAnswer(ack)
	if !answered {
		t.Fatal("clicking 取消 did not answer the dial")
	}
	if ok {
		t.Fatal("取消 answered true")
	}
	if a.hostKeyOpen {
		t.Fatal("the modal stayed open after cancelling")
	}
}

func TestHostKeyModalBackdropCancels(t *testing.T) {
	a, ack := newHostKeyPrompt()
	tt := hostKeyTester(a)
	tt.Frame()

	// The panel is centered in 1024×720 and 600 wide: (8, 8) is backdrop.
	tt.ClickAt(8, 8)

	ok, answered := hostKeyAnswer(ack)
	if !answered {
		t.Fatal("clicking the backdrop did not answer the dial")
	}
	if ok {
		t.Fatal("the backdrop answered true")
	}
	if a.hostKeyOpen || a.hostKeyAck != nil {
		t.Fatalf("the prompt was left open (open=%v, ack=%v)", a.hostKeyOpen, a.hostKeyAck)
	}
}

func TestHostKeyModalEscapeCancels(t *testing.T) {
	a, ack := newHostKeyPrompt()
	tt := hostKeyTester(a)
	tt.Frame()

	tt.Key(0, ui.KeyEscape)

	ok, answered := hostKeyAnswer(ack)
	if !answered {
		t.Fatal("pressing Escape did not answer the dial")
	}
	if ok {
		t.Fatal("Escape answered true")
	}
	if a.hostKeyOpen {
		t.Fatal("the modal stayed open after Escape")
	}
}

func TestAnswerHostKeyAnswersOnce(t *testing.T) {
	a, ack := newHostKeyPrompt()
	a.answerHostKey(true)
	a.answerHostKey(false)

	ok, answered := hostKeyAnswer(ack)
	if !answered || !ok {
		t.Fatalf("first answer = %v (answered=%v), want true", ok, answered)
	}
	if second, again := hostKeyAnswer(ack); again {
		t.Fatalf("the prompt was answered twice: second answer %v", second)
	}
	if a.hostKeyAck != nil || a.hostKeyOpen {
		t.Fatalf("state after answering: open=%v ack=%v", a.hostKeyOpen, a.hostKeyAck)
	}

	// A prompt that is gone answers nothing, and does not panic.
	a.answerHostKey(false)
	if _, again := hostKeyAnswer(ack); again {
		t.Fatal("an answer was sent after the prompt had gone")
	}
}

func TestAskHostKeyWithoutWindowDeclines(t *testing.T) {
	a := New()
	done := make(chan bool, 1)
	go func() {
		done <- a.askHostKey(context.Background(), &sshx.HostKeyError{
			Host:        "example.com:22",
			Fingerprint: hostKeyTestFingerprint,
		})
	}()

	select {
	case ok := <-done:
		if ok {
			t.Fatal("askHostKey trusted without a window")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("askHostKey kept waiting without a window")
	}

	// No prompt is pending: the modal stays closed and an answer is a no-op.
	tt := hostKeyTester(a)
	tt.Frame()
	if tt.HasText("首次连接这台服务器") {
		t.Fatal("the modal rendered without a prompt")
	}
	a.answerHostKey(true)
}

func TestHostKeyModalOverConnectForm(t *testing.T) {
	a, ack := newHostKeyPrompt()
	// A dial starts from the connect form: that dialog is still open while
	// the server's key is unknown, so the prompt stacks over it.
	a.formOpen = true

	tt := ui.NewTester(a.View, 1280, 800)
	tt.Frame()

	if !tt.HasText("首次连接这台服务器") {
		t.Fatalf("the prompt does not show over the connect form; texts = %q", tt.Texts())
	}
	if !tt.HasText(hostKeyTestFingerprint) {
		t.Fatalf("the prompt does not show the whole fingerprint over the form; texts = %q", tt.Texts())
	}

	if err := tt.Click("信任并连接"); err != nil {
		t.Fatalf("click 信任并连接 over the connect form: %v", err)
	}
	ok, answered := hostKeyAnswer(ack)
	if !answered || !ok {
		t.Fatalf("trusting over the connect form answered %v (answered=%v)", ok, answered)
	}
	if !a.formOpen {
		t.Fatal("answering the prompt closed the connect form")
	}
}

func TestHostKeyModalInAppView(t *testing.T) {
	a, ack := newHostKeyPrompt()

	tt := ui.NewTester(a.View, 1280, 800)
	tt.Frame()

	if !tt.HasText("首次连接这台服务器") {
		t.Fatalf("the app view does not show the prompt; texts = %q", tt.Texts())
	}
	if !tt.HasText(hostKeyTestFingerprint) {
		t.Fatalf("the app view does not show the whole fingerprint; texts = %q", tt.Texts())
	}

	if err := tt.Click("信任并连接"); err != nil {
		t.Fatalf("click 信任并连接 in the app view: %v", err)
	}
	ok, answered := hostKeyAnswer(ack)
	if !answered || !ok {
		t.Fatalf("trusting from the app view answered %v (answered=%v)", ok, answered)
	}

	tt.Frame()
	if tt.HasText("首次连接这台服务器") {
		t.Fatalf("the prompt is still there after trusting; texts = %q", tt.Texts())
	}
}

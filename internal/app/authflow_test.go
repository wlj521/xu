package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"

	"xu/internal/sshx"
	"xu/internal/store"
)

func TestAuthModesIncludeMFA(t *testing.T) {
	for mode, want := range map[int]string{
		authModePassword: sshx.AuthPassword,
		authModeKey:      sshx.AuthKey,
		authModeAgent:    sshx.AuthAgent,
		authModeMFA:      sshx.AuthMFA,
	} {
		if got := authTypeForMode(mode); got != want {
			t.Fatalf("authTypeForMode(%d) = %q, want %q", mode, got, want)
		}
	}
	if got := authModeForType("keyboard-interactive"); got != authModeMFA {
		t.Fatalf("keyboard-interactive mode = %d, want MFA", got)
	}
}

func TestConnectionFormShowsKeyAndMFAControls(t *testing.T) {
	a := New()
	a.formOpen = true
	a.authMode = authModeKey
	a.keyPath = filepath.Join("keys", "deploy_ed25519")

	tt := ui.NewTester(a.View, 1280, 900)
	tt.Frame()
	for _, want := range []string{"私钥文件", "私钥口令", "生成密钥…", "查看公钥", "MFA"} {
		if !tt.HasText(want) {
			t.Fatalf("key auth form does not show %q; texts = %q", want, tt.Texts())
		}
	}
	if !tt.HasText("口令只保留在本次连接的内存中") {
		t.Fatalf("key form does not explain passphrase lifetime; texts = %q", tt.Texts())
	}

	a.authMode = authModeMFA
	tt.Frame()
	if !tt.HasText("keyboard-interactive 提示") {
		t.Fatalf("MFA form hint is missing; texts = %q", tt.Texts())
	}
}

func TestFormProfileCarriesKeySettings(t *testing.T) {
	a := New()
	a.host = "db.example.com"
	a.port = "22"
	a.username = "deploy"
	a.authMode = authModeKey
	a.keyPath = filepath.Join("C:", "keys", "deploy_ed25519")
	a.keyPassphrase = "memory-only"

	p, _, err := a.formProfile()
	if err != nil {
		t.Fatal(err)
	}
	if p.AuthType != sshx.AuthKey || p.KeyPath != a.keyPath || p.KeyPassphrase != "memory-only" {
		t.Fatalf("form profile = %+v", p)
	}
}

func TestSavingKeyProfileDoesNotKeepPassphrase(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	st, err := store.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a := New()
	a.SetStore(st)
	defer a.Close()

	a.host = "db.example.com"
	a.port = "2222"
	a.username = "deploy"
	a.authMode = authModeKey
	a.keyPath = filepath.Join("C:", "keys", "deploy_ed25519")
	a.keyPassphrase = "never-persist-this"
	a.saveProfileForm()
	if a.connectErr != "" {
		t.Fatalf("save profile: %s", a.connectErr)
	}
	if a.keyPassphrase != "" {
		t.Fatal("the form kept the private-key passphrase after saving")
	}

	profiles, err := st.Profiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].KeyPath != a.keyPath || profiles[0].AuthType != sshx.AuthKey {
		t.Fatalf("saved profiles = %+v", profiles)
	}
	dbData, err := os.ReadFile(filepath.Join(dir, "xu.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(dbData, []byte("never-persist-this")) {
		t.Fatal("the private-key passphrase was written to SQLite")
	}
}

func TestSavedKeySettingsReachSSHProfile(t *testing.T) {
	a := New()
	saved := store.Profile{
		ID: 7, Name: "db", Host: "db.example.com", Port: 2222, Username: "deploy",
		AuthType: sshx.AuthKey, KeyPath: filepath.Join("C:", "keys", "deploy_key"),
	}
	a.applyProfile(saved)
	if a.authMode != authModeKey || a.keyPath != saved.KeyPath || a.keyPassphrase != "" {
		t.Fatalf("form did not load key settings: auth=%d path=%q passphrase=%q", a.authMode, a.keyPath, a.keyPassphrase)
	}
	p, err := a.sshProfileForSaved(saved)
	if err != nil {
		t.Fatal(err)
	}
	if p.AuthType != sshx.AuthKey || p.KeyPath != saved.KeyPath {
		t.Fatalf("SSH profile = %+v", p)
	}
}

func TestAuthPromptCollectsMFAAnswers(t *testing.T) {
	a := New()
	ack := make(chan authPromptResult, 1)
	a.authPromptOpen = true
	a.authPromptHost = "db.example.com:22"
	a.authPromptTitle = "Two-factor authentication"
	a.authPromptMessage = "Enter your password and verification code"
	a.authPromptFields = []authPromptField{
		{Label: "Password", Echo: false, Value: "secret"},
		{Label: "Verification code", Echo: false, Value: "123456"},
	}
	a.authPromptAck = ack

	tt := ui.NewTester(func(c *ui.Context) {
		theme := a.applyTheme(c)
		a.authPromptModal(c, theme)
	}, 900, 700)
	tt.Frame()
	for _, want := range []string{"Two-factor authentication", "Password", "Verification code", "取消", "继续"} {
		if !tt.HasText(want) {
			t.Fatalf("auth prompt does not show %q; texts = %q", want, tt.Texts())
		}
	}
	if err := tt.Click("继续"); err != nil {
		t.Fatalf("submit auth prompt: %v", err)
	}

	select {
	case result := <-ack:
		if !result.OK || len(result.Answers) != 2 ||
			result.Answers[0] != "secret" || result.Answers[1] != "123456" {
			t.Fatalf("auth prompt result = %+v", result)
		}
	default:
		t.Fatal("submitting the auth prompt returned no result")
	}
	if a.authPromptOpen || a.authPromptAck != nil {
		t.Fatal("the auth prompt stayed open after submitting")
	}

	tt.Frame()
	if tt.HasText("Two-factor authentication") {
		t.Fatal("the auth prompt was rendered after submitting")
	}
}

func TestAuthPromptCancelAndEmptyInput(t *testing.T) {
	a := New()
	ack := make(chan authPromptResult, 1)
	a.authPromptOpen = true
	a.authPromptTitle = "Unlock key"
	a.authPromptHost = "key"
	a.authPromptFields = []authPromptField{{Label: "Passphrase", Echo: false}}
	a.authPromptAck = ack

	tt := ui.NewTester(func(c *ui.Context) {
		theme := a.applyTheme(c)
		a.authPromptModal(c, theme)
	}, 900, 700)
	tt.Frame()
	if err := tt.Click("继续"); err != nil {
		t.Fatalf("submit empty auth prompt: %v", err)
	}
	if a.authPromptErr == "" || !a.authPromptOpen {
		t.Fatalf("empty auth input did not stay open with an error: open=%v err=%q", a.authPromptOpen, a.authPromptErr)
	}

	if err := tt.Click("取消"); err != nil {
		t.Fatalf("cancel auth prompt: %v", err)
	}
	select {
	case result := <-ack:
		if result.OK {
			t.Fatal("canceling the auth prompt returned OK")
		}
	default:
		t.Fatal("canceling the auth prompt returned no result")
	}
}

func TestGeneratedKeyPairAndPublicKeyCopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_xu_test")
	public, fingerprint, err := generateKeyPair(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("private key missing: %v", err)
	}
	if _, err := os.Stat(path + ".pub"); err != nil {
		t.Fatalf("public key missing: %v", err)
	}
	signer, err := sshx.LoadSigner(path, "")
	if err != nil {
		t.Fatal(err)
	}
	wantPublic := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
	if public+"\n" != wantPublic || fingerprint != ssh.FingerprintSHA256(signer.PublicKey()) {
		t.Fatalf("generated public key = %q %q", public, fingerprint)
	}

	a := New()
	a.openKeyInfo(path, public, fingerprint)
	tt := ui.NewTester(func(c *ui.Context) {
		theme := a.applyTheme(c)
		a.keyInfoModal(c, theme)
	}, 900, 700)
	tt.Frame()
	for _, want := range []string{"SSH 公钥", fingerprint, "复制公钥"} {
		if !tt.HasText(want) {
			t.Fatalf("key info does not show %q; texts = %q", want, tt.Texts())
		}
	}
	if err := tt.Click("复制公钥"); err != nil {
		t.Fatalf("copy public key: %v", err)
	}
	if got := tt.Clipboard(); got != public+"\n" {
		t.Fatalf("clipboard = %q", got)
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatalf("close key info: %v", err)
	}
	if a.keyInfoOpen {
		t.Fatal("key info stayed open")
	}
}

func TestGenerateEncryptedKeyPair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_xu_encrypted")
	if _, _, err := generateKeyPair(path, "generated-passphrase"); err != nil {
		t.Fatal(err)
	}
	if _, err := sshx.LoadSigner(path, ""); err != sshx.ErrPrivateKeyNeedsPassphrase {
		t.Fatalf("encrypted generated key unlock error = %v", err)
	}
	if _, err := sshx.LoadSigner(path, "generated-passphrase"); err != nil {
		t.Fatalf("generated key did not unlock: %v", err)
	}
}

func TestAttachAuthCallbacksReachesJumpChain(t *testing.T) {
	a := New()
	p := a.attachAuthCallbacks(sshx.Profile{
		AuthType: sshx.AuthMFA,
		KeyPath:  "key",
		Jump:     &sshx.Profile{AuthType: sshx.AuthKey, KeyPath: "jump-key"},
	})
	if p.Passphrase == nil || p.Interactive == nil || p.Jump == nil ||
		p.Jump.Passphrase == nil || p.Jump.Interactive == nil {
		t.Fatalf("callbacks were not attached through the chain: %+v", p)
	}
}

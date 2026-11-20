package app

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"

	"xu/internal/sshx"
)

var openSSHKeyFile = func(opts mygo.OpenDialogOptions) ([]string, error) {
	return mygo.Dialog.Open(opts)
}

var saveSSHKeyFile = func(opts mygo.SaveDialogOptions) (string, error) {
	return mygo.Dialog.Save(opts)
}

func (a *App) choosePrivateKey() {
	paths, err := openSSHKeyFile(mygo.OpenDialogOptions{
		Parent:      a.win,
		Title:       "选择 SSH 私钥",
		ButtonLabel: "选择",
		Filters: []mygo.FileFilter{
			{Name: "SSH 私钥", Extensions: []string{"*"}},
		},
	})
	if err != nil {
		a.connectErr = "打开私钥选择器失败: " + err.Error()
		return
	}
	if len(paths) == 0 {
		return
	}
	a.keyPath = paths[0]
	a.keyPassphrase = ""
	a.connectErr = ""
}

func (a *App) generatePrivateKey() {
	path, err := saveSSHKeyFile(mygo.SaveDialogOptions{
		Parent:      a.win,
		Title:       "生成 Ed25519 SSH 密钥",
		DefaultPath: defaultGeneratedKeyPath(),
		ButtonLabel: "生成",
		Filters: []mygo.FileFilter{
			{Name: "SSH 私钥", Extensions: []string{"*"}},
		},
	})
	if err != nil {
		a.connectErr = "打开密钥保存位置失败: " + err.Error()
		return
	}
	if strings.TrimSpace(path) == "" {
		return
	}
	path, err = filepath.Abs(path)
	if err != nil {
		a.connectErr = "密钥路径无效: " + err.Error()
		return
	}
	if _, err := os.Stat(path); err == nil {
		a.connectErr = "目标文件已存在，请选择新的文件名"
		return
	}

	public, fingerprint, err := generateKeyPair(path, a.keyPassphrase)
	if err != nil {
		a.connectErr = "生成密钥失败: " + err.Error()
		return
	}
	a.keyPath = path
	a.keyPassphrase = ""
	a.connectErr = ""
	a.openKeyInfo(path, public, fingerprint)
}

func generateKeyPair(path, passphrase string) (public, fingerprint string, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", "", err
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(private, "xu generated key")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(private, "xu generated key", []byte(passphrase))
	}
	if err != nil {
		return "", "", err
	}
	privateData := pem.EncodeToMemory(block)
	if err := os.WriteFile(path, privateData, 0o600); err != nil {
		return "", "", err
	}
	_ = os.Chmod(path, 0o600)

	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		_ = os.Remove(path)
		return "", "", err
	}
	publicKey := signer.PublicKey()
	public = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey)))
	fingerprint = ssh.FingerprintSHA256(publicKey)
	if err := os.WriteFile(path+".pub", []byte(public+"\n"), 0o644); err != nil {
		_ = os.Remove(path)
		return "", "", err
	}
	return public, fingerprint, nil
}

func defaultGeneratedKeyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "id_xu_ed25519"
	}
	dir := filepath.Join(home, ".ssh")
	if _, err := os.Stat(dir); err != nil {
		dir = home
	}
	return filepath.Join(dir, "id_xu_ed25519")
}

func (a *App) showKeyInfo() {
	path := strings.TrimSpace(a.keyPath)
	if path == "" {
		a.connectErr = "请先选择或生成私钥文件"
		return
	}
	passphrase := a.keyPassphrase
	host := strings.TrimSpace(a.host)
	if host == "" {
		host = filepath.Base(path)
	}

	go func() {
		public, fingerprint, err := publicKeyForFile(path, passphrase)
		if errors.Is(err, sshx.ErrPrivateKeyNeedsPassphrase) {
			answers, promptErr := a.askAuthFields(
				a.ctx,
				host,
				"解锁私钥",
				"查看公钥需要先解锁这个加密私钥。口令只用于本次读取，不会保存。",
				[]string{"私钥口令"},
				[]bool{false},
			)
			if promptErr != nil {
				err = promptErr
			} else {
				public, fingerprint, err = publicKeyForFile(path, answers[0])
			}
		}
		a.post(func() {
			if err != nil {
				a.connectErr = "读取公钥失败: " + err.Error()
				return
			}
			a.openKeyInfo(path, public, fingerprint)
		})
	}()
}

func publicKeyForFile(path, passphrase string) (public, fingerprint string, err error) {
	path = expandKeyPath(path)
	signer, err := sshx.LoadSigner(path, passphrase)
	if err == nil {
		key := signer.PublicKey()
		return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), ssh.FingerprintSHA256(key), nil
	}
	if !errors.Is(err, sshx.ErrPrivateKeyNeedsPassphrase) {
		return "", "", err
	}

	sidecar, readErr := os.ReadFile(path + ".pub")
	if readErr != nil {
		return "", "", err
	}
	key, _, _, _, parseErr := ssh.ParseAuthorizedKey(sidecar)
	if parseErr != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), ssh.FingerprintSHA256(key), nil
}

func expandKeyPath(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

func (a *App) openKeyInfo(path, public, fingerprint string) {
	a.keyInfoPath = path
	a.keyInfoPublic = public
	a.keyInfoFingerprint = fingerprint
	a.keyInfoOpen = true
}

func (a *App) keyInfoModal(c *ui.Context, t *ui.Theme) {
	if !a.keyInfoOpen {
		return
	}
	shade := ui.Hex("#000000").Alpha(0.55)
	if !a.uiDark() {
		shade = ui.Hex("#0f172a").Alpha(0.3)
	}
	ui.DialogBase(c, &a.keyInfoOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(shade)
		panel.Width(fitDialogWidth(c, 600)).Padding(0).Background(colPanel).
			Border(1, colLine2).Radius(14).
			Shadow(0, 26, 70, 0, ui.Hex("#000000").Alpha(0.42)).Children(func() {
			ui.Row(c).Height(64).Shrink(0).Padding(0, 18).Gap(11).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(colLine).Children(func() {
				ui.Box(c).Size(30, 30).Radius(8).
					Background(colCyan.Alpha(0.1)).Border(1, colCyan.Alpha(0.26)).Center().Children(func() {
					iconEl(c, iconLock, 15, colCyan)
				})
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, "SSH 公钥").FontSize(14.5).FontWeight(650)
					ui.Text(c, shortenMiddle(a.keyInfoPath, 62)).FontSize(9.5).
						Font("Cascadia Code").TextColor(colMuted).NoWrap()
				})
				if iconButton(c, t, "key-info-close", iconClose, "关闭").Clicked() {
					a.keyInfoOpen = false
				}
			})

			ui.Column(c).Padding(16, 18).Gap(10).Children(func() {
				fieldColumn(c, "指纹", func() {
					ui.Box(c).FillWidth().Padding(10, 11).Radius(8).
						Background(a.hostKeyFieldColor()).Border(1, colLine).Children(func() {
						ui.Text(c, a.keyInfoFingerprint).FontSize(10.5).Font("Cascadia Code").
							TextColor(colText2).Selectable()
					})
				})
				fieldColumn(c, "公钥", func() {
					ui.Box(c).FillWidth().Padding(10, 11).Radius(8).
						Background(a.hostKeyFieldColor()).Border(1, colLine).Children(func() {
						ui.Text(c, a.keyInfoPublic).FontSize(9.5).Font("Cascadia Code").
							LineHeight(1.55).TextColor(colText2).Selectable()
					})
				})
				ui.Text(c, "把公钥添加到服务器的 ~/.ssh/authorized_keys 后即可使用这把私钥登录。").
					FontSize(9.5).TextColor(colMuted).LineHeight(1.45)
			})

			ui.Row(c).Height(56).Shrink(0).Padding(0, 18).Gap(8).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(colLine).Children(func() {
				iconEl(c, iconShield, 11, colGreen)
				ui.Text(c, "复制公钥不会暴露私钥。").FontSize(9).TextColor(colMuted2).NoWrap()
				ui.Text(c, "").Grow(1)
				if ghostAction(c, t, "key-info-copy", "复制公钥", iconCopy).Clicked() {
					c.WriteClipboard(a.keyInfoPublic + "\n")
					a.status = "公钥已复制"
				}
				if primaryAction(c, "key-info-done", "完成", iconCheck).Clicked() {
					a.keyInfoOpen = false
				}
			})
		})
	})
}

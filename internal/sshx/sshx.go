package sshx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const (
	AuthPassword            = "password"
	AuthKey                 = "key"
	AuthAgent               = "agent"
	AuthMFA                 = "mfa"
	authKeyboardInteractive = "keyboard-interactive"
)

var (
	ErrPrivateKeyNeedsPassphrase = errors.New("私钥已加密，需要输入口令")
	ErrPrivateKeyPassphrase      = errors.New("私钥口令不正确")
)

type PassphrasePrompt func(ctx context.Context, host string) (string, error)
type KeyboardInteractivePrompt func(ctx context.Context, host, name, instruction string, questions []string, echos []bool) ([]string, error)

type Profile struct {
	Host     string
	Port     int
	Username string
	Password string
	// AuthType selects the authentication method. An empty value keeps the
	// historical behaviour: try the password, then local key files.
	AuthType string
	// KeyPath chooses one private key file. Empty keeps the legacy
	// ~/.xu/keys lookup. KeyPassphrase lives only in memory and is never
	// stored in SQLite.
	KeyPath       string
	KeyPassphrase string
	Passphrase    PassphrasePrompt
	Interactive   KeyboardInteractivePrompt
	// Jump is the SSH host used to reach this profile. It may itself have a
	// Jump, forming a chain.
	Jump *Profile
}

// HostKeyError is returned from the handshake when the host key is not
// trusted yet. The UI asks the user, then retries.
type HostKeyError struct {
	Host        string
	Fingerprint string
	Key         ssh.PublicKey
}

func (e *HostKeyError) Error() string {
	return fmt.Sprintf("未知主机密钥 %s（%s）", e.Host, e.Fingerprint)
}

// TrustFunc reports whether host's key with this fingerprint is trusted.
type TrustFunc func(host, fingerprint string) bool

// Phase tells how far a dial has got, so a UI can show real progress instead
// of one generic "connecting" message for the whole wait.
type Phase int

const (
	PhaseJump        Phase = iota // connecting through a jump host
	PhaseTCP                      // dialing the TCP socket
	PhaseKeyExchange              // exchanging versions and keys
	PhaseHostKey                  // waiting for the user to trust a new host key
	PhaseAuth                     // verifying the account
	PhaseSession                  // opening the terminal session
)

// PhaseFunc receives each phase as it starts. It is called from the dial
// goroutine, so the UI must marshal it to its own thread.
type PhaseFunc func(Phase)

func Dial(ctx context.Context, p Profile, trust TrustFunc) (*ssh.Client, error) {
	return DialWithProgress(ctx, p, trust, nil)
}

const maxJumpDepth = 8
const interactiveHandshakeDeadline = 2*time.Minute + 30*time.Second

// DialWithProgress is Dial with a callback that reports every phase, which
// the connecting tab uses to say what it is actually doing.
func DialWithProgress(ctx context.Context, p Profile, trust TrustFunc, progress PhaseFunc) (*ssh.Client, error) {
	return dialWithProgress(ctx, p, trust, progress, 0)
}

func dialWithProgress(ctx context.Context, p Profile, trust TrustFunc, progress PhaseFunc, depth int) (*ssh.Client, error) {
	if depth > maxJumpDepth {
		return nil, fmt.Errorf("跳板机层级超过 %d 级", maxJumpDepth)
	}

	report := func(ph Phase) {
		if progress != nil {
			progress(ph)
		}
	}

	auth, closeAuth, err := authMethods(ctx, p)
	if err != nil {
		return nil, err
	}
	defer closeAuth()

	addr := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))

	var raw net.Conn
	if p.Jump != nil {
		report(PhaseJump)
		jump, err := dialWithProgress(ctx, *p.Jump, trust, progress, depth+1)
		if err != nil {
			return nil, fmt.Errorf("连接跳板机 %s 失败: %w", net.JoinHostPort(p.Jump.Host, strconv.Itoa(p.Jump.Port)), err)
		}
		raw, err = dialThroughJump(ctx, jump, addr)
		if err != nil {
			_ = jump.Close()
			return nil, err
		}
		raw = &jumpConn{Conn: raw, jump: jump}
	} else {
		report(PhaseTCP)
		d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		raw, err = d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("无法连接 %s: %w", addr, err)
		}
	}
	handshakeDeadline := 15 * time.Second
	if interactiveAuth(p.AuthType) {
		handshakeDeadline = interactiveHandshakeDeadline
	}
	raw.SetDeadline(time.Now().Add(handshakeDeadline))

	// Cancelling the context (the user closed the connecting tab) must abort
	// the handshake too, not just the TCP dial: closing the socket makes the
	// blocked handshake return at once.
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-ctx.Done():
			raw.Close()
		case <-watchDone:
		}
	}()

	report(PhaseKeyExchange)
	cfg := &ssh.ClientConfig{
		User: p.Username,
		Auth: auth,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			fp := ssh.FingerprintSHA256(key)
			if trust != nil && trust(hostname, fp) {
				// The host key is verified and the server's auth methods
				// negotiate next.
				report(PhaseAuth)
				return nil
			}
			// The old connection is discarded after the answer and the dial
			// retries from scratch, so its read deadline must not expire while
			// the user verifies the fingerprint.
			raw.SetDeadline(time.Time{})
			report(PhaseHostKey)
			return &HostKeyError{Host: hostname, Fingerprint: fp, Key: key}
		},
		Timeout: 15 * time.Second,
	}

	cc, chans, reqs, err := ssh.NewClientConn(raw, addr, cfg)
	if err != nil {
		raw.Close()
		return nil, err
	}
	raw.SetDeadline(time.Time{})
	report(PhaseSession)
	return ssh.NewClient(cc, chans, reqs), nil
}

func interactiveAuth(authType string) bool {
	authType = strings.ToLower(strings.TrimSpace(authType))
	return authType == AuthMFA || authType == authKeyboardInteractive
}

// jumpConn keeps the jump client alive for as long as the target connection.
// The SSH client closes the connection it was built on, which in turn closes
// the channel and the jump session.
type jumpConn struct {
	net.Conn
	jump *ssh.Client
}

func (c *jumpConn) Close() error {
	err := c.Conn.Close()
	jumpErr := c.jump.Close()
	if err != nil {
		return err
	}
	return jumpErr
}

func dialThroughJump(ctx context.Context, jump *ssh.Client, addr string) (net.Conn, error) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = jump.Close()
		case <-done:
		}
	}()

	conn, err := jump.Dial("tcp", addr)
	close(done)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("跳板机转发到 %s 失败: %w", addr, err)
	}
	return conn, nil
}

type agentDialer func() (io.ReadWriteCloser, error)

func authMethods(ctx context.Context, p Profile) ([]ssh.AuthMethod, func(), error) {
	return authMethodsWithContext(ctx, p, dialAgent)
}

// authMethodsWith builds the authentication methods for a profile. The
// returned close function releases any Agent connection after the SSH
// handshake; callers must always call it, including on error.
func authMethodsWith(p Profile, dial agentDialer) ([]ssh.AuthMethod, func(), error) {
	return authMethodsWithContext(context.Background(), p, dial)
}

func authMethodsWithContext(ctx context.Context, p Profile, dial agentDialer) ([]ssh.AuthMethod, func(), error) {
	noop := func() {}
	authType := strings.ToLower(strings.TrimSpace(p.AuthType))

	switch authType {
	case AuthPassword:
		if p.Password == "" {
			return nil, noop, errors.New("请填写密码")
		}
		return []ssh.AuthMethod{ssh.Password(p.Password)}, noop, nil
	case AuthKey:
		methods, err := keyAuthMethodsFor(ctx, p)
		if err != nil {
			return nil, noop, err
		}
		return methods, noop, nil
	case AuthMFA, authKeyboardInteractive:
		if p.Interactive == nil {
			return nil, noop, errors.New("服务器需要交互式认证，但当前会话无法显示认证提示")
		}
		host := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
		method := ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
			return p.Interactive(ctx, host, name, instruction, questions, echos)
		})
		return []ssh.AuthMethod{method}, noop, nil
	case AuthAgent:
		if dial == nil {
			return nil, noop, errors.New("SSH Agent 不可用")
		}
		conn, err := dial()
		if err != nil {
			return nil, noop, fmt.Errorf("无法连接 SSH Agent：%w", err)
		}
		methods, closeAgent, err := agentAuthMethods(conn)
		if err != nil {
			return nil, noop, err
		}
		return methods, closeAgent, nil
	case "":
		var methods []ssh.AuthMethod
		if p.Password != "" {
			methods = append(methods, ssh.Password(p.Password))
		}
		methods = append(methods, keyAuthMethods()...)
		if len(methods) == 0 {
			return nil, noop, errors.New("没有可用的认证方式：请填写密码，或把未加密私钥放到 ~/.xu/keys/id_ed25519")
		}
		return methods, noop, nil
	default:
		return nil, noop, fmt.Errorf("不支持的认证方式 %q", p.AuthType)
	}
}

func keyAuthMethodsFor(ctx context.Context, p Profile) ([]ssh.AuthMethod, error) {
	if path := strings.TrimSpace(p.KeyPath); path != "" {
		signer, err := signerForProfile(ctx, p, path)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}

	methods := keyAuthMethods()
	if len(methods) == 0 {
		return nil, errors.New("没有可用的私钥：请选择私钥文件，或把未加密私钥放到 ~/.xu/keys/id_ed25519、id_ecdsa、id_rsa")
	}
	return methods, nil
}

func signerForProfile(ctx context.Context, p Profile, path string) (ssh.Signer, error) {
	signer, err := LoadSigner(path, p.KeyPassphrase)
	if !errors.Is(err, ErrPrivateKeyNeedsPassphrase) || p.Passphrase == nil {
		return signer, err
	}

	host := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	passphrase, promptErr := p.Passphrase(ctx, host)
	if promptErr != nil {
		return nil, promptErr
	}
	if passphrase == "" {
		return nil, ErrPrivateKeyNeedsPassphrase
	}
	return LoadSigner(path, passphrase)
}

// LoadSigner reads one private key. Its errors never contain the passphrase.
func LoadSigner(path, passphrase string) (ssh.Signer, error) {
	path = expandHome(path)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取私钥 %s 失败: %w", filepath.Base(path), err)
	}
	signer, err := ssh.ParsePrivateKey(b)
	if err == nil {
		return signer, nil
	}

	var missing *ssh.PassphraseMissingError
	if !errors.As(err, &missing) {
		return nil, fmt.Errorf("无法解析私钥 %s: %w", filepath.Base(path), err)
	}
	if passphrase == "" {
		return nil, ErrPrivateKeyNeedsPassphrase
	}
	signer, err = ssh.ParsePrivateKeyWithPassphrase(b, []byte(passphrase))
	if err != nil {
		return nil, ErrPrivateKeyPassphrase
	}
	return signer, nil
}

func expandHome(path string) string {
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

// agentAuthMethods turns an open Agent protocol connection into SSH public
// key auth. The Agent performs the signature, so the private key never enters
// the application.
func agentAuthMethods(conn io.ReadWriteCloser) ([]ssh.AuthMethod, func(), error) {
	client := agent.NewClient(conn)
	signers, err := client.Signers()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("读取 SSH Agent 密钥失败：%w", err)
	}
	if len(signers) == 0 {
		_ = conn.Close()
		return nil, nil, errors.New("SSH Agent 中没有可用密钥，请先执行 ssh-add 加载私钥")
	}
	return []ssh.AuthMethod{ssh.PublicKeysCallback(client.Signers)}, func() {
		_ = conn.Close()
	}, nil
}

func keyAuthMethods() []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	for _, path := range defaultKeyPaths() {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(b)
		if err != nil {
			// Encrypted keys need a passphrase; support those later.
			continue
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	return methods
}

func defaultKeyPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dir := filepath.Join(home, ".xu", "keys")
	return []string{
		filepath.Join(dir, "id_ed25519"),
		filepath.Join(dir, "id_ecdsa"),
		filepath.Join(dir, "id_rsa"),
	}
}

package sshx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

type Profile struct {
	Host     string
	Port     int
	Username string
	Password string
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
	PhaseTCP         Phase = iota // dialing the TCP socket
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

// DialWithProgress is Dial with a callback that reports every phase, which
// the connecting tab uses to say what it is actually doing.
func DialWithProgress(ctx context.Context, p Profile, trust TrustFunc, progress PhaseFunc) (*ssh.Client, error) {
	report := func(ph Phase) {
		if progress != nil {
			progress(ph)
		}
	}

	auth, err := authMethods(p)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))

	report(PhaseTCP)
	d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("无法连接 %s: %w", addr, err)
	}
	raw.SetDeadline(time.Now().Add(15 * time.Second))

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

func authMethods(p Profile) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if p.Password != "" {
		methods = append(methods, ssh.Password(p.Password))
	}

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

	if len(methods) == 0 {
		return nil, errors.New("没有可用的认证方式：请填写密码，或把未加密私钥放到 ~/.xu/keys/id_ed25519")
	}
	return methods, nil
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

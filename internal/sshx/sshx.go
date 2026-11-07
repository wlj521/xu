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

func Dial(ctx context.Context, p Profile, trust TrustFunc) (*ssh.Client, error) {
	auth, err := authMethods(p)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))

	d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("无法连接 %s: %w", addr, err)
	}
	raw.SetDeadline(time.Now().Add(15 * time.Second))

	cfg := &ssh.ClientConfig{
		User: p.Username,
		Auth: auth,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			fp := ssh.FingerprintSHA256(key)
			if trust != nil && trust(hostname, fp) {
				return nil
			}
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

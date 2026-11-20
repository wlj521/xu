package sshx

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func writeEncryptedKey(t *testing.T, passphrase string) (string, ssh.Signer) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(privateKey, "xu test key", []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "id_xu_test")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, signer
}

func TestLoadSignerWithPassphrase(t *testing.T) {
	path, want := writeEncryptedKey(t, "correct horse battery staple")

	if _, err := LoadSigner(path, ""); !errors.Is(err, ErrPrivateKeyNeedsPassphrase) {
		t.Fatalf("unlocked encrypted key error = %v", err)
	}
	if _, err := LoadSigner(path, "wrong"); !errors.Is(err, ErrPrivateKeyPassphrase) {
		t.Fatalf("wrong passphrase error = %v", err)
	}
	got, err := LoadSigner(path, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.PublicKey().Marshal(), want.PublicKey().Marshal()) {
		t.Fatal("the unlocked signer does not match the generated key")
	}
}

func TestExplicitPrivateKeyAuthenticationSignsThroughHandshake(t *testing.T) {
	path, keySigner := writeEncryptedKey(t, "key-passphrase")
	hostSigner, _ := testSigner(t)
	accepted := make(chan ssh.PublicKey, 1)
	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if !bytes.Equal(key.Marshal(), keySigner.PublicKey().Marshal()) {
				return nil, errors.New("client offered an unexpected key")
			}
			select {
			case accepted <- key:
			default:
			}
			return nil, nil
		},
	}
	serverConfig.AddHostKey(hostSigner)

	prompted := 0
	auth, closeAuth, err := authMethodsWith(Profile{
		Host: "example.com", Port: 2222, AuthType: AuthKey, KeyPath: path,
		Passphrase: func(_ context.Context, host string) (string, error) {
			prompted++
			if host != "example.com:2222" {
				t.Fatalf("passphrase prompt host = %q", host)
			}
			return "key-passphrase", nil
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer closeAuth()
	if prompted != 1 {
		t.Fatalf("passphrase prompts = %d, want 1", prompted)
	}

	if err := runAuthHandshake(serverConfig, auth); err != nil {
		t.Fatalf("SSH handshake with an explicit key: %v", err)
	}
	select {
	case got := <-accepted:
		if !bytes.Equal(got.Marshal(), keySigner.PublicKey().Marshal()) {
			t.Fatal("the server accepted a different key")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the server did not receive the explicit key")
	}
}

func TestKeyboardInteractiveAuthentication(t *testing.T) {
	hostSigner, _ := testSigner(t)
	serverConfig := &ssh.ServerConfig{
		KeyboardInteractiveCallback: func(_ ssh.ConnMetadata, client ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			answers, err := client("Two-factor authentication", "Enter the current code", []string{"Verification code: "}, []bool{false})
			if err != nil {
				return nil, err
			}
			if len(answers) != 1 || answers[0] != "123456" {
				return nil, errors.New("unexpected verification code")
			}
			return nil, nil
		},
	}
	serverConfig.AddHostKey(hostSigner)

	auth, closeAuth, err := authMethodsWith(Profile{
		Host: "example.com", Port: 22, AuthType: AuthMFA,
		Interactive: func(_ context.Context, host, name, instruction string, questions []string, echos []bool) ([]string, error) {
			if host != "example.com:22" || name != "Two-factor authentication" {
				t.Fatalf("prompt context = %q %q", host, name)
			}
			if instruction != "Enter the current code" || len(questions) != 1 || echos[0] {
				t.Fatalf("prompt = %q %v %v", instruction, questions, echos)
			}
			return []string{"123456"}, nil
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer closeAuth()
	if err := runAuthHandshake(serverConfig, auth); err != nil {
		t.Fatalf("SSH handshake with keyboard-interactive auth: %v", err)
	}
}

func TestMFAWithoutPromptIsRejected(t *testing.T) {
	if _, closeAuth, err := authMethodsWith(Profile{AuthType: AuthMFA}, nil); err == nil {
		closeAuth()
		t.Fatal("MFA without a prompt callback must fail")
	}
}

func runAuthHandshake(serverConfig *ssh.ServerConfig, auth []ssh.AuthMethod) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		raw, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		server, _, _, err := ssh.NewServerConn(raw, serverConfig)
		if err != nil {
			_ = raw.Close()
			serverDone <- err
			return
		}
		serverDone <- nil
		_ = server.Close()
	}()

	raw, err := net.DialTimeout("tcp", listener.Addr().String(), 5*time.Second)
	if err != nil {
		return err
	}
	config := &ssh.ClientConfig{
		User:            "tester",
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, channels, requests, err := ssh.NewClientConn(raw, listener.Addr().String(), config)
	if err != nil {
		_ = raw.Close()
		return err
	}
	_ = channels
	_ = requests
	_ = client.Close()

	select {
	case err := <-serverDone:
		return err
	case <-time.After(5 * time.Second):
		return errors.New("SSH test server did not finish the handshake")
	}
}

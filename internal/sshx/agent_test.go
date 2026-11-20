package sshx

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func testSigner(t *testing.T) (ssh.Signer, ed25519.PrivateKey) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signer, privateKey
}

func TestAgentAuthenticationSignsThroughSSHHandshake(t *testing.T) {
	keySigner, privateKey := testSigner(t)

	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: privateKey}); err != nil {
		t.Fatal(err)
	}
	agentClient, agentServer := net.Pipe()
	go func() { _ = agent.ServeAgent(keyring, agentServer) }()

	auth, closeAuth, err := authMethodsWith(Profile{AuthType: AuthAgent}, func() (io.ReadWriteCloser, error) {
		return agentClient, nil
	})
	if err != nil {
		t.Fatalf("build agent auth: %v", err)
	}
	defer closeAuth()
	if len(auth) != 1 {
		t.Fatalf("auth methods = %d, want 1", len(auth))
	}

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

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
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
		t.Fatal(err)
	}
	clientConfig := &ssh.ClientConfig{
		User:            "tester",
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, clientChans, clientReqs, err := ssh.NewClientConn(raw, listener.Addr().String(), clientConfig)
	if err != nil {
		_ = raw.Close()
		t.Fatalf("SSH handshake with agent auth: %v", err)
	}
	_ = clientChans
	_ = clientReqs
	_ = client.Close()

	select {
	case got := <-accepted:
		if !bytes.Equal(got.Marshal(), keySigner.PublicKey().Marshal()) {
			t.Fatalf("accepted key = %s, want %s", got.Type(), keySigner.PublicKey().Type())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not receive the agent's public key")
	}

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("SSH server: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SSH server did not finish the handshake")
	}
}

func TestAgentAuthenticationRequiresLoadedKey(t *testing.T) {
	agentClient, agentServer := net.Pipe()
	go func() { _ = agent.ServeAgent(agent.NewKeyring(), agentServer) }()

	_, closeAuth, err := authMethodsWith(Profile{AuthType: AuthAgent}, func() (io.ReadWriteCloser, error) {
		return agentClient, nil
	})
	if err == nil {
		closeAuth()
		t.Fatal("an empty agent must not produce an authentication method")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("ssh-add")) {
		t.Fatalf("error should tell the user how to load a key: %v", err)
	}
}

func TestExplicitPasswordAuthRequiresPassword(t *testing.T) {
	_, closeAuth, err := authMethodsWith(Profile{AuthType: AuthPassword}, nil)
	if err == nil {
		closeAuth()
		t.Fatal("password auth without a password must fail")
	}
}

func TestUnknownAuthTypeIsRejected(t *testing.T) {
	_, closeAuth, err := authMethodsWith(Profile{AuthType: "token"}, nil)
	if err == nil {
		closeAuth()
		t.Fatal("unknown auth type must fail")
	}
}

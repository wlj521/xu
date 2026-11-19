package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func newRMTestClient(t *testing.T, execDelay time.Duration) *ssh.Client {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveRMTestConn(conn, cfg, execDelay)
		}
	}()

	client, err := ssh.Dial("tcp", ln.Addr().String(), &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func serveRMTestConn(conn net.Conn, cfg *ssh.ServerConfig, execDelay time.Duration) {
	defer conn.Close()

	_, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		ch, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer ch.Close()
			for req := range requests {
				switch req.Type {
				case "exec":
					_ = req.Reply(true, nil)
					time.Sleep(execDelay)
					_, _ = ch.Write([]byte("ok"))
					_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
					return
				case "signal":
					_ = req.Reply(true, nil)
					return
				default:
					_ = req.Reply(false, nil)
				}
			}
		}()
	}
}

func TestRunRemoteRMStress(t *testing.T) {
	client := newRMTestClient(t, 0)
	for i := 0; i < 100; i++ {
		if err := runRemoteRM(context.Background(), client, fmt.Sprintf("/tmp/rm-%d", i)); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

func TestRunRemoteRMContextExpiresDuringExecStress(t *testing.T) {
	client := newRMTestClient(t, 10*time.Millisecond)
	for i := 0; i < 50; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		err := runRemoteRM(ctx, client, fmt.Sprintf("/tmp/rm-%d", i))
		cancel()
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("run %d: err = %v, want cancellation", i, err)
		}
	}
}

func TestRunRemoteRMConnectionDropStress(t *testing.T) {
	for i := 0; i < 50; i++ {
		client := newRMTestClient(t, 200*time.Millisecond)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = runRemoteRM(context.Background(), client, "/tmp/rm")
		}()
		time.Sleep(2 * time.Millisecond)
		_ = client.Close()
		wg.Wait()
	}
}

func TestRemoveTreeRemoteConnectionDropStress(t *testing.T) {
	for i := 0; i < 30; i++ {
		root := t.TempDir()
		target := filepath.Join(root, "tree")
		writeTree(t, target)
		client := newTestSFTP(t, root)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = removeTree(context.Background(), true, client, "tree", nil)
		}()
		time.Sleep(time.Millisecond)
		_ = client.Close()
		wg.Wait()
	}
}

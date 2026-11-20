package sshx

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func startJumpTestServer(t *testing.T, user, password string, forward bool) string {
	t.Helper()
	hostSigner, _ := testSigner(t)
	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if conn.User() == user && string(pw) == password {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected")
		},
	}
	config.AddHostKey(hostSigner)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				server, channels, requests, err := ssh.NewServerConn(raw, config)
				if err != nil {
					_ = raw.Close()
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for newChannel := range channels {
					if !forward || newChannel.ChannelType() != "direct-tcpip" {
						_ = newChannel.Reject(ssh.Prohibited, "no channels")
						continue
					}
					handleDirectTCPIP(newChannel)
				}
			}()
		}
	}()
	return listener.Addr().String()
}

func handleDirectTCPIP(newChannel ssh.NewChannel) {
	var request struct {
		DestinationAddress string
		DestinationPort    uint32
		OriginAddress      string
		OriginPort         uint32
	}
	if err := ssh.Unmarshal(newChannel.ExtraData(), &request); err != nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	conn, err := net.Dial(
		"tcp",
		net.JoinHostPort(request.DestinationAddress, strconv.FormatUint(uint64(request.DestinationPort), 10)),
	)
	if err != nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	channel, requests, err := newChannel.Accept()
	if err != nil {
		_ = conn.Close()
		return
	}
	go ssh.DiscardRequests(requests)
	go func() {
		defer channel.Close()
		defer conn.Close()
		_, _ = io.Copy(conn, channel)
	}()
	go func() {
		defer channel.Close()
		defer conn.Close()
		_, _ = io.Copy(channel, conn)
	}()
}

func jumpTestEndpoint(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func TestDialThroughJumpHost(t *testing.T) {
	targetAddr := startJumpTestServer(t, "target", "target-pass", false)
	jumpAddr := startJumpTestServer(t, "jump", "jump-pass", true)
	targetHost, targetPort := jumpTestEndpoint(t, targetAddr)
	jumpHost, jumpPort := jumpTestEndpoint(t, jumpAddr)

	profile := Profile{
		Host:     targetHost,
		Port:     targetPort,
		Username: "target",
		Password: "target-pass",
		AuthType: AuthPassword,
		Jump: &Profile{
			Host:     jumpHost,
			Port:     jumpPort,
			Username: "jump",
			Password: "jump-pass",
			AuthType: AuthPassword,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := DialWithProgress(ctx, profile, func(_, _ string) bool { return true }, nil)
	if err != nil {
		t.Fatalf("dial through jump: %v", err)
	}
	if client.User() != "target" {
		t.Fatalf("connected as %q, want target", client.User())
	}
	_ = client.Close()
}

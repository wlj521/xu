//go:build !windows

package sshx

import (
	"errors"
	"io"
	"net"
	"os"
)

func dialAgent() (io.ReadWriteCloser, error) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, errors.New("SSH_AUTH_SOCK 未设置（请先启动 ssh-agent）")
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

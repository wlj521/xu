//go:build windows

package sshx

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

const windowsAgentPipe = `\\.\pipe\openssh-ssh-agent`

func dialAgent() (io.ReadWriteCloser, error) {
	handle, err := windows.CreateFile(
		windows.StringToUTF16Ptr(windowsAgentPipe),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("打开 OpenSSH Agent 管道失败：%w；请启动 OpenSSH Authentication Agent 服务", err)
	}
	return os.NewFile(uintptr(handle), windowsAgentPipe), nil
}

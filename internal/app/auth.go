package app

import (
	"strings"

	"xu/internal/sshx"
)

const (
	authModePassword = iota
	authModeKey
	authModeAgent
	authModeMFA
)

var authModeTypes = []string{sshx.AuthPassword, sshx.AuthKey, sshx.AuthAgent, sshx.AuthMFA}

func authTypeForMode(mode int) string {
	if mode < 0 || mode >= len(authModeTypes) {
		return sshx.AuthPassword
	}
	return authModeTypes[mode]
}

func authModeForType(authType string) int {
	authType = strings.ToLower(strings.TrimSpace(authType))
	if authType == sshx.AuthMFA || authType == "keyboard-interactive" {
		return authModeMFA
	}
	for i, candidate := range authModeTypes {
		if authType == candidate {
			return i
		}
	}
	return authModePassword
}

func authModeHint(mode int, editing bool) string {
	switch mode {
	case authModeKey:
		return "选择任意私钥文件；加密私钥会安全询问口令。留空时仍尝试 ~/.xu/keys 下的默认私钥。"
	case authModeAgent:
		return "使用系统 SSH Agent 中的密钥签名，私钥不会进入 Xu。Windows 需先启动 OpenSSH Authentication Agent 并执行 ssh-add。"
	case authModeMFA:
		return "连接时由服务器发起 keyboard-interactive 提示，在应用内输入验证码或动态口令。"
	default:
		if editing {
			return "密码保存在系统钥匙串；留空则沿用已保存密码。"
		}
		return "密码会保存到系统钥匙串，不会写入会话数据库。"
	}
}

func isPasswordAuth(authType string) bool {
	authType = strings.ToLower(strings.TrimSpace(authType))
	return authType == "" || authType == sshx.AuthPassword
}

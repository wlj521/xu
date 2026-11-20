package app

import (
	"strings"

	"xu/internal/sshx"
)

const (
	authModePassword = iota
	authModeKey
	authModeAgent
)

var authModeTypes = []string{sshx.AuthPassword, sshx.AuthKey, sshx.AuthAgent}

func authTypeForMode(mode int) string {
	if mode < 0 || mode >= len(authModeTypes) {
		return sshx.AuthPassword
	}
	return authModeTypes[mode]
}

func authModeForType(authType string) int {
	authType = strings.ToLower(strings.TrimSpace(authType))
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
		return "读取 ~/.xu/keys 下的未加密私钥：id_ed25519、id_ecdsa 或 id_rsa。"
	case authModeAgent:
		return "使用系统 SSH Agent 中的密钥签名，私钥不会进入 Xu。Windows 需先启动 OpenSSH Authentication Agent 并执行 ssh-add。"
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

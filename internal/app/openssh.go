package app

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
)

type sshConfigBlock struct {
	aliases    []string
	hostName   string
	user       string
	port       int
	identity   string
	proxyJump  string
	sourceLine int
}

func parseOpenSSHConfig(data []byte) (xuBackup, []string, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	var blocks []sshConfigBlock
	var current *sshConfigBlock
	includeSeen := false
	matchSeen := false
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(stripSSHConfigComment(scanner.Text()))
		if line == "" {
			continue
		}
		key, value, ok := splitSSHConfigLine(line)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "host":
			if current != nil {
				blocks = append(blocks, *current)
			}
			current = &sshConfigBlock{
				aliases:    strings.Fields(value),
				port:       22,
				sourceLine: lineNo,
			}
		case "match":
			matchSeen = true
			if current != nil {
				blocks = append(blocks, *current)
				current = nil
			}
		case "hostname":
			if current != nil {
				current.hostName = unquoteSSHConfigValue(value)
			}
		case "user":
			if current != nil {
				current.user = unquoteSSHConfigValue(value)
			}
		case "port":
			if current != nil {
				if port, err := strconv.Atoi(unquoteSSHConfigValue(value)); err == nil && port > 0 && port <= 65535 {
					current.port = port
				}
			}
		case "identityfile":
			if current != nil && current.identity == "" {
				current.identity = expandKeyPath(unquoteSSHConfigValue(value))
			}
		case "proxyjump":
			if current != nil {
				current.proxyJump = unquoteSSHConfigValue(value)
			}
		case "include":
			includeSeen = true
		}
	}
	if err := scanner.Err(); err != nil {
		return xuBackup{}, nil, err
	}
	if current != nil {
		blocks = append(blocks, *current)
	}
	if len(blocks) == 0 {
		return xuBackup{}, nil, errors.New("未找到 Host 配置")
	}

	warnings := make(map[string]bool)
	defaultUser := defaultSSHConfigUser()
	if includeSeen {
		warnings["未展开 Include 指令；请直接选择被包含的配置文件"] = true
	}
	if matchSeen {
		warnings["已忽略 Match 条件块"] = true
	}
	profiles := make([]backupProfile, 0, len(blocks))
	index := make(map[string]int)
	proxyByAlias := make(map[string]string)
	for _, block := range blocks {
		for _, alias := range block.aliases {
			alias = strings.TrimSpace(alias)
			alias = unquoteSSHConfigValue(alias)
			if alias == "" {
				continue
			}
			if strings.ContainsAny(alias, "*?!") {
				warnings["已跳过通配或否定 Host 模式"] = true
				continue
			}
			host := strings.TrimSpace(block.hostName)
			if host == "" {
				host = alias
			}
			user := strings.TrimSpace(block.user)
			if user == "" {
				user = defaultUser
			}
			port := block.port
			if port <= 0 || port > 65535 {
				port = 22
			}
			authType := "password"
			if block.identity != "" {
				authType = "key"
			}
			profile := backupProfile{
				Name:        alias,
				Host:        host,
				Port:        port,
				Username:    user,
				AuthType:    authType,
				KeyPath:     block.identity,
				Group:       "OpenSSH 导入",
				JumpProfile: -1,
			}
			if i, found := index[strings.ToLower(alias)]; found {
				profiles[i] = profile
			} else {
				index[strings.ToLower(alias)] = len(profiles)
				profiles = append(profiles, profile)
			}
			if block.proxyJump != "" {
				proxyByAlias[strings.ToLower(alias)] = block.proxyJump
			}
		}
	}
	if len(profiles) == 0 {
		return xuBackup{}, nil, errors.New("配置中没有可导入的具体 Host")
	}
	for alias, rawJump := range proxyByAlias {
		target := sshConfigJumpAlias(rawJump)
		idx, ok := index[strings.ToLower(target)]
		if !ok || idx == index[alias] {
			warnings["部分 ProxyJump 无法映射到已导入的 Host，请在导入后手动选择跳板机"] = true
			continue
		}
		profiles[index[alias]].JumpProfile = idx
	}

	list := make([]string, 0, len(warnings))
	for warning := range warnings {
		list = append(list, warning)
	}
	sort.Strings(list)
	return xuBackup{
		Format:     xuBackupFormat,
		Version:    xuBackupVersion,
		ExportedAt: "OpenSSH import",
		Groups:     []backupGroup{{Name: "OpenSSH 导入"}},
		Profiles:   profiles,
	}, list, nil
}

func stripSSHConfigComment(line string) string {
	var quote rune
	escaped := false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == '#' {
			return line[:i]
		}
	}
	return line
}

func splitSSHConfigLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", false
	}
	end := len(line)
	for i, r := range line {
		if r == '=' || r == ' ' || r == '\t' {
			end = i
			break
		}
	}
	key = strings.TrimSpace(line[:end])
	value = strings.TrimSpace(line[end:])
	value = strings.TrimPrefix(value, "=")
	value = strings.TrimSpace(value)
	return key, value, key != ""
}

func unquoteSSHConfigValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func defaultSSHConfigUser() string {
	for _, name := range []string{"USERNAME", "USER", "LOGNAME"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return "root"
}

func sshConfigJumpAlias(raw string) string {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "ssh://"))
	if raw == "" || strings.EqualFold(raw, "none") || strings.Contains(raw, ",") {
		return ""
	}
	if at := strings.LastIndex(raw, "@"); at >= 0 {
		raw = raw[at+1:]
	}
	if strings.Count(raw, ":") == 1 {
		if colon := strings.LastIndex(raw, ":"); colon > 0 {
			raw = raw[:colon]
		}
	}
	return strings.TrimSpace(raw)
}

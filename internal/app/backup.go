package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"xu/internal/store"
)

const (
	xuBackupFormat  = "xu-backup"
	xuBackupVersion = 1
)

type xuBackup struct {
	Format        string               `json:"format"`
	Version       int                  `json:"version"`
	ExportedAt    string               `json:"exported_at"`
	Groups        []backupGroup        `json:"groups,omitempty"`
	Profiles      []backupProfile      `json:"profiles"`
	QuickCommands []backupQuickCommand `json:"quick_commands,omitempty"`
	Forwards      []backupForward      `json:"forwards,omitempty"`
	Settings      map[string]string    `json:"settings,omitempty"`
}

type backupGroup struct {
	Name string `json:"name"`
}

type backupProfile struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	AuthType    string `json:"auth_type"`
	KeyPath     string `json:"key_path,omitempty"`
	Group       string `json:"group,omitempty"`
	JumpProfile int    `json:"jump_profile"`
}

// backupQuickCommand.Profile is -1 for a global command, otherwise it indexes
// xuBackup.Profiles.
type backupQuickCommand struct {
	Profile int    `json:"profile"`
	Name    string `json:"name"`
	Command string `json:"command"`
}

// backupForward.Profile indexes xuBackup.Profiles.
type backupForward struct {
	Profile   int    `json:"profile"`
	Kind      int    `json:"kind"`
	Listen    string `json:"listen"`
	Target    string `json:"target"`
	AutoStart bool   `json:"auto_start"`
}

type backupImportSummary struct {
	Created        int
	Updated        int
	Skipped        int
	Groups         int
	Commands       int
	Forwards       int
	Settings       int
	NeedPassword   int
	Warnings       int
	Failed         int
	SourceProfiles int
}

func (s backupImportSummary) notice() string {
	parts := []string{}
	switch {
	case s.Created+s.Updated > 0:
		parts = append(parts, fmt.Sprintf("已导入 %d 个连接", s.Created+s.Updated))
	case s.SourceProfiles > 0:
		parts = append(parts, "没有导入连接")
	}
	if s.Updated > 0 {
		parts = append(parts, fmt.Sprintf("更新 %d 个", s.Updated))
	}
	if s.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("跳过 %d 个", s.Skipped))
	}
	if s.Groups > 0 {
		parts = append(parts, fmt.Sprintf("新增 %d 个分组", s.Groups))
	}
	if s.Commands > 0 {
		parts = append(parts, fmt.Sprintf("%d 条快捷命令", s.Commands))
	}
	if s.Forwards > 0 {
		parts = append(parts, fmt.Sprintf("%d 条转发规则", s.Forwards))
	}
	if s.Settings > 0 {
		parts = append(parts, fmt.Sprintf("%d 项设置", s.Settings))
	}
	if s.NeedPassword > 0 {
		parts = append(parts, fmt.Sprintf("%d 个密码需重新输入", s.NeedPassword))
	}
	if s.Warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d 条提示", s.Warnings))
	}
	if s.Failed > 0 {
		parts = append(parts, fmt.Sprintf("失败 %d 个", s.Failed))
	}
	if len(parts) == 0 {
		return "备份中没有可导入的内容"
	}
	return strings.Join(parts, " · ")
}

var saveBackupFile = func(opts mygo.SaveDialogOptions) (string, error) {
	return mygo.Dialog.Save(opts)
}

var openBackupFile = func(opts mygo.OpenDialogOptions) ([]string, error) {
	return mygo.Dialog.Open(opts)
}

func buildXuBackup(ctx context.Context, st *store.Store) (xuBackup, error) {
	if st == nil {
		return xuBackup{}, errors.New("会话库未就绪")
	}
	groups, err := st.Groups(ctx)
	if err != nil {
		return xuBackup{}, err
	}
	profiles, err := st.Profiles(ctx)
	if err != nil {
		return xuBackup{}, err
	}
	commands, err := st.QuickCommands(ctx)
	if err != nil {
		return xuBackup{}, err
	}
	settings, err := st.Settings(ctx)
	if err != nil {
		return xuBackup{}, err
	}

	groupName := make(map[int64]string, len(groups))
	backup := xuBackup{
		Format:     xuBackupFormat,
		Version:    xuBackupVersion,
		ExportedAt: time.Now().Format(time.RFC3339),
		Profiles:   make([]backupProfile, 0, len(profiles)),
		Settings:   make(map[string]string),
	}
	for _, group := range groups {
		groupName[group.ID] = group.Name
		backup.Groups = append(backup.Groups, backupGroup{Name: group.Name})
	}
	for key, value := range settings {
		if backupSettingAllowed(key) {
			backup.Settings[key] = value
		}
	}

	profileIndex := make(map[int64]int, len(profiles))
	for i, p := range profiles {
		profileIndex[p.ID] = i
	}
	for _, p := range profiles {
		jump := -1
		if p.JumpProfileID != 0 {
			if idx, ok := profileIndex[p.JumpProfileID]; ok {
				jump = idx
			}
		}
		backup.Profiles = append(backup.Profiles, backupProfile{
			Name:        p.Name,
			Host:        p.Host,
			Port:        p.Port,
			Username:    p.Username,
			AuthType:    p.AuthType,
			KeyPath:     p.KeyPath,
			Group:       groupName[p.GroupID],
			JumpProfile: jump,
		})
	}

	for _, command := range commands {
		profile := -1
		if command.ProfileID != 0 {
			idx, ok := profileIndex[command.ProfileID]
			if !ok {
				continue
			}
			profile = idx
		}
		backup.QuickCommands = append(backup.QuickCommands, backupQuickCommand{
			Profile: profile,
			Name:    command.Name,
			Command: command.Command,
		})
	}

	for _, p := range profiles {
		idx := profileIndex[p.ID]
		rules, err := st.Forwards(ctx, p.ID)
		if err != nil {
			return xuBackup{}, err
		}
		for _, rule := range rules {
			backup.Forwards = append(backup.Forwards, backupForward{
				Profile:   idx,
				Kind:      rule.Kind,
				Listen:    rule.Listen,
				Target:    rule.Target,
				AutoStart: rule.AutoStart,
			})
		}
	}
	return backup, nil
}

func backupSettingAllowed(key string) bool {
	for _, prefix := range []string{"terminal.", "ui.", "connection.", "transfer."} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (a *App) exportBackup() {
	if a.store == nil {
		a.backupFailure("会话库未就绪，暂时不能导出")
		return
	}
	backup, err := buildXuBackup(a.ctx, a.store)
	if err != nil {
		a.backupFailure("生成备份失败: " + err.Error())
		return
	}
	path, err := saveBackupFile(mygo.SaveDialogOptions{
		Parent:      a.win,
		Title:       "导出 Xu 连接备份",
		DefaultPath: defaultBackupPath(),
		ButtonLabel: "导出",
		Filters:     []mygo.FileFilter{{Name: "Xu 备份", Extensions: []string{"json"}}},
	})
	if err != nil {
		a.backupFailure("打开保存对话框失败: " + err.Error())
		return
	}
	if strings.TrimSpace(path) == "" {
		return
	}
	if filepath.Ext(path) == "" {
		path += ".json"
	}
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		a.backupFailure("生成 JSON 失败: " + err.Error())
		return
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		a.backupFailure("写入备份失败: " + err.Error())
		return
	}
	notice := fmt.Sprintf("已导出 %d 个连接 · %d 个分组 · %d 条快捷命令 · %d 条转发规则 · %d 项设置",
		len(backup.Profiles), len(backup.Groups), len(backup.QuickCommands), len(backup.Forwards), len(backup.Settings))
	a.managerErr = ""
	a.managerNotice = notice
	a.status = notice + " → " + filepath.Base(path)
}

func defaultBackupPath() string {
	name := "xu-backup-" + time.Now().Format("20060102-150405") + ".json"
	home, err := os.UserHomeDir()
	if err != nil {
		return name
	}
	downloads := filepath.Join(home, "Downloads")
	if info, err := os.Stat(downloads); err == nil && info.IsDir() {
		return filepath.Join(downloads, name)
	}
	return filepath.Join(home, name)
}

func (a *App) backupFailure(msg string) {
	a.managerErr = msg
	a.status = msg
}

func (a *App) openBackupImporter() {
	if a.store == nil {
		a.backupFailure("会话库未就绪，暂时不能导入")
		return
	}
	opts := mygo.OpenDialogOptions{
		Parent: a.win,
		Title:  "导入 Xu 备份或 OpenSSH config",
		Filters: []mygo.FileFilter{
			{Name: "Xu 备份 / OpenSSH config", Extensions: []string{"json", "conf", "config", "txt"}},
			{Name: "所有文件", Extensions: []string{"*"}},
		},
	}
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, ".ssh")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			opts.DefaultPath = dir
		}
	}
	paths, err := openBackupFile(opts)
	if err != nil {
		a.backupFailure("打开文件选择器失败: " + err.Error())
		return
	}
	if len(paths) == 0 {
		return
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		a.backupFailure("读取备份失败: " + err.Error())
		return
	}
	backup, source, warnings, err := parseBackupFile(data)
	if err != nil {
		a.backupFailure("解析备份失败: " + err.Error())
		return
	}
	if len(backup.Profiles) == 0 {
		a.backupFailure("文件中没有可导入的 SSH 连接")
		return
	}
	a.backupPath = paths[0]
	a.backupSource = source
	a.backupPreview = backup
	a.backupConflictPick = "跳过"
	a.backupIncludeSettings = source == "Xu 备份" && len(backup.Settings) > 0
	a.backupWarnings = warnings
	a.backupOpen = true
	a.managerErr = ""
}

func parseBackupFile(data []byte) (xuBackup, string, []string, error) {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	if len(data) == 0 {
		return xuBackup{}, "", nil, errors.New("文件为空")
	}
	if data[0] == '{' {
		backup, err := parseXuBackup(data)
		return backup, "Xu 备份", nil, err
	}
	backup, warnings, err := parseOpenSSHConfig(data)
	return backup, "OpenSSH config", warnings, err
}

func parseXuBackup(data []byte) (xuBackup, error) {
	var backup xuBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		return xuBackup{}, err
	}
	if backup.Format != xuBackupFormat {
		return xuBackup{}, errors.New("不是 Xu 连接备份")
	}
	if backup.Version != xuBackupVersion {
		return xuBackup{}, fmt.Errorf("不支持的备份版本 %d", backup.Version)
	}
	for i := range backup.Profiles {
		if backup.Profiles[i].Port <= 0 || backup.Profiles[i].Port > 65535 {
			return xuBackup{}, fmt.Errorf("连接 %q 的端口无效", backup.Profiles[i].Name)
		}
		if backup.Profiles[i].JumpProfile < -1 || backup.Profiles[i].JumpProfile >= len(backup.Profiles) {
			return xuBackup{}, fmt.Errorf("连接 %q 引用了不存在的跳板机", backup.Profiles[i].Name)
		}
	}
	for _, command := range backup.QuickCommands {
		if command.Profile < -1 || command.Profile >= len(backup.Profiles) {
			return xuBackup{}, fmt.Errorf("快捷命令 %q 引用了不存在的连接", command.Name)
		}
	}
	for _, rule := range backup.Forwards {
		if rule.Profile < 0 || rule.Profile >= len(backup.Profiles) {
			return xuBackup{}, fmt.Errorf("转发规则 %q 引用了不存在的连接", rule.Listen)
		}
	}
	return backup, nil
}

type backupConflictPolicy int

const (
	backupConflictSkip backupConflictPolicy = iota
	backupConflictOverwrite
	backupConflictRename
)

func backupConflictForLabel(label string) backupConflictPolicy {
	switch label {
	case "覆盖":
		return backupConflictOverwrite
	case "重命名":
		return backupConflictRename
	default:
		return backupConflictSkip
	}
}

func (a *App) applyBackupPreview() backupImportSummary {
	summary := backupImportSummary{SourceProfiles: len(a.backupPreview.Profiles)}
	if a.store == nil {
		summary.Failed++
		return summary
	}
	policy := backupConflictForLabel(a.backupConflictPick)
	a.refreshGroups()
	a.refreshProfiles()

	groupIDs := make(map[string]int64, len(a.groups))
	for _, group := range a.groups {
		groupIDs[strings.ToLower(strings.TrimSpace(group.Name))] = group.ID
	}
	groupNames := make(map[string]bool)
	for _, group := range a.backupPreview.Groups {
		groupNames[strings.ToLower(strings.TrimSpace(group.Name))] = true
	}
	for _, profile := range a.backupPreview.Profiles {
		groupNames[strings.ToLower(strings.TrimSpace(profile.Group))] = true
	}
	for name := range groupNames {
		if name == "" || groupIDs[name] != 0 {
			continue
		}
		group := store.Group{Name: nameForImportedGroup(a.backupPreview, name)}
		if err := a.store.SaveGroup(a.ctx, &group); err != nil {
			summary.Failed++
			summary.Warnings++
			continue
		}
		groupIDs[name] = group.ID
		summary.Groups++
	}

	byName := make(map[string]store.Profile, len(a.profiles))
	byEndpoint := make(map[string]store.Profile, len(a.profiles))
	usedNames := make(map[string]bool, len(a.profiles))
	for _, profile := range a.profiles {
		byName[strings.ToLower(profile.Name)] = profile
		byEndpoint[fsProfileKey(profile.Host, profile.Port, profile.Username)] = profile
		usedNames[strings.ToLower(profile.Name)] = true
	}

	actualIDs := make([]int64, len(a.backupPreview.Profiles))
	owned := make([]bool, len(a.backupPreview.Profiles))
	for i, imported := range a.backupPreview.Profiles {
		imported.Name = strings.TrimSpace(imported.Name)
		imported.Host = strings.TrimSpace(imported.Host)
		imported.Username = strings.TrimSpace(imported.Username)
		if imported.Name == "" {
			imported.Name = imported.Username + "@" + imported.Host
		}
		if imported.Host == "" || imported.Username == "" || imported.Port <= 0 || imported.Port > 65535 {
			summary.Skipped++
			continue
		}
		existing, found := byName[strings.ToLower(imported.Name)]
		if !found {
			existing, found = byEndpoint[fsProfileKey(imported.Host, imported.Port, imported.Username)]
		}
		if found && policy == backupConflictSkip {
			actualIDs[i] = existing.ID
			summary.Skipped++
			continue
		}

		profile := store.Profile{
			Name:     imported.Name,
			Host:     imported.Host,
			Port:     imported.Port,
			Username: imported.Username,
			AuthType: strings.ToLower(strings.TrimSpace(imported.AuthType)),
			KeyPath:  imported.KeyPath,
			GroupID:  groupIDs[strings.ToLower(strings.TrimSpace(imported.Group))],
		}
		if profile.AuthType == "" {
			profile.AuthType = "password"
		}
		if found && policy == backupConflictOverwrite {
			profile.ID = existing.ID
		} else if found || usedNames[strings.ToLower(profile.Name)] {
			profile.Name = uniqueBackupName(profile.Name, usedNames)
		}
		if err := a.store.SaveProfile(a.ctx, &profile); err != nil {
			summary.Failed++
			continue
		}
		actualIDs[i] = profile.ID
		owned[i] = true
		if found && policy == backupConflictOverwrite {
			summary.Updated++
		} else {
			summary.Created++
		}
		if profile.AuthType == "password" || profile.AuthType == "" {
			summary.NeedPassword++
		}
		usedNames[strings.ToLower(profile.Name)] = true
		byName[strings.ToLower(profile.Name)] = profile
		byEndpoint[fsProfileKey(profile.Host, profile.Port, profile.Username)] = profile
	}

	profiles, _ := a.store.Profiles(a.ctx)
	byID := make(map[int64]store.Profile, len(profiles))
	for _, profile := range profiles {
		byID[profile.ID] = profile
	}
	for i, imported := range a.backupPreview.Profiles {
		if !owned[i] || imported.JumpProfile < 0 || imported.JumpProfile >= len(actualIDs) {
			continue
		}
		target := actualIDs[imported.JumpProfile]
		if target == 0 || target == actualIDs[i] {
			continue
		}
		profile, ok := byID[actualIDs[i]]
		if !ok {
			continue
		}
		profile.JumpProfileID = target
		if err := a.store.SaveProfile(a.ctx, &profile); err != nil {
			summary.Failed++
		}
	}

	a.refreshProfiles()
	for i := range a.backupPreview.Profiles {
		if !owned[i] || actualIDs[i] == 0 {
			continue
		}
		if err := a.validateJumpChain(actualIDs[i], make(map[int64]bool)); err != nil {
			profile, ok := a.profileByID(actualIDs[i])
			if !ok {
				continue
			}
			profile.JumpProfileID = 0
			if err := a.store.SaveProfile(a.ctx, &profile); err == nil {
				summary.Warnings++
				a.refreshProfiles()
			}
		}
	}

	commands, _ := a.store.QuickCommands(a.ctx)
	commandByName := make(map[string]store.QuickCommand, len(commands))
	commandNames := make(map[string]bool, len(commands))
	for _, command := range commands {
		key := importedCommandKey(command.ProfileID, command.Name)
		commandByName[key] = command
		commandNames[key] = true
	}
	for _, imported := range a.backupPreview.QuickCommands {
		if strings.TrimSpace(imported.Name) == "" || strings.TrimSpace(imported.Command) == "" {
			summary.Skipped++
			continue
		}
		profileID := int64(0)
		if imported.Profile >= 0 {
			if imported.Profile >= len(actualIDs) || !owned[imported.Profile] || actualIDs[imported.Profile] == 0 {
				summary.Skipped++
				continue
			}
			profileID = actualIDs[imported.Profile]
		}
		key := importedCommandKey(profileID, imported.Name)
		existing, found := commandByName[key]
		if found && policy == backupConflictSkip {
			summary.Skipped++
			continue
		}
		command := store.QuickCommand{ProfileID: profileID, Name: imported.Name, Command: imported.Command}
		if found && policy == backupConflictOverwrite {
			command.ID = existing.ID
		} else if found || commandNames[key] {
			command.Name = uniqueImportedCommandName(imported.Name, profileID, commandNames)
		}
		if err := a.store.SaveQuickCommand(a.ctx, &command); err != nil {
			summary.Failed++
			continue
		}
		summary.Commands++
		commandNames[importedCommandKey(profileID, command.Name)] = true
		commandByName[importedCommandKey(profileID, command.Name)] = command
	}

	existingForwards := make(map[int64][]store.ForwardRule)
	for _, imported := range a.backupPreview.Forwards {
		if imported.Profile < 0 || imported.Profile >= len(actualIDs) ||
			!owned[imported.Profile] || actualIDs[imported.Profile] == 0 {
			summary.Skipped++
			continue
		}
		profileID := actualIDs[imported.Profile]
		rules, ok := existingForwards[profileID]
		if !ok {
			rules, _ = a.store.Forwards(a.ctx, profileID)
			existingForwards[profileID] = rules
		}
		rule := store.ForwardRule{
			ProfileID: profileID,
			Kind:      imported.Kind,
			Listen:    imported.Listen,
			Target:    imported.Target,
			AutoStart: imported.AutoStart,
		}
		for _, existing := range rules {
			if existing.Kind != rule.Kind || existing.Listen != rule.Listen {
				continue
			}
			if policy != backupConflictOverwrite {
				rule.ID = -1
			} else {
				rule.ID = existing.ID
			}
			break
		}
		if rule.ID == -1 {
			summary.Skipped++
			continue
		}
		if err := a.store.SaveForward(a.ctx, &rule); err != nil {
			summary.Failed++
			continue
		}
		summary.Forwards++
	}

	if a.backupIncludeSettings && a.backupSource == "Xu 备份" {
		keys := make([]string, 0, len(a.backupPreview.Settings))
		for key := range a.backupPreview.Settings {
			if backupSettingAllowed(key) {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := a.store.SetSetting(a.ctx, key, a.backupPreview.Settings[key]); err != nil {
				summary.Failed++
				continue
			}
			summary.Settings++
		}
		if summary.Settings > 0 {
			a.loadSettings()
			a.applyLogHighlight()
			a.applyTermSettings()
			a.ApplyThemeSource()
		}
	}

	a.refreshGroups()
	a.refreshProfiles()
	a.loadQuickCommands()
	notice := summary.notice()
	if summary.Failed > 0 {
		a.managerErr = fmt.Sprintf("导入完成，但 %d 项失败", summary.Failed)
	} else {
		a.managerErr = ""
	}
	a.managerNotice = notice
	a.status = notice
	return summary
}

func nameForImportedGroup(backup xuBackup, lower string) string {
	for _, group := range backup.Groups {
		if strings.ToLower(strings.TrimSpace(group.Name)) == lower {
			return strings.TrimSpace(group.Name)
		}
	}
	for _, profile := range backup.Profiles {
		if strings.ToLower(strings.TrimSpace(profile.Group)) == lower {
			return strings.TrimSpace(profile.Group)
		}
	}
	return lower
}

func uniqueBackupName(base string, used map[string]bool) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "导入连接"
	}
	candidate := base + " (导入)"
	for n := 2; used[strings.ToLower(candidate)]; n++ {
		candidate = fmt.Sprintf("%s (导入 %d)", base, n)
	}
	return candidate
}

func importedCommandKey(profileID int64, name string) string {
	return fmt.Sprintf("%d\x00%s", profileID, strings.ToLower(strings.TrimSpace(name)))
}

func uniqueImportedCommandName(base string, profileID int64, used map[string]bool) string {
	candidate := base + " (导入)"
	for n := 2; used[importedCommandKey(profileID, candidate)]; n++ {
		candidate = fmt.Sprintf("%s (导入 %d)", base, n)
	}
	return candidate
}

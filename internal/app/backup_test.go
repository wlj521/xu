package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

func newBackupTestApp(t *testing.T) (*App, *store.Store) {
	t.Helper()
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	st, err := store.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a := New()
	a.SetStore(st)
	t.Cleanup(a.Close)
	return a, st
}

func TestBuildBackupExportsRelationships(t *testing.T) {
	_, st := newBackupTestApp(t)
	ctx := context.Background()

	group := store.Group{Name: "生产"}
	if err := st.SaveGroup(ctx, &group); err != nil {
		t.Fatal(err)
	}
	jump := store.Profile{Name: "bastion", Host: "bastion.example.com", Port: 22, Username: "ops", AuthType: "key"}
	if err := st.SaveProfile(ctx, &jump); err != nil {
		t.Fatal(err)
	}
	app := store.Profile{
		Name: "app", Host: "app.example.com", Port: 2222, Username: "deploy",
		AuthType: "key", KeyPath: "/keys/app", GroupID: group.ID, JumpProfileID: jump.ID,
	}
	if err := st.SaveProfile(ctx, &app); err != nil {
		t.Fatal(err)
	}
	for _, command := range []*store.QuickCommand{
		{Name: "uptime", Command: "uptime"},
		{ProfileID: app.ID, Name: "logs", Command: "tail -f app.log"},
	} {
		if err := st.SaveQuickCommand(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SaveForward(ctx, &store.ForwardRule{
		ProfileID: app.ID, Kind: 0, Listen: "127.0.0.1:8080", Target: "127.0.0.1:80", AutoStart: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(ctx, "terminal.font.size", "16"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(ctx, workspaceSettingKey, `{"tabs":[1],"active":0}`); err != nil {
		t.Fatal(err)
	}

	backup, err := buildXuBackup(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if backup.Format != xuBackupFormat || backup.Version != xuBackupVersion {
		t.Fatalf("backup header = %+v", backup)
	}
	if len(backup.Groups) != 1 || backup.Groups[0].Name != "生产" {
		t.Fatalf("backup groups = %+v", backup.Groups)
	}
	if len(backup.Profiles) != 2 {
		t.Fatalf("backup profiles = %+v", backup.Profiles)
	}
	byName := make(map[string]backupProfile)
	for _, profile := range backup.Profiles {
		byName[profile.Name] = profile
	}
	appProfile := byName["app"]
	if appProfile.Group != "生产" || appProfile.KeyPath != "/keys/app" || appProfile.JumpProfile < 0 {
		t.Fatalf("app backup = %+v", appProfile)
	}
	if backup.Profiles[appProfile.JumpProfile].Name != "bastion" {
		t.Fatalf("jump reference = %+v", backup.Profiles[appProfile.JumpProfile])
	}
	if len(backup.QuickCommands) != 2 {
		t.Fatalf("backup commands = %+v", backup.QuickCommands)
	}
	for _, command := range backup.QuickCommands {
		if command.Name == "logs" && (command.Profile < 0 || backup.Profiles[command.Profile].Name != "app") {
			t.Fatalf("profile command = %+v", command)
		}
		if command.Name == "uptime" && command.Profile != -1 {
			t.Fatalf("global command = %+v", command)
		}
	}
	if len(backup.Forwards) != 1 || backup.Profiles[backup.Forwards[0].Profile].Name != "app" {
		t.Fatalf("backup forwards = %+v", backup.Forwards)
	}
	if backup.Settings["terminal.font.size"] != "16" {
		t.Fatalf("backup settings = %+v", backup.Settings)
	}
	if _, ok := backup.Settings[workspaceSettingKey]; ok {
		t.Fatal("workspace tabs leaked into the backup")
	}
}

func TestImportBackupConflictPolicies(t *testing.T) {
	cases := []struct {
		name       string
		policy     string
		wantCount  int
		wantHost   string
		wantShared string
		wantSkip   int
	}{
		{name: "skip", policy: "跳过", wantCount: 2, wantHost: "old.example.com", wantShared: "shared", wantSkip: 3},
		{name: "overwrite", policy: "覆盖", wantCount: 2, wantHost: "new.example.com", wantShared: "shared"},
		{name: "rename", policy: "重命名", wantCount: 3, wantHost: "new.example.com", wantShared: "shared (导入)"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			a, st := newBackupTestApp(t)
			ctx := context.Background()
			existing := store.Profile{
				Name: "shared", Host: "old.example.com", Port: 22, Username: "root", AuthType: "key",
			}
			if err := st.SaveProfile(ctx, &existing); err != nil {
				t.Fatal(err)
			}
			a.refreshProfiles()

			a.backupPreview = xuBackup{
				Format: xuBackupFormat, Version: xuBackupVersion,
				Groups: []backupGroup{{Name: "Imported"}},
				Profiles: []backupProfile{
					{
						Name: "shared", Host: "new.example.com", Port: 22, Username: "root",
						AuthType: "key", KeyPath: "/new/key", Group: "Imported", JumpProfile: -1,
					},
					{
						Name: "other", Host: "other.example.com", Port: 22, Username: "deploy",
						AuthType: "key", Group: "Imported", JumpProfile: 0,
					},
				},
				QuickCommands: []backupQuickCommand{
					{Profile: -1, Name: "global", Command: "pwd"},
					{Profile: 0, Name: "deploy", Command: "make release"},
				},
				Forwards: []backupForward{
					{Profile: 0, Kind: 0, Listen: "127.0.0.1:9000", Target: "127.0.0.1:80", AutoStart: true},
				},
			}
			a.backupSource = "Xu 备份"
			a.backupConflictPick = tc.policy
			sum := a.applyBackupPreview()

			profiles, err := st.Profiles(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(profiles) != tc.wantCount {
				t.Fatalf("profiles = %+v, want %d", profiles, tc.wantCount)
			}
			var shared, other store.Profile
			for _, profile := range profiles {
				switch profile.Name {
				case "shared":
					shared = profile
				case "shared (导入)":
					shared = profile
				case "other":
					other = profile
				}
			}
			if shared.Name != tc.wantShared || shared.Host != tc.wantHost {
				t.Fatalf("shared profile = %+v", shared)
			}
			if tc.name == "rename" {
				foundOriginal := false
				for _, profile := range profiles {
					if profile.Name == "shared" && profile.Host == "old.example.com" {
						foundOriginal = true
					}
				}
				if !foundOriginal {
					t.Fatalf("rename policy changed the original profile: %+v", profiles)
				}
			}
			if tc.wantSkip > 0 && sum.Skipped != tc.wantSkip {
				t.Fatalf("summary = %+v, want skipped %d", sum, tc.wantSkip)
			}
			if other.JumpProfileID != shared.ID {
				t.Fatalf("other jump = %d, want shared ID %d", other.JumpProfileID, shared.ID)
			}
			commands, err := st.QuickCommands(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var global, deploy bool
			for _, command := range commands {
				if command.ProfileID == 0 && command.Name == "global" {
					global = true
				}
				if command.ProfileID == shared.ID && command.Name == "deploy" {
					deploy = true
				}
			}
			if !global {
				t.Fatalf("global command missing: %+v", commands)
			}
			if tc.name == "skip" {
				if deploy {
					t.Fatalf("a skipped profile imported its command: %+v", commands)
				}
			} else if !deploy {
				t.Fatalf("profile command missing: %+v", commands)
			}
			rules, err := st.Forwards(ctx, shared.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "skip" {
				if len(rules) != 0 {
					t.Fatalf("a skipped profile imported forwards: %+v", rules)
				}
			} else if len(rules) != 1 || !rules[0].AutoStart {
				t.Fatalf("imported forwards = %+v", rules)
			}
			groups, err := st.Groups(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(groups) != 1 || groups[0].Name != "Imported" {
				t.Fatalf("groups = %+v", groups)
			}
		})
	}
}

func TestParseOpenSSHConfig(t *testing.T) {
	config := `
# global defaults
Host bastion
    HostName bastion.example.com
    User ops

Host prod
    HostName 10.0.0.10
    User deploy
    Port 2222
    IdentityFile "~/.ssh/id_prod"
    ProxyJump ops@bastion:22

Host db db-alias
    HostName db.example.com
    User root

Host *
    ServerAliveInterval 30
`
	backup, warnings, err := parseOpenSSHConfig([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	if len(backup.Profiles) != 4 {
		t.Fatalf("profiles = %+v, want bastion, prod, db and db-alias", backup.Profiles)
	}
	byName := make(map[string]backupProfile)
	for _, profile := range backup.Profiles {
		byName[profile.Name] = profile
	}
	prod := byName["prod"]
	if prod.Host != "10.0.0.10" || prod.Username != "deploy" || prod.Port != 2222 ||
		prod.AuthType != "key" || prod.KeyPath != expandKeyPath("~/.ssh/id_prod") {
		t.Fatalf("prod = %+v", prod)
	}
	if prod.JumpProfile < 0 || backup.Profiles[prod.JumpProfile].Name != "bastion" {
		t.Fatalf("prod jump = %+v", prod)
	}
	db := byName["db"]
	if db.Host != "db.example.com" || db.Username != "root" || db.Port != 22 || db.AuthType != "password" {
		t.Fatalf("db = %+v", db)
	}
	if byName["db-alias"].Host != "db.example.com" {
		t.Fatalf("db alias = %+v", byName["db-alias"])
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "通配") {
		t.Fatalf("warnings = %v, want only the wildcard notice", warnings)
	}
}

func TestOpenBackupImporterPreviewsOpenSSHConfig(t *testing.T) {
	a, _ := newBackupTestApp(t)
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("Host prod\n HostName prod.example.com\n User root\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldOpen := openBackupFile
	t.Cleanup(func() { openBackupFile = oldOpen })
	openBackupFile = func(mygo.OpenDialogOptions) ([]string, error) {
		return []string{path}, nil
	}

	a.openBackupImporter()
	if !a.backupOpen || a.backupSource != "OpenSSH config" {
		t.Fatalf("preview = open:%v source:%q", a.backupOpen, a.backupSource)
	}
	if len(a.backupPreview.Profiles) != 1 || a.backupPreview.Profiles[0].Name != "prod" {
		t.Fatalf("preview profiles = %+v", a.backupPreview.Profiles)
	}
	if a.backupConflictPick != "跳过" || a.backupIncludeSettings {
		t.Fatalf("preview policy = %q includeSettings=%v", a.backupConflictPick, a.backupIncludeSettings)
	}
}

func TestBackupModalRendersPreview(t *testing.T) {
	a := New()
	a.backupOpen = true
	a.backupSource = "Xu 备份"
	a.backupPath = filepath.Join("backup", "xu-backup.json")
	a.backupConflictPick = "跳过"
	a.backupPreview = xuBackup{
		Profiles: []backupProfile{
			{Name: "prod", Host: "prod.example.com", Port: 22, Username: "root", Group: "生产"},
			{Name: "db", Host: "db.example.com", Port: 2222, Username: "deploy"},
		},
		Groups:        []backupGroup{{Name: "生产"}},
		QuickCommands: []backupQuickCommand{{Profile: -1, Name: "pwd", Command: "pwd"}},
		Settings:      map[string]string{"terminal.font.size": "14"},
	}

	tt := ui.NewTester(func(c *ui.Context) {
		theme := a.applyTheme(c)
		a.backupModal(c, theme)
	}, 1024, 720)
	tt.Frame()
	for _, want := range []string{
		"导入 Xu 备份", "同名或同主机连接", "prod", "db.example.com", "确认来源可信", "开始导入",
		"备份不包含密码", "同时恢复应用设置",
	} {
		if !tt.HasText(want) {
			t.Fatalf("backup modal does not show %q; texts = %q", want, tt.Texts())
		}
	}
}

func TestExportBackupWritesJSONWithoutWorkspace(t *testing.T) {
	a, st := newBackupTestApp(t)
	ctx := context.Background()
	profile := store.Profile{Name: "prod", Host: "prod.example.com", Port: 22, Username: "root", AuthType: "key"}
	if err := st.SaveProfile(ctx, &profile); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(ctx, "terminal.font.size", "15"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(ctx, workspaceSettingKey, `{"tabs":[1],"active":0}`); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "backup.json")
	oldSave := saveBackupFile
	t.Cleanup(func() { saveBackupFile = oldSave })
	saveBackupFile = func(mygo.SaveDialogOptions) (string, error) { return path, nil }

	a.exportBackup()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), workspaceSettingKey) {
		t.Fatalf("workspace state leaked into backup: %s", data)
	}
	var backup xuBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		t.Fatal(err)
	}
	if len(backup.Profiles) != 1 || backup.Settings["terminal.font.size"] != "15" {
		t.Fatalf("exported backup = %+v", backup)
	}
	if !strings.Contains(a.managerNotice, "已导出 1 个连接") {
		t.Fatalf("export notice = %q", a.managerNotice)
	}
}

func TestImportBackupSettings(t *testing.T) {
	a, st := newBackupTestApp(t)
	a.backupSource = "Xu 备份"
	a.backupConflictPick = "跳过"
	a.backupIncludeSettings = true
	a.backupPreview = xuBackup{
		Profiles: []backupProfile{{
			Name: "new", Host: "new.example.com", Port: 22, Username: "root", AuthType: "key",
		}},
		Settings: map[string]string{
			"terminal.font.size":     "17",
			"terminal.scrollback_mb": "24",
			workspaceSettingKey:      `{"tabs":[1],"active":0}`,
		},
	}

	sum := a.applyBackupPreview()
	if sum.Created != 1 || sum.Settings != 2 {
		t.Fatalf("summary = %+v", sum)
	}
	if a.settings.FontSize != 17 || a.settings.ScrollbackMB != 24 {
		t.Fatalf("loaded settings = %+v", a.settings)
	}
	values, err := st.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if values[workspaceSettingKey] != "" {
		t.Fatalf("workspace tabs were imported: %q", values[workspaceSettingKey])
	}
}

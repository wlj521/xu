package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/store"
)

// Public test vector from the community FinalShellDecodePass tool.
const finalShellCipher = "UU8hWV51DmVNgmX/pUd0LlaEo53VTa6s"
const finalShellPlain = "beac3d85988e"

func TestDecodeFinalShellPassword(t *testing.T) {
	got, err := decodeFinalShellPassword(finalShellCipher)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != finalShellPlain {
		t.Fatalf("decoded %d bytes, want the known test vector", len(got))
	}
}

func TestParseFinalShellExport(t *testing.T) {
	obj := []byte(`{"name":"办公室","host":"192.168.0.102","port":22,"user_name":"root",
		"password":"` + finalShellCipher + `","authentication_type":1,"conection_type":100}`)
	conns, err := parseFinalShellExport(obj)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 1 || conns[0].Name != "办公室" || conns[0].Host != "192.168.0.102" ||
		conns[0].Port != 22 || conns[0].UserName != "root" || conns[0].ConnType != 100 {
		t.Fatalf("object parse = %+v", conns)
	}

	arr, err := parseFinalShellExport([]byte(`[` + string(obj) + `,{"host":"10.0.0.1","user_name":"ops","conection_type":100}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(arr) != 2 || arr[1].Host != "10.0.0.1" || arr[1].UserName != "ops" {
		t.Fatalf("array parse = %+v", arr)
	}

	if _, err := parseFinalShellExport([]byte(`{"host":`)); err == nil {
		t.Fatal("invalid JSON should fail")
	}
}

func TestImportFinalShellConnections(t *testing.T) {
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	st, err := store.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := New()
	a.SetStore(st)

	dir := t.TempDir()
	one := filepath.Join(dir, "办公室_connect_config.json")
	if err := os.WriteFile(one, []byte(`{
		"name":"办公室导入","host":"192.168.0.102","port":22,"user_name":"root",
		"password":"`+finalShellCipher+`","authentication_type":1,"conection_type":100
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	two := filepath.Join(dir, "more_connect_config.json")
	if err := os.WriteFile(two, []byte(`[
		{"name":"重复","host":"192.168.0.102","port":22,"user_name":"root","conection_type":100},
		{"name":"另一台","host":"10.1.2.3","port":2222,"user_name":"deploy","conection_type":100},
		{"name":"非 SSH","host":"10.1.2.4","port":3389,"user_name":"admin","conection_type":101}
	]`), 0o600); err != nil {
		t.Fatal(err)
	}

	oldPassword, oldOpen := setImportedPassword, openFinalShellImport
	t.Cleanup(func() {
		setImportedPassword, openFinalShellImport = oldPassword, oldOpen
	})
	var saved []string
	setImportedPassword = func(id int64, kind, value string) error {
		if kind != "password" || id == 0 {
			t.Fatalf("password saved as id=%d kind=%q", id, kind)
		}
		saved = append(saved, value)
		return nil
	}
	openFinalShellImport = func(opts mygo.OpenDialogOptions) ([]string, error) {
		if !opts.Multiple {
			t.Fatal("FinalShell imports must allow selecting several files")
		}
		return []string{one, two}, nil
	}

	a.importFinalShell()

	if len(a.profiles) != 2 {
		t.Fatalf("profiles = %+v, want the two importable connections", a.profiles)
	}
	got := map[string]store.Profile{}
	for _, p := range a.profiles {
		got[p.Host] = p
	}
	if p := got["192.168.0.102"]; p.Name != "办公室导入" || p.AuthType != "password" || p.Port != 22 {
		t.Fatalf("office profile = %+v", p)
	}
	if p := got["10.1.2.3"]; p.Name != "另一台" || p.AuthType != "key" || p.Username != "deploy" || p.Port != 2222 {
		t.Fatalf("second profile = %+v", p)
	}
	if len(saved) != 1 {
		t.Fatalf("saved %d passwords, want the one decryptable export", len(saved))
	}
	if saved[0] != finalShellPlain {
		t.Fatalf("saved password does not match the known test vector")
	}
	if !strings.Contains(a.managerNotice, "已导入 2 个连接") ||
		!strings.Contains(a.managerNotice, "跳过 2 个") {
		t.Fatalf("notice = %q", a.managerNotice)
	}
}

func TestConnectionManagerHasImportAction(t *testing.T) {
	a := New()
	a.managerOpen = true
	tt := ui.NewTester(a.View, 1440, 900)
	tt.Frame()
	if !tt.HasText("导入") {
		t.Fatalf("connection manager has no import action; texts = %q", tt.Texts())
	}
}

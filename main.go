package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"xu/internal/app"
	"xu/internal/bundle"
	"xu/internal/store"
)

func main() {
	// The portable build embeds its native DLLs; this unpacks them and points
	// the SQLite and terminal plugins at them before any plugin loads.
	if err := bundle.Prepare(); err != nil {
		log.Printf("native libraries: %v", err)
	}

	// Keep all app data (database, window state, keys, updater files) in
	// ~/.xu instead of the platform config directories or ~/.ssh.
	if home, err := os.UserHomeDir(); err == nil {
		dataDir := filepath.Join(home, ".xu")
		mygo.App.SetPath(mygo.PathUserData, dataDir)

		keysDir := filepath.Join(dataDir, "keys")
		_ = os.MkdirAll(keysDir, 0o700)
		readme := filepath.Join(keysDir, "README.txt")
		if _, err := os.Stat(readme); os.IsNotExist(err) {
			_ = os.WriteFile(readme, []byte(
				"连接配置可直接选择任意 OpenSSH 私钥；放在这里的 id_ed25519、id_ecdsa 或 id_rsa 会作为默认回退。\n"+
					"加密私钥可在连接时填写口令，口令只保留在内存中，不会写入数据库或日志。\n",
			), 0o600)
		}
	}

	a := app.New()

	mygo.App.WhenReady(func() {
		if st, err := store.Open(context.Background()); err != nil {
			a.SetStatus("会话库初始化失败: " + err.Error())
		} else {
			a.SetStore(st)
			go a.AutoCheckUpdates()
		}

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:          "Xu",
			Width:          1440,
			Height:         900,
			MinWidth:       1280,
			MinHeight:      720,
			StateKey:       "main",
			TitleBarStyle:  mygo.TitleBarHidden,
			TitleBarHeight: 32,
			Content:        ui.View(a.View),
		})
		a.Attach(win)
		a.ApplyThemeSource()
		mygo.Theme.OnUpdated(func() { a.OnSystemAppearanceChanged() })
	})

	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.Close() })

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

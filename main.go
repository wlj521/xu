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
				"把 OpenSSH 格式的私钥放在这里：id_ed25519、id_ecdsa 或 id_rsa。\n"+
					"当前版本不支持带口令的私钥。\n",
			), 0o600)
		}
	}

	a := app.New()

	mygo.App.WhenReady(func() {
		if st, err := store.Open(context.Background()); err != nil {
			a.SetStatus("会话库初始化失败: " + err.Error())
		} else {
			a.SetStore(st)
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
		a.OpenLocalTerminal()
	})

	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.Close() })

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

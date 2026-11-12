//go:build portable

// Package bundle prepares the native libraries an unpackaged build loads
// from beside its executable. This portable variant embeds them in the
// executable and unpacks them into the user's cache before the plugins load
// them.
package bundle

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// The build script fills this directory with the two DLLs before compiling
// with -tags=portable, then removes them again.
//
//go:embed assets/ghostty-vt.dll
var ghosttyDLL []byte

//go:embed assets/Sqlitecore.dll
var sqliteDLL []byte

// Prepare unpacks the embedded DLLs and points the terminal and SQLite
// plugins at them. It runs before mygo.App.Run, so their libraries are found
// before anything can load them.
func Prepare() error {
	dir, err := runtimeDir()
	if err != nil {
		return err
	}
	ghostty, err := unpack(dir, "ghostty-vt.dll", ghosttyDLL)
	if err != nil {
		return err
	}
	sqlite, err := unpack(dir, "Sqlitecore.dll", sqliteDLL)
	if err != nil {
		return err
	}
	_ = os.Setenv("MYGO_GHOSTTY_VT", ghostty)
	_ = os.Setenv("MYGO_NATIVE_SQLITECORE", sqlite)
	return nil
}

// runtimeDir is a per-build directory in the user's cache, so a new build
// does not overwrite DLLs that a running process still has open.
func runtimeDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("portable runtime: %w", err)
	}
	sum := sha256.Sum256(append(append([]byte{}, ghosttyDLL...), sqliteDLL...))
	dir := filepath.Join(cache, "Xu", "runtime", hex.EncodeToString(sum[:8]))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("portable runtime: %w", err)
	}
	return dir, nil
}

// unpack writes data to dir/name once, and returns its path. A file already
// there with the expected size is reused.
func unpack(dir, name string, data []byte) (string, error) {
	path := filepath.Join(dir, name)
	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(data)) {
		return path, nil
	}
	tmp, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("portable runtime: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("portable runtime: write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("portable runtime: close %s: %w", name, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", fmt.Errorf("portable runtime: install %s: %w", name, err)
	}
	return path, nil
}

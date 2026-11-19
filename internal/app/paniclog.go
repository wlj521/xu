package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

// logPanic leaves a stack in ~/.xu/logs when a background task panics. The
// task itself reports a normal error, so one bad delete cannot take the whole
// window down with it.
func logPanic(scope string, recovered any) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".xu", "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "panic.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "[%s] %s: %v\n%s\n",
		time.Now().Format(time.RFC3339), scope, recovered, debug.Stack())
}

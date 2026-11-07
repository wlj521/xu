package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"xu/internal/app"
)

func TestConnectionForm(t *testing.T) {
	a := app.New()
	tt := ui.NewTester(a.View, 1280, 800)
	tt.Frame()

	if !tt.HasText("还没有打开的会话") {
		t.Fatalf("texts = %q", tt.Texts())
	}

	if err := tt.Click("新建连接"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("新建 SSH 连接") || !tt.HasText("名称") {
		t.Fatalf("connection modal missing; texts = %q", tt.Texts())
	}
}

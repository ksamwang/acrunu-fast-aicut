//go:build windows

package ffmpeg

import (
	"context"
	"testing"
)

func TestNewCommandContextHidesWindowsConsole(t *testing.T) {
	cmd := newCommandContext(context.Background(), "cmd.exe", "/c", "exit", "0")
	if cmd.SysProcAttr == nil {
		t.Fatal("expected Windows process attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("expected child process window to be hidden")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatal("expected CREATE_NO_WINDOW creation flag")
	}
}

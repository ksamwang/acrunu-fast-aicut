package ffmpeg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBinaryPathPrefersEnvVar(t *testing.T) {
	t.Setenv("FFPROBE_PATH", `C:\custom\ffprobe.exe`)
	if got := resolveBinaryPath("FFPROBE_PATH", "ffprobe"); got != `C:\custom\ffprobe.exe` {
		t.Fatalf("expected env path, got %q", got)
	}
}

func TestFindBundledBinaryFindsWindowsToolsLayout(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, ".tools", "ffmpeg", "windows-x64", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	target := filepath.Join(binDir, executableName("ffprobe"))
	if err := os.WriteFile(target, []byte("stub"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got := findBundledBinary(root, "ffprobe")
	if got != target {
		t.Fatalf("expected bundled path %q, got %q", target, got)
	}
}

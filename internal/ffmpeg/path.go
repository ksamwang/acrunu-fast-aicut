package ffmpeg

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func ffmpegPath() string {
	return resolveBinaryPath("FFMPEG_PATH", "ffmpeg")
}

func ffprobePath() string {
	return resolveBinaryPath("FFPROBE_PATH", "ffprobe")
}

func resolveBinaryPath(envKey string, baseName string) string {
	if path := os.Getenv(envKey); path != "" {
		return path
	}

	for _, root := range binarySearchRoots() {
		if path := findBundledBinary(root, baseName); path != "" {
			return path
		}
	}

	return executableName(baseName)
}

// BinariesReady reports whether both binaries required by local preprocessing
// can be resolved without starting an FFmpeg process.
func BinariesReady() (ffmpegReady bool, ffprobeReady bool) {
	return binaryExists(ffmpegPath()), binaryExists(ffprobePath())
}

func binarySearchRoots() []string {
	roots := make([]string, 0, 2)
	if executablePath, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(executablePath))
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	return roots
}

func binaryExists(path string) bool {
	if filepath.IsAbs(path) || strings.ContainsAny(path, `/\\`) {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(path)
	return err == nil
}

func findBundledBinary(startDir string, baseName string) string {
	execName := executableName(baseName)
	for _, dir := range candidateRoots(startDir) {
		for _, candidate := range []string{
			filepath.Join(dir, "ffmpeg", "bin", execName),
			filepath.Join(dir, ".tools", "ffmpeg", platformDirName(), "bin", execName),
			filepath.Join(dir, ".tools", "ffmpeg", "bin", execName),
			filepath.Join(dir, ".tools", "ffmpeg", "ffmpeg-8.1.1-essentials_build", "bin", execName),
		} {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	return ""
}

func candidateRoots(startDir string) []string {
	roots := []string{}
	current := startDir
	for {
		roots = append(roots, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return roots
}

func executableName(baseName string) string {
	if runtime.GOOS == "windows" {
		return baseName + ".exe"
	}
	return baseName
}

func platformDirName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows-x64"
	case "darwin":
		return "darwin"
	default:
		return "linux"
	}
}

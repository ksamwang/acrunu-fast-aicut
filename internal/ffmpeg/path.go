package ffmpeg

import (
	"os"
	"path/filepath"
	"runtime"
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

	if cwd, err := os.Getwd(); err == nil {
		if path := findBundledBinary(cwd, baseName); path != "" {
			return path
		}
	}

	return executableName(baseName)
}

func findBundledBinary(startDir string, baseName string) string {
	execName := executableName(baseName)
	for _, dir := range candidateRoots(startDir) {
		for _, candidate := range []string{
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

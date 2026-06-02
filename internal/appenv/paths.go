package appenv

import (
	"os"
	"path/filepath"
	"strings"
)

const appDirName = "nasnotify"

func DataDir() string {
	return firstWritableDir(dataDirCandidates())
}

func LogDir() string {
	if dir := strings.TrimSpace(os.Getenv("UGAPP_LOG_DIR")); dir != "" {
		return firstWritableDir([]string{dir})
	}
	return firstWritableDir(logDirCandidates())
}

func dataDirCandidates() []string {
	candidates := []string{}
	if dir := strings.TrimSpace(os.Getenv("UGAPP_DATA_DIR")); dir != "" {
		candidates = append(candidates, dir)
	}
	if dir := executableRootDir(); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "data"))
	}
	candidates = append(candidates, "data", "/data")

	if dir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(dir) != "" {
		candidates = append(candidates, filepath.Join(dir, appDirName))
	}
	if dir, err := os.UserHomeDir(); err == nil && strings.TrimSpace(dir) != "" {
		candidates = append(candidates, filepath.Join(dir, "."+appDirName))
	}

	candidates = append(candidates, filepath.Join(os.TempDir(), appDirName))
	return candidates
}

func logDirCandidates() []string {
	candidates := []string{}
	candidates = append(candidates, filepath.Join(DataDir(), "log"))
	if dir := executableRootDir(); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "log"))
	}
	candidates = append(candidates, "log", "/log", filepath.Join(os.TempDir(), appDirName, "log"))
	return candidates
}

func firstWritableDir(candidates []string) string {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if err := os.MkdirAll(candidate, 0o700); err != nil {
			continue
		}
		probe, err := os.CreateTemp(candidate, ".write-test-*")
		if err != nil {
			continue
		}
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		return candidate
	}
	return filepath.Join(os.TempDir(), appDirName)
}

func executableRootDir() string {
	exe, err := os.Executable()
	if err != nil || strings.TrimSpace(exe) == "" {
		return ""
	}
	return filepath.Clean(filepath.Join(filepath.Dir(exe), ".."))
}

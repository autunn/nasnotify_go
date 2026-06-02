package appenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogDirDoesNotCreateConfigDirectory(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "log")
	t.Setenv("UGAPP_LOG_DIR", logDir)
	t.Setenv("UGAPP_DATA_DIR", filepath.Join(t.TempDir(), "data"))

	if got := LogDir(); got != logDir {
		t.Fatalf("LogDir() = %q; want %q", got, logDir)
	}
	if _, err := os.Stat(filepath.Join(logDir, "config")); !os.IsNotExist(err) {
		t.Fatalf("LogDir() should not create config directory in log dir, stat err = %v", err)
	}
}

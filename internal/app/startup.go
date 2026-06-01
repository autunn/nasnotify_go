package app

import (
	"fmt"
	"log"
	"os"
	"time"
)

const startupTracePath = "/tmp/nasnotify-startup.log"

func recordStartupEvent(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %s\n", time.Now().Format(time.RFC3339), message)
	if f, err := os.OpenFile(startupTracePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666); err == nil {
		_, _ = f.WriteString(line)
		_ = f.Close()
	}
	log.Print(message)
}

func fatalStartup(format string, args ...any) {
	recordStartupEvent(format, args...)
	log.Fatalf(format, args...)
}

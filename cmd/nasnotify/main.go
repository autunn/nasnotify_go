package main

import (
	"flag"
	"log"

	"nasnotify-go/internal/app"
)

var Version = "v2026.06.01"

func main() {
	port := flag.Int("port", app.DefaultHTTPListenPortForTest, "HTTP port to listen on")
	flag.Parse()

	if err := app.New(Version, *port).Run(); err != nil {
		log.Fatalf("NasNotify %s failed: %v", Version, err)
	}
}

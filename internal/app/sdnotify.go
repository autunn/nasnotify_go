package app

import (
	"log"
	"net"
	"os"
	"strings"
)

func notifyServiceManagerReady() {
	if err := sdNotify("READY=1"); err != nil {
		log.Printf("systemd notify READY=1 failed: %v", err)
	}
}

func sdNotify(state string) error {
	socketPath := strings.TrimSpace(os.Getenv("NOTIFY_SOCKET"))
	if socketPath == "" {
		return nil
	}

	addr := &net.UnixAddr{Name: socketPath, Net: "unixgram"}
	if strings.HasPrefix(socketPath, "@") {
		addr.Name = "\x00" + strings.TrimPrefix(socketPath, "@")
	}

	conn, err := net.DialUnix("unixgram", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write([]byte(state))
	return err
}

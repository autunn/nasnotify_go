//go:build linux

package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

type DesktopIntegration struct {
	runtime *RuntimeHost
	conn    *dbus.Conn
}

type supportModulesPayload struct {
	Modules []string `json:"Modules"`
}

func NewDesktopIntegration(runtimeHost *RuntimeHost) *DesktopIntegration {
	return &DesktopIntegration{runtime: runtimeHost}
}

func (d *DesktopIntegration) Start(ctx context.Context, wg *sync.WaitGroup) {
	if err := d.registerSupportModules(); err != nil {
		log.Printf("desktop integration dbus unavailable: %v", err)
	}
}

func (d *DesktopIntegration) Close() {
	if d.conn != nil {
		_ = d.conn.Close()
	}
}

func (d *DesktopIntegration) registerSupportModules() error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return err
	}

	reply, err := conn.RequestName(AppID, dbus.NameFlagDoNotQueue)
	if err != nil {
		_ = conn.Close()
		return err
	}

	if reply != dbus.RequestNameReplyPrimaryOwner && reply != dbus.RequestNameReplyAlreadyOwner {
		_ = conn.Close()
		return errors.New("dbus name is not available")
	}

	objectPath := dbus.ObjectPath("/" + strings.ReplaceAll(AppID, ".", "/"))
	if err := conn.Export(desktopSupportModulesService{}, objectPath, AppID); err != nil {
		_ = conn.Close()
		return err
	}

	d.conn = conn
	return nil
}

type desktopSupportModulesService struct{}

func (desktopSupportModulesService) SupportModules() ([]byte, *dbus.Error) {
	payload, err := json.Marshal(supportModulesPayload{
		Modules: []string{AppID},
	})
	if err != nil {
		return nil, dbus.MakeFailedError(err)
	}
	return payload, nil
}

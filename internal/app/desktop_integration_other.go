//go:build !linux

package app

import (
	"context"
	"sync"
)

type DesktopIntegration struct{}

func NewDesktopIntegration(_ *RuntimeHost) *DesktopIntegration {
	return &DesktopIntegration{}
}

func (d *DesktopIntegration) Start(_ context.Context, _ *sync.WaitGroup) {}

func (d *DesktopIntegration) Close() {}

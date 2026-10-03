//go:build linux

package main

import (
	"context"
	"log"
	"path"
	"strings"
	"time"

	systemd "github.com/coreos/go-systemd/v22/dbus"
	godbus "github.com/godbus/dbus/v5"
)

type deviceAllow struct {
	Path        string
	Permissions string
}

// systemd rebuilds the scope's device program from DeviceAllow on every daemon-reload,
// which drops a grant that only exists in BPF, so record the devices there as well.
func grantSystemdDevices(cgroupPath string, devicePaths []string) {
	unit := path.Base(cgroupPath)
	if !strings.HasSuffix(unit, ".scope") || len(devicePaths) == 0 {
		return
	}

	entries := make([]deviceAllow, 0, len(devicePaths))
	for _, devicePath := range devicePaths {
		entries = append(entries, deviceAllow{Path: devicePath, Permissions: "rwm"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := systemd.NewSystemConnectionContext(ctx)
	if err != nil {
		log.Printf("Warning: cannot connect to systemd, %s keeps a BPF-only grant that a daemon-reload will drop: %v\n", unit, err)
		return
	}
	defer conn.Close()

	property := systemd.Property{Name: "DeviceAllow", Value: godbus.MakeVariant(entries)}
	if err := conn.SetUnitPropertiesContext(ctx, unit, true, property); err != nil {
		log.Printf("Warning: cannot add DeviceAllow to %s, it keeps a BPF-only grant that a daemon-reload will drop: %v\n", unit, err)
		return
	}

	log.Printf("Added %d device(s) to DeviceAllow of %s\n", len(entries), unit)
}

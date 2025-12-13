//go:build linux

package main

// #include "ctypes.h"
import "C"
import (
	"context"
	"device-volume-driver/internal/cgroup"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	_ "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
)

// Version string, set at build time.
var Version = "development"

const pluginId = "dvd"
const rootPath = "/host"

func Ptr[T any](v T) *T {
	return &v
}

func main() {
	log.Printf("Starting Device Mapping Manager version %s\n", Version)
	cli, err := client.New(client.FromEnv)
	if err != nil {
		log.Fatal(err)
	}
	defer cli.Close()

	containers, err := cli.ContainerList(context.Background(), client.ContainerListOptions{})
	if err != nil {
		panic(err)
	}

	for _, container := range containers.Items {
		log.Printf("Checking existing container %s %s\n", container.ID[:10], container.Image)
		processContainer(cli, container.ID)
	}

	listenForMounts(cli)
}

func getDeviceInfo(devicePath string) (string, int64, int64, error) {
	var stat unix.Stat_t

	if err := unix.Stat(devicePath, &stat); err != nil {
		log.Println(err)
		return "", -1, -1, err
	}

	var deviceType string

	switch stat.Mode & unix.S_IFMT {
	case unix.S_IFBLK:
		deviceType = "b"
	case unix.S_IFCHR:
		deviceType = "c"
	default:
		log.Println("aborting: device is neither a character or block device")
		return "", -1, -1, fmt.Errorf("unsupported device type... aborting")
	}

	major := int64(unix.Major(stat.Rdev))
	minor := int64(unix.Minor(stat.Rdev))
	log.Printf("Found device: %s %s %d:%d\n", devicePath, deviceType, major, minor)

	return deviceType, major, minor, nil
}

func listenForMounts(cli *client.Client) {
	res := cli.Events(context.Background(), client.EventsListOptions{Filters: make(client.Filters).Add("event", "start")})

	for {
		select {
		case err := <-res.Err:
			log.Fatal(err)
		case msg := <-res.Messages:
			processContainer(cli, msg.Actor.ID)
		}
	}
}

func processContainer(cli *client.Client, id string) {
	info, err := cli.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
	if err != nil {
		panic(err)
	}

	pid := info.Container.State.Pid
	version, err := cgroup.GetDeviceCGroupVersion("/", pid)
	log.Printf("The cgroup version for process %d is: %v\n", pid, version)
	if err != nil {
		log.Println(err)
		return
	}

	log.Printf("Checking mounts for process %d\n", pid)
	processMounts(info.Container.Mounts, pid, id, version)
}

func processMounts(mounts []container.MountPoint, pid int, containerId string, version int) {
	api, err := cgroup.New(version)
	if err != nil {
		log.Println(err)
		return
	}

	cgroupPath, sysfsPath, err := api.GetDeviceCGroupMountPath("/", pid)
	if err != nil {
		log.Println(err)
		return
	}

	for _, mount := range mounts {
		log.Printf(
			"%s/%v requested a volume mount for %s at %s\n",
			containerId, pid, mount.Source, mount.Destination,
		)

		if !strings.HasPrefix(mount.Source, "/dev") {
			log.Printf("%s is not a device... skipping\n", mount.Source)
			continue
		}

		cgroupPath = path.Join(rootPath, sysfsPath, cgroupPath)
		log.Printf("The cgroup path for process %d is at %v\n", pid, cgroupPath)
		fileInfo, err := os.Stat(mount.Source)
		if err != nil {
			log.Println(err)
			continue
		}
		if fileInfo.IsDir() {
			err := filepath.Walk(mount.Source,
				func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if info.IsDir() {
						return nil
					}
					if err := applyDeviceRules(api, path, cgroupPath, pid); err != nil {
						log.Println(err)
					}
					return nil
				})
			if err != nil {
				log.Println(err)
			}
			continue
		}
		if err := applyDeviceRules(api, mount.Source, cgroupPath, pid); err != nil {
			log.Println(err)
		}
	}
}

func applyDeviceRules(api cgroup.Interface, mountPath string, cgroupPath string, pid int) error {
	deviceType, major, minor, err := getDeviceInfo(mountPath)

	if err != nil {
		log.Println(err)
		return err
	} else {
		log.Printf("Adding device rule for process %d at %s\n", pid, cgroupPath)
		err = api.AddDeviceRules(cgroupPath, []cgroup.DeviceRule{
			{
				Access: "rwm",
				Major:  Ptr[int64](major),
				Minor:  Ptr[int64](minor),
				Type:   deviceType,
				Allow:  true,
			},
		})

		if err != nil {
			log.Println(err)
			return err
		}
	}

	return nil
}

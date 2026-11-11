//go:build !linux && !windows

package local

import (
	"fmt"
	"os"

	"swcode/daemon/config"
)

const noService = "swcode has a service on Linux (systemd) and Windows (Task Scheduler) only."

// ServiceInstalled is false where swcode has no service.
func ServiceInstalled(Place) bool { return false }

// ServiceInstall says that this system has no service for swcode.
func ServiceInstall(config.Config, Place) int {
	fmt.Fprintln(os.Stderr, noService)
	return failed
}

// ServiceUninstall says that this system has no service for swcode.
func ServiceUninstall(config.Config, Place) int {
	fmt.Fprintln(os.Stderr, noService)
	return failed
}

// StartService has nothing to start where swcode has no service.
func StartService(Place) bool { return false }

// StopService has nothing to stop where swcode has no service.
func StopService(Place) bool { return false }

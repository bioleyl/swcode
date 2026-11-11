//go:build linux

package local

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"swcode/daemon/app"
	"swcode/daemon/config"
)

const (
	serviceUnitFile = serviceName + ".service"
	unitFolderMode  = 0o755
	unitFileMode    = 0o644
)

// unitFolder is where this person's systemd units live.
func unitFolder(place Place) string {
	configDir := place.Environment["XDG_CONFIG_HOME"]
	if configDir == "" {
		configDir = filepath.Join(place.Home, ".config")
	}
	return filepath.Join(configDir, "systemd", "user")
}

func unitPath(place Place) string { return filepath.Join(unitFolder(place), serviceUnitFile) }

// serviceUnit runs the daemon as a service: a restart ends the program with a failure, which starts it again, while
// stopping it through systemctl is a clean exit that keeps it stopped.
func serviceUnit(executable string) string {
	return strings.Join([]string{
		"[Unit]",
		"Description=" + serviceDescription,
		"",
		"[Service]",
		"Type=simple",
		"WorkingDirectory=%h",
		"ExecStart=" + execWord(executable) + " serve --service",
		"Restart=on-failure",
		"RestartSec=2",
		"",
		"[Install]",
		"WantedBy=default.target",
		"",
	}, "\n")
}

// execWord is a path as systemd's ExecStart takes it: quoted when it has a space or a quote.
func execWord(path string) string {
	if !strings.ContainsAny(path, " \t\"\\") {
		return path
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(path) + `"`
}

func systemctl(arguments ...string) Result {
	return Run(Call{Program: "systemctl", Args: append([]string{"--user"}, arguments...)})
}

// ServiceInstalled tells whether this person's systemd has swcode's unit.
func ServiceInstalled(place Place) bool {
	_, err := os.Stat(unitPath(place))
	return err == nil
}

// ServiceInstall writes the systemd user unit, starts it, and has it start again when the person logs in.
func ServiceInstall(settings config.Config, place Place) int {
	if IsUp(app.ControlURL(settings.Host, settings.Port)) && Stop(settings, place) != 0 {
		return failed
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	if err := os.MkdirAll(unitFolder(place), unitFolderMode); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	if !systemctl("show-environment").OK {
		fmt.Fprintln(os.Stderr, "systemd's user manager does not answer: is systemd running (in WSL, systemd=true in /etc/wsl.conf)?")
		return failed
	}
	if err := os.WriteFile(unitPath(place), []byte(serviceUnit(executable)), unitFileMode); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	systemctl("daemon-reload")
	if !systemctl("enable", "--now", serviceUnitFile).OK {
		_ = os.Remove(unitPath(place))
		systemctl("daemon-reload")
		fmt.Fprintf(os.Stderr, "systemd's user manager did not start the service: is systemd running (in WSL, systemd=true in /etc/wsl.conf)?\n")
		return failed
	}
	fmt.Printf("Installed the swcode service (%s): it starts when you log in.\n", executable)
	return 0
}

// ServiceUninstall stops the service, keeps it from starting again, and removes its unit.
func ServiceUninstall(_ config.Config, place Place) int {
	if !ServiceInstalled(place) {
		fmt.Println("swcode is not installed as a service.")
		return 0
	}
	systemctl("disable", "--now", serviceUnitFile)
	if err := os.Remove(unitPath(place)); err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	systemctl("daemon-reload")
	fmt.Println("Removed the swcode service. Its settings and sessions are kept.")
	return 0
}

// StartService starts the installed service.
func StartService(Place) bool { return systemctl("start", serviceUnitFile).OK }

// StopService stops the installed service cleanly: systemd does not start it again.
func StopService(Place) bool { return systemctl("stop", serviceUnitFile).OK }

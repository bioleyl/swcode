//go:build windows

package local

import (
	"fmt"
	"os"
	"strings"

	"swcode/daemon/app"
	"swcode/daemon/config"
)

// schtasks runs Task Scheduler's own command: Windows starts a per-person service as a task at logon.
func schtasks(arguments ...string) Call {
	return Call{Program: "schtasks.exe", Args: arguments}
}

// ServiceInstalled tells whether this person's tasks have swcode's.
func ServiceInstalled(Place) bool {
	return Run(schtasks("/Query", "/TN", serviceName)).OK
}

// ServiceInstall registers the task that runs the daemon when the person logs in, and starts it now.
func ServiceInstall(settings config.Config, place Place) int {
	if IsUp(app.ControlURL(settings.Host, settings.Port)) && Stop(settings, place) != 0 {
		return failed
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	script := strings.Join([]string{
		"$exe = " + PowerShellString(executable),
		`$argument = '-NoProfile -WindowStyle Hidden -Command "& ' + $exe + ' serve --service; exit $LASTEXITCODE"'`,
		`$powershell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'`,
		"$action = New-ScheduledTaskAction -Execute $powershell -Argument $argument",
		"$trigger = New-ScheduledTaskTrigger -AtLogOn",
		"$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -DontStopOnIdleEnd -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)",
		"Register-ScheduledTask -TaskName " + PowerShellString(serviceName) + " -Action $action -Trigger $trigger -Settings $settings -Description " + PowerShellString(serviceDescription) + " -Force -ErrorAction Stop | Out-Null",
	}, "; ")
	if registered := Run(PowerShell(script)); !registered.OK {
		fmt.Fprintf(os.Stderr, "Windows did not register the task: %s\n", registered.Output)
		return failed
	}
	if !StartService(place) {
		fmt.Fprintln(os.Stderr, "Windows registered the task, but did not start it: start it in Task Scheduler, or log in again.")
		return failed
	}
	fmt.Printf("Installed the swcode task (%s): it starts when you log in.\n", executable)
	return 0
}

// ServiceUninstall stops the daemon, then removes its task. Settings and sessions are kept.
func ServiceUninstall(settings config.Config, place Place) int {
	if !ServiceInstalled(place) {
		fmt.Println("swcode is not installed as a service.")
		return 0
	}
	if IsUp(app.ControlURL(settings.Host, settings.Port)) {
		Stop(settings, place)
	}
	Run(PowerShell("Stop-ScheduledTask -TaskName " + PowerShellString(serviceName) + " -ErrorAction SilentlyContinue"))
	removed := Run(PowerShell("Unregister-ScheduledTask -TaskName " + PowerShellString(serviceName) + " -Confirm:$false -ErrorAction Stop"))
	if !removed.OK {
		fmt.Fprintf(os.Stderr, "Windows did not remove the task: %s\n", removed.Output)
		return failed
	}
	fmt.Println("Removed the swcode service. Its settings and sessions are kept.")
	return 0
}

// StartService starts the installed task.
func StartService(Place) bool {
	return Run(schtasks("/Run", "/TN", serviceName)).OK
}

// StopService leaves stopping to the daemon's own request, which closes its sessions: Stop asks it.
func StopService(Place) bool { return false }

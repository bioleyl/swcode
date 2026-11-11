// Command swcode opens the app, runs the daemon, stops it, installs the program, runs it as a service, or updates it.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"swcode/daemon/cli"
	"swcode/daemon/config"
	"swcode/daemon/local"
	"swcode/daemon/serve"
	"swcode/daemon/update"
	"swcode/daemon/version"
)

const (
	failed       = 1
	wslDistroEnv = "WSL_DISTRO_NAME"
	// updateLimit is far more than a download takes, even over a slow line.
	updateLimit = 10 * time.Minute
)

func place(environment map[string]string) local.Place {
	home, _ := os.UserHomeDir()
	return local.Place{Environment: environment, Home: home, Platform: runtime.GOOS, WSLDistro: environment[wslDistroEnv]}
}

// settings is the settings file's, the environment's on top.
func settings(environment map[string]string) config.Config {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	loaded, err := config.Load(config.Environment(environment, home), config.Defaults{Cwd: cwd, Home: home})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(failed)
	}
	return loaded
}

// commandLine is the program that runs, as an update replaces it: nothing is replaced when it cannot say where it
// is.
func commandLine() update.Program {
	path, err := os.Executable()
	if err != nil {
		return update.Program{}
	}
	return update.CommandLine(path)
}

func runServe(command cli.Command, environment map[string]string) int {
	loaded := settings(environment)
	where := place(environment)
	cwd, _ := os.Getwd()
	options := serve.Options{Cwd: cwd, Environment: environment, Home: where.Home, Mode: command.Mode, Program: commandLine()}
	if command.Mode != cli.ModeTerminal {
		options.LogFile = local.LogFileOf(loaded)
	}
	outcome, err := serve.Run(loaded, options)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	if outcome == serve.Stopped {
		return 0
	}
	// A service manager starts the daemon again on the version or the settings just put in place; without one, a
	// daemon in the background starts the next one.
	if command.Mode == cli.ModeService {
		return failed
	}
	if !local.StartInBackground(where) {
		return failed
	}
	return 0
}

// runUpdate installs the latest release in place of this program, then starts the daemon that runs on it.
func runUpdate(environment map[string]string) int {
	loaded := settings(environment)
	ctx, cancel := context.WithTimeout(context.Background(), updateLimit)
	defer cancel()
	installed, err := update.Apply(ctx, http.DefaultClient, loaded.ReleasesURL, version.Build, commandLine())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failed
	}
	if installed == "" {
		fmt.Printf("swcode %s is the latest version.\n", version.Build)
		return 0
	}
	fmt.Printf("Updated swcode to %s.\n", installed)
	return local.RestartAfterUpdate(loaded, place(environment))
}

func run(command cli.Command) int {
	environment := config.ProcessEnvironment()
	switch command.Kind {
	case cli.KindHelp:
		if command.Problem != "" {
			fmt.Fprintln(os.Stderr, command.Problem+"\n")
		}
		fmt.Println(cli.Usage(version.Build))
		if command.Problem != "" {
			return failed
		}
		return 0
	case cli.KindVersion:
		fmt.Println(version.Build)
		return 0
	case cli.KindInstall:
		return local.Install(environment, place(environment))
	case cli.KindOpen:
		return local.Open(settings(environment), place(environment))
	case cli.KindStart:
		return local.Start(settings(environment), place(environment))
	case cli.KindStop:
		return local.Stop(settings(environment), place(environment))
	case cli.KindServiceInstall:
		return local.ServiceInstall(settings(environment), place(environment))
	case cli.KindServiceUninstall:
		return local.ServiceUninstall(settings(environment), place(environment))
	case cli.KindPair:
		return local.Pair(settings(environment))
	case cli.KindUpdate:
		return runUpdate(environment)
	case cli.KindServe:
		return runServe(command, environment)
	}
	return failed
}

func main() {
	os.Exit(run(cli.Parse(os.Args[1:])))
}

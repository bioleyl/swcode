package hosts

import (
	"os"
	"os/exec"
	"strings"

	"swcode/daemon/protocol"
)

const (
	wslProgram = "wsl.exe"
	sshProgram = "ssh"
	bashName   = "bash"
)

// sshOptions: the daemon talks to the shell, so no password prompt (keys or the agent do it), no terminal.
var sshOptions = []string{"-o", "BatchMode=yes", "-o", "ServerAliveInterval=30", "-T"}

// sshTerminalOptions ask for a terminal there: -tt even with no input of our own, its size kept by this one.
var sshTerminalOptions = []string{"-o", "BatchMode=yes", "-o", "ServerAliveInterval=30", "-tt"}

// ShellTransport is how a host's bash is reached: a script run there, by a program here.
type ShellTransport interface {
	// Name is how people know the host, in problems: an SSH endpoint, a WSL distribution.
	Name() string
	// Command is a process running a bash script there, its standard streams tied to this daemon.
	Command(script string) *exec.Cmd
	// TerminalCommand is a process running an interactive shell there, in a folder: ssh and wsl.exe give it a
	// pseudo-terminal of its own, which the command's own terminal keeps the size of.
	TerminalCommand(cwd string) *exec.Cmd
}

// ShellWord is a word for the remote shell, whatever it holds: single quotes, a quote inside closed and reopened.
func ShellWord(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

type transport struct {
	name     string
	command  func(script string) *exec.Cmd
	terminal func(cwd string) *exec.Cmd
}

func (t transport) Name() string                    { return t.name }
func (t transport) Command(script string) *exec.Cmd { return t.command(script) }
func (t transport) TerminalCommand(cwd string) *exec.Cmd {
	return t.terminal(cwd)
}

func started(program string, args ...string) *exec.Cmd {
	cmd := exec.Command(program, args...)
	hideWindow(cmd)
	return cmd
}

type transportOf struct{}

// ssh hands its words to the person's login shell there: bash runs the script, whatever that shell is.
func (transportOf) SSH(ref protocol.HostRefSSH) ShellTransport {
	return &transport{name: ref.Endpoint, command: func(script string) *exec.Cmd {
		args := append(append([]string{}, sshOptions...), ref.Endpoint, "exec "+bashName+" -c "+ShellWord(script))
		return started(sshProgram, args...)
	}, terminal: func(cwd string) *exec.Cmd {
		// -tt asks the far end for a terminal, whose size this process's own terminal keeps.
		args := append(append([]string{}, sshTerminalOptions...), ref.Endpoint, "cd "+ShellWord(cwd)+" && exec "+bashName+" -i")
		return started(sshProgram, args...)
	}}
}

// wsl.exe starts the distribution when it sleeps, and passes the words as they are.
func (transportOf) WSL(ref protocol.HostRefWSL) ShellTransport {
	return &transport{name: "WSL " + ref.Distro, command: func(script string) *exec.Cmd {
		return started(wslProgram, "-d", ref.Distro, "--exec", bashName, "-c", script)
	}, terminal: func(cwd string) *exec.Cmd {
		return started(wslProgram, "-d", ref.Distro, "--exec", bashName, "-c", "cd "+ShellWord(cwd)+"; exec "+bashName+" -i")
	}}
}

func (transportOf) Local(protocol.HostRefLocal) ShellTransport {
	return LocalTransport("this computer", "")
}

// TransportOf is how a WSL distribution's or an SSH endpoint's bash is reached.
func TransportOf(ref protocol.HostRef) ShellTransport {
	return protocol.MapHostRef[ShellTransport](ref, transportOf{})
}

// LocalTransport is this computer's own bash, as if it were another host, its home where said: for the tests.
func LocalTransport(name, home string) ShellTransport {
	environment := func() []string {
		if home == "" {
			return nil
		}
		return append(os.Environ(), "HOME="+home)
	}
	return &transport{name: name, command: func(script string) *exec.Cmd {
		cmd := started(bashName, "-c", script)
		cmd.Env = environment()
		return cmd
	}, terminal: func(cwd string) *exec.Cmd {
		cmd := started(bashName, "-i")
		cmd.Dir, cmd.Env = cwd, environment()
		return cmd
	}}
}

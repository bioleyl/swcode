//go:build !windows

package terminals_test

import (
	"context"
	"regexp"
	"strconv"
	"syscall"
	"testing"

	"swcode/daemon/protocol"
	"swcode/daemon/terminals"
)

// startedChild is the pid the shell's `&` child printed, once it printed it.
func startedChild(t *testing.T, service *terminals.Service, id string) int {
	t.Helper()
	printed := regexp.MustCompile(`child-(\d+)`)
	var pid int
	until(t, "the shell's child's pid", func() bool {
		found := printed.FindStringSubmatch(string(service.Output(id)))
		if found == nil {
			return false
		}
		pid, _ = strconv.Atoi(found[1])
		return pid != 0
	})
	return pid
}

func TestClosingATerminalEndsWhatItsShellStarted(t *testing.T) {
	service, folder := local(t)
	opened, refused := service.Open(context.Background(), terminals.Request{
		Cwd: folder, Command: "sleep 300 & echo child-$!", Host: protocol.LocalHost(), Title: "test",
	})
	if refused != "" {
		t.Fatal(refused)
	}
	pid := startedChild(t, service, opened.ID)
	service.Close(opened.ID)
	until(t, "the shell's child to end", func() bool {
		return syscall.Kill(pid, 0) != nil
	})
}

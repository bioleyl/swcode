//go:build windows

package main

import (
	"errors"
	"fmt"
	"runtime"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

const (
	wmiLocator   = "WbemScripting.SWbemLocator"
	processClass = "Win32_Process"
	// S_FALSE: COM was set up on this thread already, which is as good.
	comAlreadySetUp = 1
	createdFine     = 0
)

// startOutside has Windows start a program out of this program's job: WMI's Win32_Process.Create makes it from its
// own service, as it makes the WSL daemon's wsl.exe, so it outlives the window. The job itself never lets a process
// leave it: Git Bash's runtime would then start every command out of it.
func startOutside(commandLine string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil && !isComSetUp(err) {
		return err
	}
	defer ole.CoUninitialize()
	unknown, err := oleutil.CreateObject(wmiLocator)
	if err != nil {
		return err
	}
	defer unknown.Release()
	locator, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer locator.Release()
	service, err := oleutil.CallMethod(locator, "ConnectServer")
	if err != nil {
		return err
	}
	defer service.Clear()
	class, err := oleutil.CallMethod(service.ToIDispatch(), "Get", processClass)
	if err != nil {
		return err
	}
	defer class.Clear()
	made, err := oleutil.CallMethod(class.ToIDispatch(), "Create", commandLine)
	if err != nil {
		return err
	}
	defer made.Clear()
	if code := fmt.Sprint(made.Value()); code != fmt.Sprint(createdFine) {
		return fmt.Errorf("%s.Create answered %s", processClass, code)
	}
	return nil
}

func isComSetUp(err error) bool {
	var failure *ole.OleError
	return errors.As(err, &failure) && failure.Code() == comAlreadySetUp
}

//go:build windows

// ctrlbreak sends a CTRL_BREAK_EVENT to a target process group leader on
// Windows. Go's os.Process.Signal(os.Interrupt) is not implemented on
// Windows, so we drive the Win32 console control handler directly.
//
// Usage: ctrlbreak <pid>
package main

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: ctrlbreak <pid>")
		os.Exit(2)
	}
	pid, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid pid:", err)
		os.Exit(2)
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	freeConsole := kernel32.NewProc("FreeConsole")
	attachConsole := kernel32.NewProc("AttachConsole")
	setCtrlHandler := kernel32.NewProc("SetConsoleCtrlHandler")
	genEvent := kernel32.NewProc("GenerateConsoleCtrlEvent")

	// Detach from whatever console we inherited and attach to the target's.
	freeConsole.Call()
	r, _, errno := attachConsole.Call(uintptr(pid))
	if r == 0 {
		fmt.Fprintln(os.Stderr, "AttachConsole failed:", errno)
		os.Exit(1)
	}

	// Prevent our own process from being killed by the event we're about to fire.
	setCtrlHandler.Call(uintptr(unsafe.Pointer(nil)), 1)

	// CTRL_BREAK_EVENT = 1. Group id 0 means "every process attached to this console".
	r, _, errno = genEvent.Call(1, 0)
	if r == 0 {
		fmt.Fprintln(os.Stderr, "GenerateConsoleCtrlEvent failed:", errno)
		os.Exit(1)
	}
}

//go:build windows

// runserver is a Windows supervisor used by the smoke test. It launches
// send.to in a new process group so we can deliver CTRL_BREAK_EVENT to
// the child on demand. Windows does not have SIGTERM, and Go processes
// launched via bash inherit bash's console in a way that makes
// AttachConsole + GenerateConsoleCtrlEvent flaky from a sibling process.
//
// Communication:
//   - On startup, prints `CHILD_PID=<pid>` and `CTRL_PORT=<port>` on
//     stdout (before any child output).
//   - Any TCP connection to 127.0.0.1:<CTRL_PORT> triggers a graceful
//     shutdown: the supervisor sends CTRL_BREAK_EVENT to the child and
//     exits after the child does.
//
// Usage: runserver <child-exe> [child args...]
package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: runserver <child-exe> [args...]")
		os.Exit(2)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
	ctrlPort := ln.Addr().(*net.TCPAddr).Port

	cmd := exec.Command(os.Args[1], os.Args[2:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pipe stdout:", err)
		os.Exit(1)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pipe stderr:", err)
		os.Exit(1)
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
		os.Exit(1)
	}
	// Print control handshake first, before the child's stream begins.
	fmt.Printf("CHILD_PID=%d\n", cmd.Process.Pid)
	fmt.Printf("CTRL_PORT=%d\n", ctrlPort)
	os.Stdout.Sync()

	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)

	ctrlHit := make(chan struct{}, 1)
	go func() {
		conn, aerr := ln.Accept()
		if aerr == nil {
			_ = conn.Close()
		}
		ctrlHit <- struct{}{}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	sendBreak := func() {
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		gen := kernel32.NewProc("GenerateConsoleCtrlEvent")
		r, _, errno := gen.Call(1 /* CTRL_BREAK_EVENT */, uintptr(cmd.Process.Pid))
		if r == 0 {
			fmt.Fprintln(os.Stderr, "GenerateConsoleCtrlEvent failed:", errno)
			_ = cmd.Process.Kill()
		}
	}

	select {
	case <-sig:
		sendBreak()
	case <-ctrlHit:
		sendBreak()
	case waitErr := <-done:
		// child exited on its own
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		} else if waitErr != nil {
			fmt.Fprintln(os.Stderr, "child:", waitErr)
			os.Exit(1)
		}
		return
	}

	waitErr := <-done
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		os.Exit(exitErr.ExitCode())
	}
}

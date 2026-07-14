package main

import (
	"os"
	"runtime"
)

// openTTY opens the controlling terminal for direct TUI I/O, bypassing
// os.Stdin/os.Stdout so stdout stays reserved for the final selected path.
// Returns separate input/output handles and a single close func.
func openTTY() (in *os.File, out *os.File, closeFn func(), err error) {
	if runtime.GOOS == "windows" {
		in, err = os.OpenFile("CONIN$", os.O_RDWR, 0)
		if err != nil {
			return nil, nil, nil, err
		}
		out, err = os.OpenFile("CONOUT$", os.O_RDWR, 0)
		if err != nil {
			in.Close()
			return nil, nil, nil, err
		}
		return in, out, func() { in.Close(); out.Close() }, nil
	}
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	return f, f, func() { f.Close() }, nil
}

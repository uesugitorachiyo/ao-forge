//go:build windows

package cli

import (
	"syscall"
	"unsafe"
)

type coord struct {
	X int16
	Y int16
}

type smallRect struct {
	Left   int16
	Top    int16
	Right  int16
	Bottom int16
}

type consoleScreenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

var (
	kernel32                   = syscall.NewLazyDLL("kernel32.dll")
	getConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

func isTerminal(fd uintptr) bool {
	var info consoleScreenBufferInfo
	rc, _, _ := getConsoleScreenBufferInfo.Call(fd, uintptr(unsafe.Pointer(&info)))
	return rc != 0
}

func getTerminalSize(fd uintptr) (int, int, error) {
	var info consoleScreenBufferInfo
	rc, _, err := getConsoleScreenBufferInfo.Call(fd, uintptr(unsafe.Pointer(&info)))
	if rc == 0 {
		return 0, 0, err
	}
	width := int(info.Window.Right - info.Window.Left + 1)
	height := int(info.Window.Bottom - info.Window.Top + 1)
	return width, height, nil
}

//go:build !windows

package cli

import (
	"syscall"
	"unsafe"
)

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

func isTerminal(fd uintptr) bool {
	var sz winsize
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&sz)))
	return err == 0
}

func getTerminalSize(fd uintptr) (int, int, error) {
	var sz winsize
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&sz)))
	if err != 0 {
		return 0, 0, err
	}
	return int(sz.Col), int(sz.Row), nil
}

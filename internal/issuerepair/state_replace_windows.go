//go:build windows

package issuerepair

import (
	"syscall"
	"unsafe"
)

const (
	movefileReplaceExisting = 0x00000001
	movefileWriteThrough    = 0x00000008
)

var moveFileExProc = kernel32DLL.NewProc("MoveFileExW")

func replaceStateFile(temporaryPath, statePath string) error {
	temporaryPointer, err := syscall.UTF16PtrFromString(temporaryPath)
	if err != nil {
		return err
	}
	statePointer, err := syscall.UTF16PtrFromString(statePath)
	if err != nil {
		return err
	}
	result, _, callErr := moveFileExProc.Call(
		uintptr(unsafe.Pointer(temporaryPointer)),
		uintptr(unsafe.Pointer(statePointer)),
		movefileReplaceExisting|movefileWriteThrough,
	)
	if result == 0 {
		return callErr
	}
	return nil
}

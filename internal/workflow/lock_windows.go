package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func (s *Service) lock() (func(), error) {
	if err := os.MkdirAll(s.localDir(), 0700); err != nil {
		return nil, err
	}
	f, e := os.OpenFile(filepath.Join(s.localDir(), "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	kernel := syscall.NewLazyDLL("kernel32.dll")
	lock := kernel.NewProc("LockFileEx")
	unlock := kernel.NewProc("UnlockFileEx")
	overlapped := new(syscall.Overlapped)
	ok, _, _ := lock.Call(f.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(overlapped)))
	if ok == 0 {
		f.Close()
		return nil, errors.New("workflow is busy in another session; retry after it finishes")
	}
	return func() { unlock.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(overlapped))); f.Close() }, nil
}

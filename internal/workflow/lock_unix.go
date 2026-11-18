//go:build !windows

package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// The OS releases this lock even when the host or verifier is killed.
func (s *Service) lock() (func(), error) {
	if err := os.MkdirAll(s.localDir(), 0700); err != nil {
		return nil, err
	}
	f, e := os.OpenFile(filepath.Join(s.localDir(), "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("workflow is busy in another session; retry after it finishes")
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}

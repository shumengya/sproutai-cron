//go:build !windows

package lock

import (
	"os"
	"syscall"
)

// File holds an open file with an exclusive lock.
type File struct {
	f *os.File
}

// TryLock opens path and acquires a non-blocking exclusive lock.
// Returns (nil, false, nil) if the lock is held by another process.
func TryLock(path string) (*File, bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, false, err
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = f.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &File{f: f}, true, nil
}

// Unlock releases the lock and closes the file.
func (l *File) Unlock() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}

// IsLocked reports whether path is currently exclusively locked by another process.
func IsLocked(path string) bool {
	lf, ok, err := TryLock(path)
	if err != nil || !ok {
		return err == nil && !ok
	}
	_ = lf.Unlock()
	return false
}

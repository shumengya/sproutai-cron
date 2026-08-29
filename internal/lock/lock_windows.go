//go:build windows

package lock

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// File holds an open file with an exclusive lock.
type File struct {
	f *os.File
}

// TryLock opens path and acquires a non-blocking exclusive lock (LockFileEx).
func TryLock(path string) (*File, bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, false, err
	}
	var ol windows.Overlapped
	err = windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&ol,
	)
	if err != nil {
		_ = f.Close()
		if err == windows.ERROR_LOCK_VIOLATION || err == windows.ERROR_IO_PENDING {
			return nil, false, nil
		}
		// ERROR_LOCK_VIOLATION is common; also map generic errno
		if errno, ok := err.(syscall.Errno); ok && errno == 33 {
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
	var ol windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(l.f.Fd()), 0, 1, 0, &ol)
	err := l.f.Close()
	l.f = nil
	return err
}

// IsLocked reports whether path is currently exclusively locked.
func IsLocked(path string) bool {
	lf, ok, err := TryLock(path)
	if err != nil || !ok {
		return err == nil && !ok
	}
	_ = lf.Unlock()
	return false
}

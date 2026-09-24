package hostlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// Lock owns an open inode for the service lifetime. The file is deliberately
// retained on close: removing it could let another process lock a new inode
// while an old owner still holds the original.
type Lock struct {
	mu   sync.Mutex
	file *os.File
}

func Acquire(path string) (*Lock, error) {
	if path == "" {
		return nil, errors.New("empty host lock path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create host lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open host lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire host lock: %w", err)
	}
	return &Lock{file: file}, nil
}

func (l *Lock) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close() // closing the descriptor releases flock
	l.file = nil
	return err
}

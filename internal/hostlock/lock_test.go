package hostlock

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestT08ProcessLock(t *testing.T) {
	if os.Getenv("PERPETUAL_LOCK_CHILD") == "1" {
		lock, err := Acquire(os.Getenv("PERPETUAL_LOCK_PATH"))
		if err != nil {
			os.Exit(23)
		}
		_ = lock.Close()
		os.Exit(0)
	}
	path := filepath.Join(t.TempDir(), "owner.lock")
	lock, err := Acquire(path)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	defer lock.Close()
	first, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	run := func() error {
		cmd := exec.Command(os.Args[0], "-test.run=^TestT08ProcessLock$")
		cmd.Env = append(os.Environ(), "PERPETUAL_LOCK_CHILD=1", "PERPETUAL_LOCK_PATH="+path)
		return cmd.Run()
	}
	if err := run(); err == nil {
		t.Fatal("second process acquired owned lock")
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := os.Stat(path)
	if err != nil {
		t.Fatal("lock file was removed on close")
	}
	if !os.SameFile(first, second) {
		t.Fatal("lock inode replaced")
	}
	if err := run(); err != nil {
		t.Fatalf("lock not released after shutdown: %v", err)
	}
}

func TestT08LockErrorPaths(t *testing.T) {
	if _, err := Acquire(""); err == nil {
		t.Error("empty lock path accepted")
	}
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, []byte("owned fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(filepath.Join(parent, "child.lock")); err == nil {
		t.Error("mkdir failure ignored")
	}
	if _, err := Acquire(t.TempDir()); err == nil {
		t.Error("directory accepted as lock file")
	}
}

package grpc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceRunningBinary(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "staged")
	dst := filepath.Join(dir, "agent")
	if err := os.WriteFile(src, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	// Keep the old file open, as the kernel does for a running executable.
	old, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()

	if err := replaceRunningBinary(src, dst); err != nil {
		t.Fatalf("replace: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "new" {
		t.Fatalf("dst = %q, want new", got)
	}
	if fi, _ := os.Stat(dst); fi.Mode().Perm() != 0755 {
		t.Fatalf("mode = %v, want 0755", fi.Mode().Perm())
	}
	if _, err := os.Stat(dst + ".new"); !os.IsNotExist(err) {
		t.Fatal("temp file left behind")
	}
	// The old inode is untouched: the running process keeps its code.
	buf := make([]byte, 3)
	if _, err := old.Read(buf); err != nil || string(buf) != "old" {
		t.Fatalf("old handle read %q, %v", buf, err)
	}

	if err := replaceRunningBinary(filepath.Join(dir, "missing"), dst); err == nil {
		t.Fatal("expected error for missing source")
	}
}

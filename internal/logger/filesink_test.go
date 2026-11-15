package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSinkWritesAndRotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "proxy.log")
	if err := EnableFileSink(path, 200, 2); err != nil {
		t.Fatalf("EnableFileSink: %v", err)
	}
	defer DisableFileSink()

	if FileSinkPath() != path {
		t.Fatalf("FileSinkPath = %q, want %q", FileSinkPath(), path)
	}
	for i := 0; i < 20; i++ {
		Log("[Test] línea de prueba número %02d con algo de relleno", i)
	}
	DisableFileSink()

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(current), "número 19") {
		t.Fatalf("last line missing from current log: %q", current)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected rotated file .1: %v", err)
	}
	if _, err := os.Stat(path + ".3"); err == nil {
		t.Fatal("must keep at most 2 rotated files")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("log file must be 0600, got %v", info.Mode().Perm())
	}
}

func TestFileSinkSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxy.log")
	if err := EnableFileSink(path, 0, 0); err != nil {
		t.Fatal(err)
	}
	Log("primera ejecución")
	DisableFileSink()
	if err := EnableFileSink(path, 0, 0); err != nil {
		t.Fatal(err)
	}
	Log("segunda ejecución")
	DisableFileSink()

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "primera ejecución") || !strings.Contains(string(data), "segunda ejecución") {
		t.Fatalf("log must append across restarts: %q", data)
	}
}

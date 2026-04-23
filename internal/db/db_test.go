package db

import (
	"os"
	"testing"
)

func TestNewCreatesDB(t *testing.T) {
	dir := t.TempDir()
	d, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()

	if _, err := os.Stat(dir + "/multi_model_router.db"); os.IsNotExist(err) {
		t.Fatal("database file not created")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	d, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()

	if err := d.SetConfig("test_key", "test_value"); err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	got, err := d.GetConfig("test_key")
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if got != "test_value" {
		t.Fatalf("GetConfig() = %q, want %q", got, "test_value")
	}
}

func TestGetConfigMissing(t *testing.T) {
	d, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()

	got, err := d.GetConfig("nonexistent")
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if got != "" {
		t.Fatalf("GetConfig() = %q, want empty", got)
	}
}

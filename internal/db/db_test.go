package db_test

import (
	"testing"

	"github.com/wojons/conscientiousness/internal/db"
)

// axiom:trace work_item=runtime-harness-01 spec=specs/001-architecture.md plan=phase-1/task-1-1/step-1-1-2

func TestDetectBackendPostgres(t *testing.T) {
	tests := []string{
		"postgres://user:pass@localhost:5432/db",
		"postgresql://user@host/db",
	}
	for _, url := range tests {
		backend, err := db.DetectBackend(url)
		if err != nil {
			t.Errorf("DetectBackend(%q) error: %v", url, err)
		}
		if backend != db.BackendPostgres {
			t.Errorf("DetectBackend(%q) = %s, want postgres", url, backend)
		}
	}
}

func TestDetectBackendSQLite(t *testing.T) {
	backend, err := db.DetectBackend("sqlite://dev.db")
	if err != nil {
		t.Fatalf("DetectBackend: %v", err)
	}
	if backend != db.BackendSQLite {
		t.Errorf("DetectBackend = %s, want sqlite", backend)
	}
}

func TestDetectBackendInvalid(t *testing.T) {
	tests := []string{
		"",
		"short",
		"mysql://host/db",
		"unknown://",
	}
	for _, url := range tests {
		_, err := db.DetectBackend(url)
		if err == nil {
			t.Errorf("DetectBackend(%q) should have failed", url)
		}
	}
}

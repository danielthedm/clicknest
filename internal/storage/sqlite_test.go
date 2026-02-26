package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *SQLite {
	t.Helper()
	dir := t.TempDir()
	enc, err := NewEncryptor(dir)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	db, err := NewSQLite(filepath.Join(dir, "test.db"), enc)
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateAndGetProject(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	p, err := db.CreateProject(ctx, "proj-1", "My App")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID != "proj-1" {
		t.Fatalf("expected ID proj-1, got %q", p.ID)
	}
	if p.APIKey == "" {
		t.Fatal("expected non-empty API key")
	}
	if !isValidAPIKey(p.APIKey) {
		t.Fatalf("API key has unexpected format: %q", p.APIKey)
	}

	got, err := db.GetProject(ctx, "proj-1")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Name != "My App" {
		t.Fatalf("expected name 'My App', got %q", got.Name)
	}
}

func TestGetProjectByAPIKey(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	p, err := db.CreateProject(ctx, "proj-2", "Test App")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	got, err := db.GetProjectByAPIKey(ctx, p.APIKey)
	if err != nil {
		t.Fatalf("GetProjectByAPIKey: %v", err)
	}
	if got.ID != "proj-2" {
		t.Fatalf("expected ID proj-2, got %q", got.ID)
	}
}

func TestGetProjectByAPIKey_NotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	_, err := db.GetProjectByAPIKey(ctx, "cn_nonexistent")
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got: %v", err)
	}
}

func TestCreateUser_CountUsers(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	n, err := db.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 users, got %d", n)
	}

	_, err = db.CreateUser(ctx, "admin@example.com", "hashedpassword")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	n, err = db.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers after create: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 user, got %d", n)
	}
}

func TestGetUserByEmail(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	_, err := db.CreateUser(ctx, "test@example.com", "hash123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	u, err := db.GetUserByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if u.Email != "test@example.com" {
		t.Fatalf("expected email test@example.com, got %q", u.Email)
	}
	if u.PasswordHash != "hash123" {
		t.Fatalf("expected hash123, got %q", u.PasswordHash)
	}
}

func TestGetUserByEmail_NotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	_, err := db.GetUserByEmail(ctx, "nobody@example.com")
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got: %v", err)
	}
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	_, err := db.CreateUser(ctx, "dup@example.com", "hash1")
	if err != nil {
		t.Fatalf("first CreateUser: %v", err)
	}
	_, err = db.CreateUser(ctx, "dup@example.com", "hash2")
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
}

// isValidAPIKey checks the "cn_" prefix + hex format.
func isValidAPIKey(key string) bool {
	if len(key) < 4 || key[:3] != "cn_" {
		return false
	}
	for _, c := range key[3:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

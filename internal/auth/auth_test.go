package auth

import (
	"context"
	"testing"

	"github.com/danielleslie/clicknest/internal/storage"
)

func TestProjectFromContext_Miss(t *testing.T) {
	p := ProjectFromContext(context.Background())
	if p != nil {
		t.Fatalf("expected nil for empty context, got %+v", p)
	}
}

func TestProjectFromContext_Hit(t *testing.T) {
	project := &storage.Project{ID: "proj-1", Name: "Test", APIKey: "cn_abc"}
	ctx := WithProject(context.Background(), project)
	got := ProjectFromContext(ctx)
	if got == nil {
		t.Fatal("expected project, got nil")
	}
	if got.ID != project.ID {
		t.Fatalf("expected project ID %q, got %q", project.ID, got.ID)
	}
}

func TestWithProject_DoesNotMutateParent(t *testing.T) {
	parent := context.Background()
	project := &storage.Project{ID: "proj-1"}
	_ = WithProject(parent, project)

	// The original context should still return nil.
	if p := ProjectFromContext(parent); p != nil {
		t.Fatal("WithProject should not mutate the parent context")
	}
}

func TestValidateAPIKey_EmptyKey(t *testing.T) {
	_, err := ValidateAPIKey(context.Background(), nil, "")
	if err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized for empty key, got: %v", err)
	}
}

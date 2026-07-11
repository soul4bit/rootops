package content

import (
	"path/filepath"
	"testing"
)

func TestLoadCatalog(t *testing.T) {
	root := filepath.Join("..", "..", "content")

	catalog, err := LoadCatalog(root)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	if len(catalog.Articles) != 1 {
		t.Fatalf("articles = %d, want 1", len(catalog.Articles))
	}

	if _, ok := catalog.Quizzes["linux-terminal"]; !ok {
		t.Fatalf("quiz for linux-terminal not loaded")
	}

	if _, ok := catalog.Labs["linux-terminal"]; !ok {
		t.Fatalf("lab for linux-terminal not loaded")
	}

	if problems := ValidateCatalog(catalog); len(problems) > 0 {
		t.Fatalf("ValidateCatalog() problems = %v", problems)
	}
}

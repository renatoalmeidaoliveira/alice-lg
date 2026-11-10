package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/julienschmidt/httprouter"

	"github.com/alice-lg/alice-lg/pkg/config"
)

func makeDocsDir(t *testing.T) string {
	dir := t.TempDir()
	files := map[string]string{
		"02-vrfs.md":        "# VRFs e Route Targets\n\nText",
		"01-introducao.md":  "Intro without heading\n",
		"03-labels.md":      "```\n# not a title\n```\n\n# MPLS Labels #\n",
		".hidden.md":        "# Hidden",
		"notes.txt":         "not a doc",
		"img/topologia.png": "png",
		"img/.secret.png":   "secret",
		"sub/ignored.md":    "# Ignored",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDocsList(t *testing.T) {
	docs := NewDocs(config.DocsConfig{Path: makeDocsDir(t)})
	list, err := docs.List()
	if err != nil {
		t.Fatal(err)
	}
	expected := []struct{ id, title string }{
		{"01-introducao", "Introducao"},
		{"02-vrfs", "VRFs e Route Targets"},
		{"03-labels", "MPLS Labels"},
	}
	if len(list) != len(expected) {
		t.Fatal("unexpected docs:", list)
	}
	for i, e := range expected {
		if list[i].ID != e.id || list[i].Title != e.title {
			t.Errorf("doc %d: expected %v, got %v", i, e, list[i])
		}
		if list[i].Content != "" {
			t.Error("list should not include content")
		}
	}
}

func TestDocsGet(t *testing.T) {
	docs := NewDocs(config.DocsConfig{Path: makeDocsDir(t)})
	doc, err := docs.Get("02-vrfs")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Content != "# VRFs e Route Targets\n\nText" {
		t.Error("unexpected content:", doc.Content)
	}

	for _, id := range []string{"../02-vrfs", ".hidden", "sub/ignored", "missing", ""} {
		if _, err := docs.Get(id); err != ErrDocNotFound {
			t.Errorf("expected not found for %q, got %v", id, err)
		}
	}
}

func TestDocsDisabled(t *testing.T) {
	docs := NewDocs(config.DocsConfig{})
	list, err := docs.List()
	if err != nil || len(list) != 0 {
		t.Error("disabled docs should be empty:", list, err)
	}
	if _, err := docs.Get("01-introducao"); err != ErrDocNotFound {
		t.Error("expected not found")
	}
}

func TestDocsAssets(t *testing.T) {
	docs := NewDocs(config.DocsConfig{Path: makeDocsDir(t)})
	router := httprouter.New()
	docs.RegisterDocsAssets(router)

	cases := map[string]int{
		"/docs-assets/img/topologia.png": http.StatusOK,
		"/docs-assets/img/.secret.png":   http.StatusNotFound,
		"/docs-assets/img/":              http.StatusNotFound,
		"/docs-assets/../go.mod":         http.StatusNotFound,
	}
	for path, status := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.URL.Path = path
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != status {
			t.Errorf("%s: expected %d, got %d", path, status, res.Code)
		}
	}
}

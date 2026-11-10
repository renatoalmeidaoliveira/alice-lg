package http

/*
 Docs serves a directory of markdown documents, which
 are listed in the sidebar of the web interface.

 Every file ending in .md in the docs directory (no
 subdirectories) is a document. The document id is the
 filename without the extension. Documents are ordered
 by filename, so a numeric prefix can be used for
 ordering, e.g. 01-introduction.md, 02-vrfs.md.

 The title is taken from the first level one heading
 ("# Title"). If there is none, it is derived from the id.

 All other files in the docs directory (e.g. images)
 are served below /docs-assets/.

 The directory is read on every request, so documents
 can be added or changed without restarting alice.
*/

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/julienschmidt/httprouter"

	"github.com/alice-lg/alice-lg/pkg/config"
)

const (
	// DocsAssetsPath is the url prefix of the docs assets
	DocsAssetsPath = "/docs-assets"

	// maxDocSize limits the size of a markdown document
	maxDocSize = 2 << 20 // 2 MiB
)

var (
	// ErrDocNotFound is returned if a document does not exist
	ErrDocNotFound = &ErrResourceNotFoundError{}

	docIDPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	docPrefixPattern = regexp.MustCompile(`^[0-9]+[-_. ]+`)
)

// Doc is a markdown document
type Doc struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content,omitempty"`
}

// DocsResponse lists all documents
type DocsResponse struct {
	Title string `json:"title"`
	Docs  []*Doc `json:"docs"`
}

// DocResponse is a single document with content
type DocResponse struct {
	Doc *Doc `json:"doc"`
}

// Docs provides markdown documents from a directory
type Docs struct {
	Config config.DocsConfig
}

// NewDocs creates a new docs provider
func NewDocs(cfg config.DocsConfig) *Docs {
	return &Docs{Config: cfg}
}

// Enabled checks if a docs directory is configured
func (d *Docs) Enabled() bool {
	return d.Config.Path != ""
}

// docTitle extracts the title from the first level one
// heading of a markdown document or derives it from the id.
func docTitle(id string, content []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), maxDocSize)
	inCode := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimSpace(strings.TrimRight(line[2:], "#"))
			if title != "" {
				return title
			}
		}
	}

	// Derive from id: strip ordering prefix and separators
	title := docPrefixPattern.ReplaceAllString(id, "")
	title = strings.NewReplacer("-", " ", "_", " ").Replace(title)
	title = strings.TrimSpace(title)
	if title == "" {
		return id
	}
	return strings.ToUpper(title[:1]) + title[1:]
}

// readDoc loads a document by id
func (d *Docs) readDoc(id string) (*Doc, error) {
	if !docIDPattern.MatchString(id) || strings.Contains(id, "..") {
		return nil, ErrDocNotFound
	}
	path := filepath.Join(d.Config.Path, id+".md")
	stat, err := os.Stat(path)
	if err != nil || !stat.Mode().IsRegular() {
		return nil, ErrDocNotFound
	}
	if stat.Size() > maxDocSize {
		return nil, errors.New("document too large")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrDocNotFound
	}
	return &Doc{
		ID:      id,
		Title:   docTitle(id, content),
		Content: string(content),
	}, nil
}

// List returns all documents without content
func (d *Docs) List() ([]*Doc, error) {
	docs := []*Doc{}
	if !d.Enabled() {
		return docs, nil
	}
	entries, err := os.ReadDir(d.Config.Path)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		id := name[:len(name)-len(".md")]
		doc, err := d.readDoc(id)
		if err != nil {
			continue // Skip invalid names, e.g. hidden files
		}
		doc.Content = ""
		docs = append(docs, doc)
	}
	return docs, nil
}

// Get returns a document including its content
func (d *Docs) Get(id string) (*Doc, error) {
	if !d.Enabled() {
		return nil, ErrDocNotFound
	}
	return d.readDoc(id)
}

// RegisterDocsAssets serves the non markdown files
// in the docs directory, e.g. images.
func (d *Docs) RegisterDocsAssets(router *httprouter.Router) {
	if !d.Enabled() {
		return
	}
	files := http.StripPrefix(DocsAssetsPath, http.FileServer(docsAssetsFS{
		fs: http.Dir(d.Config.Path),
	}))
	router.Handler("GET", DocsAssetsPath+"/*path", files)
}

// docsAssetsFS hides directory listings and dotfiles
type docsAssetsFS struct {
	fs http.FileSystem
}

func (a docsAssetsFS) Open(name string) (http.File, error) {
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			return nil, os.ErrNotExist
		}
	}
	f, err := a.fs.Open(name)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		f.Close()
		return nil, os.ErrNotExist
	}
	return f, nil
}

// getDocs returns the docs provider of the server
func (s *Server) getDocs() *Docs {
	if s.docs == nil {
		return NewDocs(s.cfg.UI.Docs)
	}
	return s.docs
}

// API endpoints

func (s *Server) apiDocsList(
	_ context.Context,
	_ *http.Request,
	_ httprouter.Params,
) (response, error) {
	provider := s.getDocs()
	docs, err := provider.List()
	if err != nil {
		return nil, err
	}
	return &DocsResponse{
		Title: provider.Config.Title,
		Docs:  docs,
	}, nil
}

func (s *Server) apiDocShow(
	_ context.Context,
	_ *http.Request,
	params httprouter.Params,
) (response, error) {
	doc, err := s.getDocs().Get(params.ByName("docId"))
	if err != nil {
		return nil, err
	}
	return &DocResponse{Doc: doc}, nil
}

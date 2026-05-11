// Package upload provides file storage abstraction.
// LocalProvider writes to disk (dev); swap for S3Provider in prod.
package upload

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Provider stores a file and returns its public URL.
type Provider interface {
	Put(ctx context.Context, key string, r io.Reader) (publicURL string, err error)
}

// LocalProvider writes files to a directory on disk and serves them statically.
type LocalProvider struct {
	root    string // absolute path to upload root
	baseURL string // public URL prefix, e.g. "http://localhost:8080/uploads"
}

// NewLocal creates a LocalProvider, ensuring root exists.
func NewLocal(root, baseURL string) (*LocalProvider, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("upload: criar dir %s: %w", root, err)
	}
	return &LocalProvider{root: root, baseURL: baseURL}, nil
}

// Put writes r to root/key and returns baseURL/key as the public URL.
func (p *LocalProvider) Put(_ context.Context, key string, r io.Reader) (string, error) {
	dest := filepath.Join(p.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("upload: mkdir: %w", err)
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("upload: criar arquivo: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		os.Remove(dest)
		return "", fmt.Errorf("upload: escrever: %w", err)
	}
	return p.baseURL + "/" + key, nil
}

// Root returns the absolute path to the upload root directory.
// Useful for callers that need to check file existence before uploading.
func (p *LocalProvider) Root() string {
	return p.root
}

// PublicURL returns the public URL for a given key without writing any file.
// Mirrors the URL that Put would return for the same key.
func (p *LocalProvider) PublicURL(key string) string {
	return p.baseURL + "/" + key
}

// FileServer returns an http.Handler that serves files from root.
// Mount it at the same path prefix as baseURL, e.g.:
//
//	r.Handle("/uploads/*", http.StripPrefix("/uploads", p.FileServer()))
func (p *LocalProvider) FileServer() http.Handler {
	return http.FileServer(http.Dir(p.root))
}

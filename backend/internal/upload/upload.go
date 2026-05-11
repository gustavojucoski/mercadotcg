// Package upload provides file storage abstraction.
// LocalProvider writes to disk (dev); swap for S3Provider in prod.
package upload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Provider stores a file and returns its public URL.
type Provider interface {
	// Put stores r under key and returns the public URL.
	// contentType is required for S3 (Content-Type header); LocalProvider ignores it.
	Put(ctx context.Context, key string, r io.Reader, contentType string) (publicURL string, err error)

	// PublicURL returns the public URL for key without writing any file.
	PublicURL(key string) string

	// Exists reports whether key has already been stored.
	// Returns (false, nil) for a missing key; (false, err) for I/O errors.
	Exists(ctx context.Context, key string) (bool, error)
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
// contentType is ignored — LocalProvider stores raw bytes regardless of MIME.
func (p *LocalProvider) Put(_ context.Context, key string, r io.Reader, _ string) (string, error) {
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
func (p *LocalProvider) Root() string {
	return p.root
}

// PublicURL returns the public URL for a given key without writing any file.
func (p *LocalProvider) PublicURL(key string) string {
	return p.baseURL + "/" + key
}

// Exists reports whether key exists on disk.
func (p *LocalProvider) Exists(_ context.Context, key string) (bool, error) {
	dest := filepath.Join(p.root, filepath.FromSlash(key))
	_, err := os.Stat(dest)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("upload: stat %s: %w", key, err)
}

// FileServer returns an http.Handler that serves files from root.
// Mount it at the same path prefix as baseURL, e.g.:
//
//	r.Handle("/uploads/*", http.StripPrefix("/uploads", p.FileServer()))
func (p *LocalProvider) FileServer() http.Handler {
	return http.FileServer(http.Dir(p.root))
}

// NewFromEnv constructs a Provider from environment variables.
//
// STORAGE_BACKEND: "local" (default) | "s3"
//
// Local backend:
//
//	STORAGE_LOCAL_PATH     — root directory (default: "./data/images")
//	STORAGE_LOCAL_BASE_URL — public URL prefix (required)
//
// S3 backend:
//
//	S3_BUCKET             — bucket name (required)
//	S3_REGION             — AWS region (default: "us-east-1")
//	STORAGE_S3_CUSTOM_URL — CloudFront or custom origin URL (optional)
func NewFromEnv() (Provider, error) {
	backend := os.Getenv("STORAGE_BACKEND")
	if backend == "" {
		backend = "local"
	}

	switch backend {
	case "local":
		root := os.Getenv("STORAGE_LOCAL_PATH")
		if root == "" {
			root = "./data/images"
		}
		baseURL := os.Getenv("STORAGE_LOCAL_BASE_URL")
		if baseURL == "" {
			return nil, fmt.Errorf("upload: STORAGE_LOCAL_BASE_URL é obrigatório para backend local")
		}
		return NewLocal(root, baseURL)

	case "s3":
		bucket := os.Getenv("S3_BUCKET")
		if bucket == "" {
			return nil, fmt.Errorf("upload: S3_BUCKET é obrigatório para backend s3")
		}
		region := os.Getenv("S3_REGION")
		if region == "" {
			region = "us-east-1"
		}
		customURL := os.Getenv("STORAGE_S3_CUSTOM_URL")
		return NewS3(bucket, region, customURL)

	default:
		return nil, fmt.Errorf("upload: STORAGE_BACKEND desconhecido: %q (use \"local\" ou \"s3\")", backend)
	}
}

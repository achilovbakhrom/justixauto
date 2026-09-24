package repository

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// DirStorage stores files in a private local directory (development, tests).
type DirStorage struct{ Root string }

func (d DirStorage) path(key string) string { return filepath.Join(d.Root, key[:2], key) }

func (d DirStorage) Put(_ context.Context, key string, data []byte, _ string) error {
	p := d.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // p is built from a server-generated UUID key (see service.Service.Upload), never user-supplied
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (d DirStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	return os.Open(d.path(key))
}
func (d DirStorage) Delete(_ context.Context, key string) error { return os.Remove(d.path(key)) }

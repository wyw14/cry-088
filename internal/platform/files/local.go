package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Metadata struct {
	ID          string
	Name        string
	ContentType string
	Size        int64
	Digest      string
	StorageKey  string
}

type Local struct {
	Root        string
	MaxBytes    int64
	AllowedExt  map[string]bool
	AllowedMIME map[string]bool
}

func (l Local) Save(ctx context.Context, id, name, contentType string, source io.Reader) (Metadata, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	ext := strings.ToLower(filepath.Ext(filepath.Base(name)))
	if !l.AllowedExt[ext] || !l.AllowedMIME[contentType] {
		return Metadata{}, fmt.Errorf("file type is not allowed")
	}
	if err := os.MkdirAll(l.Root, 0o750); err != nil {
		return Metadata{}, fmt.Errorf("create file root: %w", err)
	}
	storageKey := id + ext
	path := filepath.Join(l.Root, storageKey)
	if filepath.Dir(path) != filepath.Clean(l.Root) {
		return Metadata{}, fmt.Errorf("invalid storage path")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return Metadata{}, fmt.Errorf("create file: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	hash := sha256.New()
	limited := io.LimitReader(source, l.MaxBytes+1)
	written, err := io.Copy(io.MultiWriter(file, hash), limited)
	if err != nil {
		return Metadata{}, fmt.Errorf("write file: %w", err)
	}
	if written > l.MaxBytes {
		return Metadata{}, fmt.Errorf("file exceeds size limit")
	}
	if err := file.Sync(); err != nil {
		return Metadata{}, fmt.Errorf("sync file: %w", err)
	}
	remove = false
	return Metadata{ID: id, Name: filepath.Base(name), ContentType: contentType, Size: written, Digest: hex.EncodeToString(hash.Sum(nil)), StorageKey: storageKey}, nil
}

func (l Local) Open(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if filepath.Base(storageKey) != storageKey {
		return nil, fmt.Errorf("invalid storage key")
	}
	return os.Open(filepath.Join(l.Root, storageKey))
}

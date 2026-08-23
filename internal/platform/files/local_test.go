package files

import (
	"bytes"
	"context"
	"testing"
)

func TestLocalStorageRejectsTraversalAndOversize(t *testing.T) {
	root := t.TempDir()
	store := Local{Root: root, MaxBytes: 4, AllowedExt: map[string]bool{".txt": true}, AllowedMIME: map[string]bool{"text/plain": true}}
	if _, err := store.Save(context.Background(), "1", "../secret.txt", "text/plain", bytes.NewBufferString("ok")); err != nil {
		t.Fatalf("basename should be safe: %v", err)
	}
	if _, err := store.Save(context.Background(), "2", "x.txt", "text/plain", bytes.NewBufferString("toolong")); err == nil {
		t.Fatal("oversize file must fail")
	}
}

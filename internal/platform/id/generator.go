package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type Generator interface {
	New(prefix string) (string, error)
}

type Random struct{}

func (Random) New(prefix string) (string, error) {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return prefix + "_" + hex.EncodeToString(buffer), nil
}

type Sequence struct{ Next int }

func (s *Sequence) New(prefix string) (string, error) {
	s.Next++
	return fmt.Sprintf("%s_%06d", prefix, s.Next), nil
}

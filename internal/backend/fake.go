package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type Fake struct{}

func (Fake) Name() string { return "fake" }

func (Fake) ReadRef(ctx context.Context, ref string) (string, error) {
	return Fake{}.ReadRefWithFlags(ctx, ref, nil)
}

func (Fake) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
	// For fake backend, we ignore flags but include them in the hash for determinism
	input := ref
	for _, flag := range flags {
		input += "|" + flag
	}
	sum := sha256.Sum256([]byte(input))
	return fmt.Sprintf("fake_%s", hex.EncodeToString(sum[:8])), nil
}

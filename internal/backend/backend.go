package backend

import "context"

type Backend interface {
	ReadRef(ctx context.Context, ref string) (string, error)
	ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error)
	Name() string
}

package crawler

import "context"

type embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

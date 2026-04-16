package embedding

import (
	"context"
	"fmt"
	"math"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// Client は OpenAI Embeddings API クライアント（並列数制限付き）。
type Client struct {
	client *openai.Client
	model  openai.EmbeddingModel
	sem    chan struct{}
}

func NewClient(apiKey string, concurrency int, model openai.EmbeddingModel) *Client {
	return &Client{
		client: openai.NewClient(apiKey),
		model:  model,
		sem:    make(chan struct{}, concurrency),
	}
}

// Embed はテキストのベクトルを生成する（指数バックオフでリトライ）。
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	const maxRetries = 3
	for attempt := range maxRetries {
		embedding, err := c.callAPI(ctx, text)
		if err == nil {
			return embedding, nil
		}
		if attempt == maxRetries-1 {
			return nil, fmt.Errorf("embedding.Embed: %w", err)
		}
		wait := time.Duration(math.Pow(2, float64(attempt))) * time.Second
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, fmt.Errorf("embedding.Embed: unreachable")
}

func (c *Client) callAPI(ctx context.Context, text string) ([]float32, error) {
	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequestStrings{
		Input: []string{text},
		Model: c.model,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	return resp.Data[0].Embedding, nil
}

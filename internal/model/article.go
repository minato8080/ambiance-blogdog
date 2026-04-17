package model

import "time"

type Article struct {
	ID          string
	BlogID      string
	URL         string
	Title       string
	Summary     string
	Tags        []string
	Embedding   []float32
	PublishedAt *time.Time
	IndexedAt   time.Time
}

type SimilarArticle struct {
	URL         string
	Title       string
	BlogURL     string
	BlogName    string
	PublishedAt *time.Time
	Tags        []string
	Similarity  float64
}

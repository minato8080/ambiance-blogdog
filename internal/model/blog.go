package model

import "time"

type BlogStatus string

const (
	BlogStatusPending  BlogStatus = "pending"
	BlogStatusIndexing BlogStatus = "indexing"
	BlogStatusReady    BlogStatus = "ready"
	BlogStatusError    BlogStatus = "error"
)

type Blog struct {
	ID           string
	PlatformID   string
	BlogURL      string
	Name         string
	Status       BlogStatus
	ErrorCount   int
	LastSyncedAt *time.Time
	DiscoveredAt time.Time
}

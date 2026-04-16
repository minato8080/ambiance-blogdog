package model

import "time"

type Platform struct {
	ID             string
	Slug           string
	Name           string
	FeedURLPattern string
	CreatedAt      time.Time
}

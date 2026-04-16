package crawler

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler は3フェーズ + 時間断面クローラーのスケジューリングを管理する。
type Scheduler struct {
	discoverer  *Discoverer
	indexer     *Indexer
	syncer      *Syncer
	historical  *Historical
	crawlEvery  time.Duration
	syncEvery   time.Duration
	staleEvery  time.Duration
}

func NewScheduler(
	discoverer *Discoverer,
	indexer *Indexer,
	syncer *Syncer,
	historical *Historical,
	crawlIntervalMin, syncIntervalMin int,
) *Scheduler {
	return &Scheduler{
		discoverer: discoverer,
		indexer:    indexer,
		syncer:     syncer,
		historical: historical,
		crawlEvery: time.Duration(crawlIntervalMin) * time.Minute,
		syncEvery:  time.Duration(syncIntervalMin) * time.Minute,
		staleEvery: 24 * time.Hour,
	}
}

// Start はすべてのクローラーをバックグラウンドで起動する。
func (s *Scheduler) Start(ctx context.Context) {
	go s.loop(ctx, "discovery", s.crawlEvery, s.discoverer.Run)
	go s.loop(ctx, "indexer", s.syncEvery, s.indexer.Run)
	go s.loop(ctx, "syncer", s.staleEvery, s.syncer.Run)
	go s.loop(ctx, "historical", s.staleEvery, s.historical.Run)
}

func (s *Scheduler) loop(ctx context.Context, name string, interval time.Duration, fn func(context.Context) error) {
	// 起動直後に1回実行
	s.run(ctx, name, fn)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.run(ctx, name, fn)
		}
	}
}

func (s *Scheduler) run(ctx context.Context, name string, fn func(context.Context) error) {
	slog.Info("crawler: start", "phase", name)
	if err := fn(ctx); err != nil {
		slog.Error("crawler: failed", "phase", name, "error", err)
		return
	}
	slog.Info("crawler: done", "phase", name)
}

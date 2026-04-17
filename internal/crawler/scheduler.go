package crawler

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler は3フェーズ + 時間断面クローラーのスケジューリングを管理する。
type Scheduler struct {
	discoverer      *Discoverer
	indexer         *Indexer
	syncer          *Syncer
	historical      *Historical
	recent          *Recent
	crawlEvery      time.Duration
	syncEvery       time.Duration
	staleEvery      time.Duration
	historicalEvery time.Duration
	recentEvery     time.Duration
}

func NewScheduler(
	discoverer *Discoverer,
	indexer *Indexer,
	syncer *Syncer,
	historical *Historical,
	recent *Recent,
	crawlIntervalMin, syncIntervalMin, historicalIntervalMin, recentIntervalMin int,
) *Scheduler {
	return &Scheduler{
		discoverer:      discoverer,
		indexer:         indexer,
		syncer:          syncer,
		historical:      historical,
		recent:          recent,
		crawlEvery:      time.Duration(crawlIntervalMin) * time.Minute,
		syncEvery:       time.Duration(syncIntervalMin) * time.Minute,
		staleEvery:      24 * time.Hour,
		historicalEvery: time.Duration(historicalIntervalMin) * time.Minute,
		recentEvery:     time.Duration(recentIntervalMin) * time.Minute,
	}
}

// Start はすべてのクローラーをバックグラウンドで起動する。
func (s *Scheduler) Start(ctx context.Context) {
	go s.loop(ctx, "discovery", s.crawlEvery, s.discoverer.Run)
	go s.loop(ctx, "indexer", s.syncEvery, s.indexer.Run)
	go s.loop(ctx, "syncer", s.staleEvery, s.syncer.Run)
	go s.loop(ctx, "historical", s.historicalEvery, s.historical.Run)
	go s.loop(ctx, "recent", s.recentEvery, s.recent.Run)
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

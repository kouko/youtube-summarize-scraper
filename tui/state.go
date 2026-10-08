package tui

import (
	"sync"

	"github.com/kouko/youtube-summarize-scraper/pipeline"
)

// AppState holds the shared state updated by the event bridge and read by the TUI.
type AppState struct {
	mu sync.RWMutex

	// Configuration
	ConfigPath    string
	ConfigContent string

	// Execution stats
	Stats        pipeline.Stats
	WatchIter    int
	QueueLen     int
	CurrentVideo string
	CurrentStage string // e.g., "stage 1: generating summary"

	// Recent events (for display)
	RecentEvents []string

	// Control flags
	IsRunning bool
	IsPaused  bool
}

// NewAppState returns a zero-initialized AppState.
func NewAppState() *AppState {
	return &AppState{
		RecentEvents: make([]string, 0, 100),
	}
}

// StateSnapshot is a lock-free copy of AppState for safe rendering.
// It deliberately does not embed AppState so the mutex is never copied.
type StateSnapshot struct {
	ConfigPath    string
	ConfigContent string
	Stats         pipeline.Stats
	WatchIter     int
	QueueLen      int
	CurrentVideo  string
	CurrentStage  string
	RecentEvents  []string
	IsRunning     bool
	IsPaused      bool
}

// Snapshot returns a point-in-time copy of the state for rendering.
// The TUI calls this every render tick; the returned value is safe to read
// without holding any lock.
func (s *AppState) Snapshot() StateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StateSnapshot{
		ConfigPath:    s.ConfigPath,
		ConfigContent: s.ConfigContent,
		Stats:         s.Stats,
		WatchIter:     s.WatchIter,
		QueueLen:      s.QueueLen,
		CurrentVideo:  s.CurrentVideo,
		CurrentStage:  s.CurrentStage,
		RecentEvents:  append([]string(nil), s.RecentEvents...),
		IsRunning:     s.IsRunning,
		IsPaused:      s.IsPaused,
	}
}

// UpdateConfig sets the configuration path and content.
func (s *AppState) UpdateConfig(path, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ConfigPath = path
	s.ConfigContent = content
}

// UpdateStats updates the pipeline statistics.
func (s *AppState) UpdateStats(stats pipeline.Stats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Stats = stats
}

// UpdateWatch sets the watch iteration.
func (s *AppState) SetWatchIter(iter int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.WatchIter = iter
}

// SetQueueLen sets the processing queue length.
func (s *AppState) SetQueueLen(len int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.QueueLen = len
}

// SetCurrentVideo sets the currently processing video ID and stage.
func (s *AppState) SetCurrentVideo(videoID, stage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentVideo = videoID
	s.CurrentStage = stage
}

// SetStage updates only the current stage label.
func (s *AppState) SetStage(stage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentStage = stage
}

// ClearCurrentVideo marks that no video is being processed right now.
func (s *AppState) ClearCurrentVideo() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentVideo = ""
	s.CurrentStage = ""
}

// IncrementSuccess adds one to the success count.
func (s *AppState) IncrementSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Stats.Success++
}

// IncrementSkipped adds one to the skipped count.
func (s *AppState) IncrementSkipped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Stats.Skipped++
}

// IncrementFailed adds one to the failed count.
func (s *AppState) IncrementFailed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Stats.Failed++
}

// IncrementPartial adds one to the partial count.
func (s *AppState) IncrementPartial() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Stats.Partial++
}

// AddRecentEvent appends a log line to the recent events, keeping only the last N.
func (s *AppState) AddRecentEvent(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.RecentEvents) >= 100 {
		// Drop the oldest
		s.RecentEvents = s.RecentEvents[1:]
	}
	s.RecentEvents = append(s.RecentEvents, line)
}

// SetRunning sets whether the pipeline is running.
func (s *AppState) SetRunning(running bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsRunning = running
}

// SetPaused sets whether the pipeline is paused.
func (s *AppState) SetPaused(paused bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsPaused = paused
}

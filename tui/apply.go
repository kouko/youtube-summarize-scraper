package tui

// applyEvent applies one parsed log event to AppState. It is called from the
// bridge's consumer goroutine only, so AppState's lock is uncontended here.
func applyEvent(s *AppState, ev LogEvent) {
	switch ev.Kind {
	case EventVideoStart:
		s.SetCurrentVideo(ev.Video, ev.Stage)
		s.AddRecentEvent(ev.Raw)
	case EventStage:
		s.SetStage(ev.Stage)
		s.AddRecentEvent(ev.Raw)
	case EventVideoSkipped:
		s.IncrementSkipped()
		s.ClearCurrentVideo()
		s.AddRecentEvent(ev.Raw)
	case EventVideoFailed:
		s.IncrementFailed()
		s.ClearCurrentVideo()
		s.AddRecentEvent(ev.Raw)
	case EventVideoPartial:
		s.IncrementPartial()
		s.AddRecentEvent(ev.Raw)
	case EventVideoDone:
		s.IncrementSuccess()
		s.ClearCurrentVideo()
		s.AddRecentEvent(ev.Raw)
	case EventBatchComplete:
		if ev.Stats != nil {
			s.UpdateStats(*ev.Stats)
		}
		s.AddRecentEvent(ev.Raw)
	default:
		s.AddRecentEvent(ev.Raw)
	}
}

package logging

import "log/slog"

// levelVarHolder is implemented by handlers that own the live [*slog.LevelVar],
// letting [LevelVarOf] recover it without depending on a concrete type.
type levelVarHolder interface {
	LevelVar() *slog.LevelVar
}

// handlerUnwrapper is implemented by middleware decorators that wrap an inner
// handler, letting [LevelVarOf] walk past them to the [ContextHandler] that
// holds the level var. Custom middleware can implement it to stay transparent
// to live-level recovery.
type handlerUnwrapper interface {
	Unwrap() slog.Handler
}

// LevelVar returns the live level source the [ContextHandler] was built with by
// [New], or nil when it was constructed directly. Use it to retarget verbosity
// at runtime: ch.LevelVar().Set(slog.LevelDebug).
func (h *ContextHandler) LevelVar() *slog.LevelVar { return h.levelVar }

// LevelVarOf recovers the live [*slog.LevelVar] backing a logger built by [New],
// walking through any transparent middleware (those implementing Unwrap). It
// returns nil when the logger was not built by [New] (or its level var is
// hidden behind opaque middleware). The returned var is shared with the logger,
// so calling Set on it retargets the logger's level live.
func LevelVarOf(logger *slog.Logger) *slog.LevelVar {
	if logger == nil {
		return nil
	}
	for h := logger.Handler(); h != nil; {
		if holder, ok := h.(levelVarHolder); ok {
			if lv := holder.LevelVar(); lv != nil {
				return lv
			}
		}
		if uw, ok := h.(handlerUnwrapper); ok {
			h = uw.Unwrap()
			continue
		}
		break
	}
	return nil
}

// SetLevel retargets the level of a logger built by [New] live, returning true
// when it succeeded (the logger exposes a recoverable [*slog.LevelVar]) and
// false otherwise. It is the convenience over LevelVarOf for the common case —
// the hook a shikumi config-reload calls to apply a new level without
// rebuilding the logger (the zap.AtomicLevel pattern).
//
//	logging.SetLevel(log, slog.LevelDebug) // verbose now, live
func SetLevel(logger *slog.Logger, level slog.Level) bool {
	if lv := LevelVarOf(logger); lv != nil {
		lv.Set(level)
		return true
	}
	return false
}

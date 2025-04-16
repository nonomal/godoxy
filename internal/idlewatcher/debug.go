package idlewatcher

import (
	"iter"
	"strconv"

	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type watcherDebug struct {
	*Watcher
}

func (w watcherDebug) MarshalMap() map[string]any {
	state := w.state.Load()
	return map[string]any{
		"name": w.Name(),
		"state": map[string]string{
			"status": string(state.status),
			"ready":  strconv.FormatBool(state.ready),
			"err":    fmtErr(state.err),
		},
		"expires":    strutils.FormatTime(w.expires()),
		"last_reset": strutils.FormatTime(w.lastReset.Load()),
		"config":     w.cfg,
	}
}

func Watchers() iter.Seq2[string, watcherDebug] {
	return func(yield func(string, watcherDebug) bool) {
		watcherMapMu.RLock()
		defer watcherMapMu.RUnlock()

		for k, w := range watcherMap {
			if !yield(k, watcherDebug{w}) {
				return
			}
		}
	}
}

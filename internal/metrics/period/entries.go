package period

import (
	"time"

	"github.com/yusing/go-proxy/pkg/json"
)

type Entries[T any] struct {
	entries  [maxEntries]*T
	index    int
	count    int
	interval time.Duration
	lastAdd  time.Time
}

const maxEntries = 100

func newEntries[T any](duration time.Duration) *Entries[T] {
	interval := duration / maxEntries
	if interval < time.Second {
		interval = time.Second
	}
	return &Entries[T]{
		interval: interval,
		lastAdd:  time.Now(),
	}
}

func (e *Entries[T]) Add(now time.Time, info *T) {
	if now.Sub(e.lastAdd) < e.interval {
		return
	}
	e.entries[e.index] = info
	e.index = (e.index + 1) % maxEntries
	if e.count < maxEntries {
		e.count++
	}
	e.lastAdd = now
}

func (e *Entries[T]) Get() []*T {
	if e.count < maxEntries {
		return e.entries[:e.count]
	}
	res := make([]*T, maxEntries)
	copy(res, e.entries[e.index:])
	copy(res[maxEntries-e.index:], e.entries[:e.index])
	return res
}

func (e *Entries[T]) MarshalJSONTo(buf []byte) []byte {
	return json.MarshalTo(map[string]any{
		"entries":  e.Get(),
		"interval": e.interval,
	}, buf)
}

func (e *Entries[T]) UnmarshalJSON(data []byte) error {
	var v struct {
		Entries  []*T          `json:"entries"`
		Interval time.Duration `json:"interval"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if len(v.Entries) == 0 {
		return nil
	}
	entries := v.Entries
	if len(entries) > maxEntries {
		entries = entries[:maxEntries]
	}
	now := time.Now()
	for _, info := range entries {
		e.Add(now, info)
	}
	e.interval = v.Interval
	return nil
}

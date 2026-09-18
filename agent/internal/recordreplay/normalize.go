package recordreplay

import (
	"math"
	"sort"
	"time"

	"github.com/zhoujun94511/xtest-nova/agent/internal/touchreader"
)

type gesture struct {
	index   int
	started time.Time
	ended   time.Time
	start   Point
	last    Point
	active  bool
}

type Normalizer struct {
	recordedAt time.Time
	gestures   map[int]gesture
	completed  []gesture
}

func NewNormalizer(recordedAt time.Time) *Normalizer {
	return &Normalizer{recordedAt: recordedAt, gestures: map[int]gesture{}}
}

func (n *Normalizer) Feed(at time.Time, event touchreader.Event) []Action {
	if event.Index < 0 || event.Index > 9 {
		return nil
	}
	point := Point{X: clamp(event.XP), Y: clamp(event.YP)}
	current := n.gestures[event.Index]
	switch event.Operation {
	case "d":
		if len(n.gestures) == 0 {
			n.completed = nil
		}
		n.gestures[event.Index] = gesture{index: event.Index, started: at, start: point, last: point, active: true}
	case "m":
		if current.active {
			current.last = point
			n.gestures[event.Index] = current
		}
	case "u":
		if !current.active {
			return nil
		}
		delete(n.gestures, event.Index)
		current.last, current.ended, current.active = point, at, false
		n.completed = append(n.completed, current)
		if len(n.gestures) != 0 {
			return nil
		}
		return n.finish()
	}
	return nil
}

func (n *Normalizer) finish() []Action {
	if len(n.completed) == 0 {
		return nil
	}
	sort.Slice(n.completed, func(i, j int) bool { return n.completed[i].index < n.completed[j].index })
	earliest, latest := n.completed[0].started, n.completed[0].ended
	for _, item := range n.completed[1:] {
		if item.started.Before(earliest) {
			earliest = item.started
		}
		if item.ended.After(latest) {
			latest = item.ended
		}
	}
	offset := earliest.Sub(n.recordedAt).Milliseconds()
	if offset < 0 {
		offset = 0
	}
	duration := latest.Sub(earliest).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	if len(n.completed) > 1 {
		contacts := make([]Contact, 0, len(n.completed))
		for _, item := range n.completed {
			contacts = append(contacts, Contact{Index: item.index, Start: item.start, End: item.last})
		}
		if duration == 0 {
			duration = 1
		}
		return []Action{{Type: "multi_touch", OffsetMillis: offset, DurationMillis: duration, Contacts: contacts}}
	}
	item := n.completed[0]
	distance := math.Hypot(item.last.X-item.start.X, item.last.Y-item.start.Y)
	action := Action{OffsetMillis: offset, DurationMillis: duration, Start: item.start}
	if distance <= 0.02 {
		if duration >= 500 {
			action.Type = "long_press"
		} else {
			action.Type = "tap"
			action.DurationMillis = 0
		}
	} else {
		action.Type, action.End = "swipe", item.last
		if action.DurationMillis == 0 {
			action.DurationMillis = 1
		}
	}
	return []Action{action}
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

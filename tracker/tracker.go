package tracker

import (
	"net/url"
	"time"

	"github.com/pkg/errors"

	"github.com/wqsa/bget/meta"
)

//Event is download event announce to tracker
type Event int

func (e Event) String() string {
	switch e {
	case EventNone:
		return ""
	case EventStart:
		return "started"
	case EventComplete:
		return "completed"
	case EventStop:
		return "stopped "
	default:
		panic("unknow envent")
	}
}

//Event value
const (
	EventNone Event = iota
	EventComplete
	EventStart
	EventStop
)

var (
	errWaitTime = errors.New("should wait")
)

//Tracker is a tracker interface
type Tracker interface {
	announce(meta.Hash, []byte, string, Event, *AnnounceStats) ([]string, error)
	waitTime() time.Time
}

// NewTracker return a Tracker instance
func newTracker(t string) (Tracker, error) {
	u, err := url.Parse(t)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	switch u.Scheme {
	case "udp":
		return newudpTracker(u), nil
	case "http", "https":
		return newhttpTracker(u), nil
	}
	return nil, errors.Errorf("unexpect scheme: %v", u.Scheme)
}

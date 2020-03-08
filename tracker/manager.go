package tracker

import (
	"time"

	"github.com/pkg/errors"

	jww "github.com/spf13/jwalterweatherman"

	"github.com/wqsa/bget/meta"
)

//Manager mange trackers of a task
type Manager struct {
	serverID   []byte
	serverAddr string
	workIndex  int
	trackers   []Tracker
	reqC       chan announceRequest
}

//NewManager return a Manager instance
func NewManager(trackers []string, id []byte, addr string) (*Manager, error) {
	m := &Manager{serverID: id, serverAddr: addr}
	m.trackers = make([]Tracker, 0, len(trackers))
	for _, u := range trackers {
		t, err := newTracker(u)
		if err != nil {
			jww.DEBUG.Printf("%v is invaild", u)
			continue
		}
		m.trackers = append(m.trackers, t)
	}
	if len(m.trackers) == 0 {
		return nil, errors.New("no vaild tracker")
	}
	return m, nil
}

//Start announce start to a vaild tracker and return peers
func (m *Manager) Start(hash meta.Hash) (<-chan []string, error) {
	peerC := make(chan []string)
	go func() {
		reqs := []announceRequest{
			announceRequest{event: EventStart},
		}
		events := []Event{EventStart}
		peers, err := m.workTracker().announce(hash, m.serverID, m.serverAddr, reqs[0].event, &reqs[0].stats)
		if err == nil {
			if len(events) == 1 {
				events[0] = EventNone
			} else {
				//pop event
				reqs = reqs[1:]
			}
			peerC <- peers
		} else {
			jww.DEBUG.Printf("announce to tracker error:%v\n", err)
			m.workIndex = (m.workIndex + 1) % len(m.trackers)
		}
		select {
		case <-time.After(time.Until(m.workTracker().waitTime())):
		case r := <-m.reqC:
			if reqs[0].event == EventNone {
				reqs[0] = r
			} else {
				//pending request
				reqs = append(reqs, r)
			}
		}
	}()
	return peerC, nil
}

//AnnounceStart announce downlaod start to tracker, but manager will auto announce start when invoke Start, so it's need't to use this function
func (m *Manager) Announce(opts ...Option) {
	op := new(options)
	for _, o := range opts {
		o.apply(op)
	}
	m.reqC <- announceRequest{
		event: op.event,
		stats: op.stats,
	}
}

//AnnounceRequest is request to tracker
type announceRequest struct {
	event Event
	stats AnnounceStats
}

func (m *Manager) workTracker() Tracker {
	return m.trackers[m.workIndex]
}

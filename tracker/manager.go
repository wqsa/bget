package tracker

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/google/logger"

	"../meta"
)

var (
	errNotFound = errors.New("can't find tracker")
	errTask     = errors.New("no this task")
	errCreate   = errors.New("create task fail")

	keyInfoHash = "info hash"
)

//Tracker error
var (
	ErrNoVaildTracker = errors.New("no vaild tracker")
)

//Manager mange trackers of a task
type Manager struct {
	serverID   []byte
	serverIP   string
	serverPort int
	now        int
	nowLock    sync.RWMutex
	trackers   []Tracker
}

//NewManager return a Manager instance
func NewManager(trackers []string, id []byte, ip string, port int) *Manager {
	m := &Manager{serverID: id, serverIP: ip, serverPort: port, now: -1}
	m.trackers = make([]Tracker, len(trackers))
	for i, a := range trackers {
		t := NewTracker(a)
		if t == nil {
			continue
		}
		m.trackers[i] = t
	}
	return m
}

//Start announce start to a vaild tracker and return peers
func (m *Manager) Start(hash meta.Hash, status *DownloadStatus) ([]string, error) {
	for i, t := range m.trackers {
		peers, err := t.Announce(hash, m.serverID, net.ParseIP(m.serverIP), m.serverPort, EventStart, status)
		if err == nil {
			m.nowLock.Lock()
			m.now = i
			m.nowLock.Unlock()
			return peers, nil
		}
	}
	return nil, ErrNoVaildTracker
}

//Announce request peers to tracker
func (m *Manager) Announce(hash meta.Hash, status *DownloadStatus) ([]string, error) {
	return m.announce(hash, status, EventNone)
}

//Stop tell tracker the task is stop
func (m *Manager) Stop(hash meta.Hash, status *DownloadStatus) ([]string, error) {
	return m.announce(hash, status, EventStop)
}

//Finish announce the tracker the task is complete
func (m *Manager) Finish(hash meta.Hash, status *DownloadStatus) ([]string, error) {
	return m.announce(hash, status, EventComplete)
}

//GetWaitTime return the time will wait for next announce
func (m *Manager) GetWaitTime() time.Time {
	m.nowLock.RLock()
	defer m.nowLock.RUnlock()
	if m.now == -1 {
		logger.Fatalln("no vaild tracker")
	}
	return m.trackers[m.now].getWaitTime()
}

func (m *Manager) announce(hash meta.Hash, status *DownloadStatus, event int) (addrs []string, err error) {
	m.nowLock.RLock()
	if m.now == -1 {
		m.nowLock.RUnlock()
		return nil, ErrNoVaildTracker
	}
	addrs, err = m.trackers[m.now].Announce(hash, m.serverID, net.ParseIP(m.serverIP), m.serverPort, event, status)
	m.nowLock.RUnlock()
	if err != nil {
		ps, err := m.Start(hash, status)
		if err != nil {
			return nil, ErrNoVaildTracker
		}
		m.nowLock.RLock()
		addrs, err = m.trackers[m.now].Announce(hash, m.serverID, net.ParseIP(m.serverIP), m.serverPort, event, status)
		m.nowLock.RUnlock()
		addrs = append(addrs, ps...)
		if err != nil {
			logger.Fatalln("tracker error")
		}
	}
	return
}

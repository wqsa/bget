package bittorrent

import (
	"sync"
	"time"

	"github.com/google/logger"

	"./meta"
	"./peer"
	"./tracker"
)

const (
	maxPeerNum      = 50
	minAnnounceWait = 10 * time.Second

	stateInit = 1 + iota
	stateRunning
	stateStop
)

type task struct {
	torrent   *meta.Torrent
	manager   *peer.Manager
	trackers  *tracker.Manager
	state     int
	stateLock sync.RWMutex
}

func newTask(torrent *meta.Torrent, path string, id [peer.PeerIDLen]byte, ip string, port int, memc chan int64) *task {
	t := new(task)
	t.torrent = torrent
	announce := make([]string, 1+len(t.torrent.AnnounceList))
	i := 0
	if t.torrent.Announce != "" {
		announce[i] = t.torrent.Announce
		i++
	}
	for _, al := range t.torrent.AnnounceList {
		for _, a := range al {
			announce[i] = a
			i++
		}
	}
	t.trackers = tracker.NewManager(announce, id[:], ip, port)
	t.manager = peer.NewManager(torrent, path, memc)
	if t.manager == nil {
		return nil
	}
	t.state = stateInit
	return t
}

func (t *task) start() {
	if t.getState() != stateInit {
		return
	}
	t.setState(stateRunning)
	go func() {
		left := int64(t.torrent.Info.PieceLength) * int64(len(t.torrent.Info.Pieces))
		status := &tracker.DownloadStatus{0, 0, left}
		addrs, err := t.trackers.Start(t.torrent.Info.Hash, status)
		if err != nil {
			logger.Warningf("no vaild tracker for %v\n", t.torrent.Info.Hash.String())
			return
		}
		logger.Infof("get %v peers from tracker", len(addrs))
		t.manager.AddAddrPeer(addrs)
		time.AfterFunc(t.trackers.GetWaitTime().Sub(time.Now()), t.announce)
	}()
	err := t.manager.Run()
	if err != nil {
		logger.Errorln(err)
	}
	return
}

func (t *task) stop() error {
	if t.getState() != stateRunning {
		return nil
	}
	t.setState(stateInit)
	status := t.manager.GetStatus()
	t.trackers.Stop(t.torrent.Info.Hash, status.Status)
	return t.manager.Close()
}

func (t *task) save() {
	t.manager.Save()
}

func (t *task) announce() {
	status := t.manager.GetStatus()
	if status.PeerNum < maxPeerNum {
		addrs, err := t.trackers.Announce(t.torrent.Info.Hash, status.Status)
		if err != nil {
			logger.Warningf("no vaild tracker for %v\n", t.torrent.Info.Hash.String())
			return
		}
		logger.Infof("get %v peers from tracker", len(addrs))
		t.manager.AddAddrPeer(addrs)
	}
	wait := t.trackers.GetWaitTime().Sub(time.Now())
	if wait < minAnnounceWait {
		time.AfterFunc(minAnnounceWait, t.announce)
		return
	}
	time.AfterFunc(wait, t.announce)
}

func (t *task) setState(state int) {
	t.stateLock.Lock()
	t.state = state
	t.stateLock.Unlock()
}

func (t *task) getState() int {
	t.stateLock.RLock()
	defer t.stateLock.RUnlock()
	return t.state
}

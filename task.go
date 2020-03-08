package bget

import (
	"errors"
	"net"
	"strconv"
	"time"

	jww "github.com/spf13/jwalterweatherman"

	"github.com/wqsa/bget/common"
	"github.com/wqsa/bget/meta"
	"github.com/wqsa/bget/peer"
	"github.com/wqsa/bget/storage"
	"github.com/wqsa/bget/tracker"
)

const (
	maxPeerNum      = 50
	minAnnounceWait = 10 * time.Second
)

const (
	stateInit = iota
	stateRunning
	stateStop
)

const (
	actionPause = iota
	actionResume
)

const (
	operateTimeout = 3 * time.Second
)

//TaskStats is stats of BT task
type TaskStats struct {
	State                int
	UploadSpeed          int64
	DownloadSpeed        int64
	UploadData           int64
	DownloadData         int64
	AverageDownloadSpeed int64
	AvergaeUploadSpeed   int64
	DownloadTime         int64
}

//Task is a BT task
type Task struct {
	torrent      *meta.Torrent
	manager      *peer.Manager
	trackers     *tracker.Manager
	storage      *storage.Storage
	state        int
	messagec     chan interface{}
	dataStats    *dataStats
	torrentStats *torrentStats
	stopc        chan struct{}
	actionc      chan int
	peerc        <-chan []string
	err          error
}

func newTask(torrent *meta.Torrent, path string, id [peer.PeerIDLen]byte, ip string, port int, memc chan int64) *Task {
	t := new(Task)
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
	var err error
	t.trackers, err = tracker.NewManager(announce, id[:], net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		return nil
	}
	t.manager = peer.NewManager(torrent, path, memc)
	if t.manager == nil {
		return nil
	}
	t.state = stateInit
	return t
}

//Pause pasue the running task
func (t *Task) Pause() error {
	select {
	case t.actionc <- actionPause:
	case <-time.After(operateTimeout):
		return errors.New("operate timeout")
	}
	return nil
}

//Resume resume a task from stop
func (t *Task) Resume() error {
	select {
	case t.actionc <- actionResume:
	case <-time.After(operateTimeout):
		return errors.New("operate timeout")
	}
	return nil
}

func (t *Task) GetFiles() error {
	return nil
}

//MarkFile mark files download or don't download
func (t *Task) MarkFile(files map[string]bool) error {
	return nil
}

//Clean clean the resource of task
func Clean() error {
	return nil
}

//Stop stop the task, it just used for delete task
func (t *Task) Stop() error {
	t.stopc <- struct{}{}
	return nil
}

//GetStats return task stats
func (t *Task) GetStats() *dataStats {
	return t.dataStats
}

func (t *Task) run() {
	for {
		select {
		case b := <-t.manager.GetData():
			if err := t.writeBlock(&b); err != nil {
				t.handleError(err)
				return
			}
		case r := <-t.manager.GetRequest():
			go func() {
				if err := t.readBlock(r.BlockInfo); err != nil {
					jww.WARN.Printf("read block failed, block:%v, error:%v\n", r.BlockInfo, err)
				}
			}()
		case action := <-t.actionc:
			if err := t.doAction(action); err != nil {
				t.handleError(err)
				return
			}
		case peers := <-t.peerc:
			t.manager.AddRawPeer(peers)
		case <-t.stopc:
			return
		}
	}
}

func (t *Task) doAction(action int) error {
	switch action {
	case actionPause:
	case actionResume:
	}
	return nil
}

func (t *Task) pause() error {
	if err := t.manager.Close(); err != nil {
		return err
	}
	t.state = stateStop
	return nil
}

func (t *Task) resume() error {
	if err := t.manager.Run(); err != nil {
		return err
	}
	var err error
	if t.peerc, err = t.trackers.Start(t.torrent.Info.Hash); err != nil {
		return err
	}
	return nil
}

func (t *Task) handleError(err error) {
	t.pause()
	t.err = err
	jww.ERROR.Printf("task error: %+v\n", err)
}

func (t *Task) readBlock(blockInfo common.BlockInfo) error {
	resultc := t.storage.ReadBlock(t.torrent.Info.Hash.String(), blockInfo)
	select {
	case result := <-resultc:
		if result.Error != nil {
			return result.Error
		}
		t.manager.ReplyRequest(&common.Block{BlockInfo: blockInfo, Data: result.Data})
		t.dataStats.addUpload(int64(len(result.Data)))
	case <-t.stopc:
		return errors.New("canceled by stop")
	}
	return nil
}

func (t *Task) writeBlock(b *common.Block) error {
	resultc := t.storage.WriteBlock(t.torrent.Info.Hash.String(), b)
	select {
	case result := <-resultc:
		if result.Error != nil {
			return result.Error
		}
	case <-t.stopc:
		return errors.New("canceled by stop")
	}

	t.dataStats.addDownload(int64(len(b.Data)))
	return nil
}

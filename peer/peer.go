package peer

import (
	"errors"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/wqsa/bget/bencode"
	"github.com/wqsa/bget/common/bitmap"
)

const (
	//PeerIDLen is length of peer id
	PeerIDLen       = 20
	compressPeerLen = 6

	stateInit = 0 + iota
	stateReady
	stateRunning
	stateStop
)

var (
	errBaseLen    = errors.New("insufficient data for base length type")
	errPeerFormat = errors.New("peer format error")
)

type peer struct {
	id            [PeerIDLen]byte
	addr          string
	conn          net.Conn
	fastExtension bool
	isInterest    bool
	isChoke       bool
	fastSet       []int
	state         int
	stateLock     sync.RWMutex
	lastActive    time.Time
	Bitmap        *bitmap.Bitmap
	closing       chan struct{}
	Download      int64
	Upload        int64
	haveReq       bool
}

func newPeer(id []byte, addr string) *peer {
	if id != nil && len(id) != PeerIDLen {
		return nil
	}
	peer := &peer{addr: addr, state: stateInit, closing: make(chan struct{})}
	if id != nil {
		copy(peer.id[:], id)
	}
	return peer
}

func (p *peer) getState() int {
	p.stateLock.RLock()
	defer p.stateLock.RUnlock()
	return p.state
}

func (p *peer) setState(state int) {
	p.stateLock.Lock()
	defer p.stateLock.Unlock()
	p.state = state
	return
}

func (p *peer) UnmarshalBencode(data []byte) error {
	var content map[string]interface{}
	err := bencode.Unmarshal(data, &content)
	if err != nil {
		return err
	}
	id, ok := content["peer id"].(string)
	if !ok || len(id) != PeerIDLen {
		return errPeerFormat
	}
	copy(p.id[:], []byte(id))
	ip, ok := content["ip"].(string)
	if !ok {
		return errPeerFormat
	}
	//check wheather port vaild is?
	port, ok := content["port"].(int)
	if !ok {
		return errPeerFormat
	}
	p.addr = net.JoinHostPort(ip, strconv.Itoa(port))
	return nil
}

type messageWithID struct {
	PeerID string
	ID     msgID
	Data   interface{}
}

func (p *peer) recvMessage(msgc chan<- *messageWithID) {
	for {
		m, err := p.readMessage()
		if err != nil {
			p.close()
			return
		}
		if m.Length == 0 {
			p.lastActive = time.Now()
			continue
		}
		msg, err := p.dispatch(m)
		if err != nil {
			p.close()
			return
		}
		select {
		case msgc <- &msg:
		case <-p.closing:
			p.close()
			return
		}
	}
}

func (p *peer) close() error {
	p.stateLock.Lock()
	defer p.stateLock.Unlock()
	p.state = stateStop
	err := p.conn.Close()
	p.conn = nil
	return err
}

func (p *peer) Close() error {
	p.stateLock.RLock()
	defer p.stateLock.RUnlock()
	if p.state != stateRunning {
		return nil
	}
	close(p.closing)
	//p.closing <- struct{}{}
	return nil
}

func isChoke(err error) bool {
	return err == errChoke
}

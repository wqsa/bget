package peer

import (
	"errors"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/wqsa/bget/bencode"
	"github.com/wqsa/bget/common/bitmap"
	"github.com/wqsa/bget/meta"
)

const (
	//PeerIDLen is length of Peer id
	PeerIDLen       = 20
	compressPeerLen = 6

	stateInit = 0 + iota
	stateReady
	stateRunning
	stateStop
)

var (
	errBaseLen    = errors.New("insufficient data for base length type")
	errPeerFormat = errors.New("Peer format error")
)

type Peer struct {
	id            [PeerIDLen]byte
	addr          string
	infoHash      meta.Hash
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
	writec        chan msgMarshaler
	readC         chan *message
}

func (p *Peer) Addr() string {
	return p.addr
}
func newPeer(id []byte, addr string) (*Peer, error) {
	if id != nil && len(id) != PeerIDLen {
		return nil, errors.New("bad peer id")
	}
	Peer := &Peer{addr: addr, state: stateInit, closing: make(chan struct{})}
	if id != nil {
		copy(Peer.id[:], id)
	}
	return Peer, nil
}

func (p *Peer) getState() int {
	p.stateLock.RLock()
	defer p.stateLock.RUnlock()
	return p.state
}

func (p *Peer) setState(state int) {
	p.stateLock.Lock()
	defer p.stateLock.Unlock()
	p.state = state
	return
}

func (p *Peer) UnmarshalBencode(data []byte) error {
	var content map[string]interface{}
	err := bencode.Unmarshal(data, &content)
	if err != nil {
		return err
	}
	id, ok := content["Peer id"].(string)
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

func (p *Peer) close() error {
	p.stateLock.Lock()
	defer p.stateLock.Unlock()
	p.state = stateStop
	err := p.conn.Close()
	p.conn = nil
	return err
}

func (p *Peer) Close() error {
	p.stateLock.RLock()
	defer p.stateLock.RUnlock()
	if p.state != stateRunning {
		return nil
	}
	close(p.closing)
	//p.closing <- struct{}{}
	return nil
}

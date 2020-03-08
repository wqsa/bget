package peer

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/google/logger"

	jww "github.com/spf13/jwalterweatherman"
	"github.com/wqsa/bget/common"
	"github.com/wqsa/bget/common/bitmap"
	"github.com/wqsa/bget/meta"
	"github.com/wqsa/bget/tracker"
)

const (
	serverID = ""

	peerTimeout    = 5 * time.Second
	requestTimeOut = 10 * time.Second

	maxUnchoke       = 4
	chokeUpdate      = 10 * time.Second
	optimChokeUpdate = 30 * time.Second

	maxSavetimeval = 30 * time.Second

	fastSetSize = 10

	taskInit = iota
	taskUploadOnly
	taskReady
	taskRunning
	taskStopErr
)

var (
	errCreateServer = errors.New("create Peer server fail")
	errNotFound     = errors.New("not find")
	errUnknowMsaage = errors.New("unknow message")
)

type Request struct {
	common.BlockInfo
	createAt time.Time
}

func newRequest(data []byte) *Request {
	if len(data) != 12 {
		return nil
	}
	return &Request{
		BlockInfo: common.BlockInfo{
			Index:  int(binary.BigEndian.Uint32(data)),
			Begin:  int(binary.BigEndian.Uint32(data[uint32Len:])),
			Length: int(binary.BigEndian.Uint32(data[2*uint32Len:])),
		},
		createAt: time.Now(),
	}
}

func (r *Request) hash() string {
	return r.BlockInfo.Hash()
}

//Status is downlod stautus
type Status struct {
	PeerNum int
	Status  *tracker.AnnounceStats
}

//Manager manage peers of the task
type Manager struct {
	hash        meta.Hash
	pieceNum    int
	pieceLength int

	unchoke  [maxUnchoke]string
	interest map[string]bool
	peers    map[string]*Peer

	requests      map[int]Request
	rejectRequest map[int]Request
	peerRequest   map[string][]*Peer
	requestc      chan Request

	download  int64
	upload    int64
	left      int64
	state     int
	stateLock sync.RWMutex

	need   *bitmap.Bitmap
	finish *bitmap.Bitmap

	blockc   chan *common.Block
	peerc    chan *Peer
	messagec chan *message
	save     chan struct{}
	closec   chan struct{}
	memc     chan int64
	datac    chan *common.Block
}

//NewManager return a Manager instance
func NewManager(torrent *meta.Torrent, path string, memc chan int64) *Manager {
	pieceNum := len(torrent.Info.Pieces) / 20
	m := &Manager{
		hash:        torrent.Info.Hash,
		pieceNum:    pieceNum,
		pieceLength: torrent.Info.PieceLength,
		// need:          need,
		finish:        bitmap.NewEmptyBitmap(pieceNum),
		peers:         make(map[string]*Peer),
		interest:      make(map[string]bool),
		requests:      make(map[int]Request),
		rejectRequest: make(map[int]Request),
		peerRequest:   make(map[string][]*Peer),
		blockc:        make(chan *common.Block),
		peerc:         make(chan *Peer),
		messagec:      make(chan *message),
		save:          make(chan struct{}),
		closec:        make(chan struct{}),
		memc:          memc,
	}
	return m
}

//Run start peers's work
func (m *Manager) Run() (err error) {
	choke := time.Tick(chokeUpdate)
	optimistic := time.Tick(optimChokeUpdate)
	m.setState(taskRunning)
	for {
		select {
		case p := <-m.peerc:
			logger.Infof("add Peer %v\n", p.addr)
			m.peers[p.addr] = p
			m.initPeer(p)
		case msg := <-m.messagec:
			p, ok := m.peers[msg.addr]
			if !ok {
				jww.FATAL.Panicf("Peer %v can't find\n", msg.addr)
			}
			if err = m.handleMessage(p, msg); err != nil {
				m.close()
				continue
			}
		case block := <-m.datac:
			m.replyRequest(block)
		case <-choke:
			logger.Infof("update choke")
			m.updateUnchoke()
			m.update()
			if m.left == 0 {

			}
		case <-optimistic:
			logger.Infoln("optimistic unchoke")
			m.optimUnchoke()
			logger.Infoln("optimistic end")
		case <-m.closec:
			return
		}
	}
}

func (m *Manager) ReplyRequest(block *common.Block) {
	select {
	case m.datac <- block:
	case <-m.closec:
		return
	}
}

func (m *Manager) replyRequest(block *common.Block) {
	req := &Request{
		BlockInfo: block.BlockInfo,
	}
	if peers, ok := m.peerRequest[req.hash()]; ok {
		for _, p := range peers {
			p.SendMessage(context.TODO(), &messagePiece{begin: block.Begin,
				index: block.Index,
				data:  block.Data})
		}
	}
}

func (m *Manager) serverPeer(p *Peer) {
	for {
		msg, err := p.ReadMessage(context.TODO())
		if err != nil {
			jww.INFO.Println("read message error:", err)
			p.Close()
		}
		m.messagec <- msg
	}
}

func (m *Manager) update() {
	// if time.Now().Sub(m.lastSave) > maxSavetimeval {
	// 	go func() {
	// 		l, err := m.fs.Dump()
	// 		if err != nil {
	// 			m.Close()
	// 		}
	// 		select {
	// 		case m.memc <- l:
	// 			m.lastSave = time.Now()
	// 		case <-m.closec:
	// 			return
	// 		}
	// 	}()
	// }
}

//AddPeer add Peer to peers
// func (m *Manager) AddPeer(ctx context.Context, conn net.Conn, extension [reserved]byte) {
// 	if m.getState() != taskRunning {
// 		return
// 	}
// 	p := Peer{
// 		conn:          conn,
// 		addr:          conn.RemoteAddr().String(),
// 		fastExtension: extension[7]&0x04 != 0,
// 		isInterest:    false,
// 		isChoke:       true,
// 		state:         stateRunning,
// 		lastActive:    time.Now(),
// 		closing:       make(chan struct{}),
// 	}
// 	select {
// 	case m.peerc <- &p:
// 	case <-m.closing:
// 		return
// 	}
// }

//AddRawPeer add Peer by addr
func (m *Manager) AddRawPeer(addrs []string) {
	if m.getState() != taskRunning {
		return
	}
	var wg sync.WaitGroup
	for _, addr := range addrs {
		wg.Add(1)
		addr := addr
		go func() {
			defer wg.Done()
			p, err := newPeer(nil, addr)
			if err != nil {
				jww.TRACE.Printf("Peer %v init fail\n", addr)
				return
			}
			if err := p.dial(); err != nil {
				jww.TRACE.Printf("peer dail failed: %+v\n", err)
			}
			select {
			case m.peerc <- p:
			case <-m.closec:
				return
			}
		}()
	}
	wg.Wait()
}

//GetStatus return downalod status
func (m *Manager) GetStatus() *Status {
	// if m.getState() != taskRunning {
	// 	return nil
	// }
	// statusc := make(chan *Status)
	// select {
	// case m.getStatus <- statusc:
	// 	select {
	// 	case status := <-statusc:
	// 		return status
	// 	case <-m.closec:
	// 		return nil
	// 	}
	// case <-m.closec:
	// 	return nil
	// }
	return nil
}

func (m *Manager) GetRequest() <-chan Request {
	return m.requestc
}

func (m *Manager) GetData() <-chan common.Block {
	return nil
}

func (m *Manager) getState() int {
	m.stateLock.RLock()
	defer m.stateLock.RUnlock()
	return m.state
}

func (m *Manager) setState(state int) {
	m.stateLock.Lock()
	m.state = state
	m.stateLock.Unlock()
	return
}

func (m *Manager) close() error {
	logger.Errorln("Peer manager closing")
	for _, p := range m.peers {
		if p.getState() == stateRunning {
			p.Close()
		}
	}
	close(m.closec)
	m.setState(taskInit)
	return nil
}

//Close close then manager
func (m *Manager) Close() error {
	if m.getState() != taskRunning {
		return nil
	}
	// errc := make(chan error)
	// m.closing <- errc
	// err := <-errc
	m.close()
	return nil
}

func (m *Manager) initPeer(p *Peer) {
	p.Bitmap = bitmap.NewEmptyBitmap(m.pieceNum)
	if p.fastExtension {
		m.generateFastSet(p)
		for _, v := range p.fastSet {
			p.SendMessage(context.TODO(), &messageAllowedFast{v})
		}
	}
	//TODO may be should check bitmap
	if m.download < int64(m.pieceLength) {
		p.SendMessage(context.TODO(), new(messageHaveNone))
	} else if m.left == 0 {
		p.SendMessage(context.TODO(), new(messageHaveAll))
	} else {
		p.SendMessage(context.TODO(), &messageBitfield{m.finish})
	}
}

func (m *Manager) have(index int) {
	for _, p := range m.peers {
		p.SendMessage(context.TODO(), new(messageHave))
	}
}

func (m *Manager) updateUnchoke() {
	//too foolish
	ps := []*Peer{}
	for k := range m.interest {
		p, ok := m.peers[k]
		if !ok {
			logger.Fatalln("can't find the Peer")
		}
		if p.getState() == stateRunning {
			ps = append(ps, p)
		} else {
			delete(m.interest, k)
		}
	}
	sort.Slice(ps, func(i, j int) bool {
		if m.left != 0 {
			return ps[i].Download > ps[j].Download
		}
		return ps[i].Upload > ps[j].Upload
	})
	n := 0
	if len(ps) < maxUnchoke {
		n = len(ps)
	} else {
		n = maxUnchoke
	}
	t := n
	for _, p := range ps[:n] {
		h := -1
		for i, v := range m.unchoke {
			if v == p.addr {
				h = i
			}
		}
		if h == -1 {
			p.SendMessage(context.TODO(), new(messageUnchoke))
		} else {
			copy(m.unchoke[:], m.unchoke[:h])
			copy(m.unchoke[:h], m.unchoke[h+1:])
			t--
		}
	}
	for i := 0; i < t; i++ {
		if p, ok := m.peers[m.unchoke[i]]; ok {
			p.SendMessage(context.TODO(), new(messageChoke))
		}
	}
	for i := range ps[:n] {
		m.unchoke[i] = ps[i].addr
	}
}

func (m *Manager) optimUnchoke() {
	for k, p := range m.peers {
		if k != m.unchoke[maxUnchoke-1] && !m.interest[k] {
			if p.getState() == stateInit {
				p.SendMessage(context.TODO(), new(messageUnchoke))
				m.unchoke[maxUnchoke-1] = k
			}
			return
		}
	}
}

func (m *Manager) handleMessage(p *Peer, msg *message) (err error) {
	logger.Infof("handle message:%#v from %v\n", msg.ID, p.addr)
	switch msgID(msg.ID) {
	case msgChoke:
		err = m.handleChoke(p)
	case msgUnchoke:
		err = m.handleUnchoke(p)
	case msgBitfield:
		bitmap := bitmap.NewBitmap(msg.body)
		err = m.handleField(p, bitmap)
	case msgPiece:
		// block := newBlock(msg.body)
		// err = m.handlePiece(p, block)
	case msgRequest:
		req := newRequest(msg.body)
		err = m.handleReq(p, req)
	case msgCancel:
		req := newRequest(msg.body)
		err = m.handleCancel(p, req)
	case msgHave:
		err = m.handleHave(p, int(binary.BigEndian.Uint32(msg.body)))
	case msgHaveAll:
		b := bitmap.NewEmptyBitmap(m.pieceNum)
		b.SetRangeBitOn(0, b.Len())
		err = m.handleField(p, b)
	case msgHaveNone:
		b := bitmap.NewEmptyBitmap(m.pieceNum)
		err = m.handleField(p, b)
	case msgSuggestPiece:
		err = m.handelSuggestPiece(p, int(binary.BigEndian.Uint32(msg.body)))
	case msgRejectRequest:
		req := newRequest(msg.body)
		err = m.handleRejectReq(p, req)
	case msgAllowedFast:
		err = m.handleAllowedFast(p, int(binary.BigEndian.Uint32(msg.body)))
	default:
		logger.Errorln("unknow message", msg.ID)
		err = errUnknowMsaage
	}
	return
}

func (m *Manager) handleChoke(p *Peer) error {
	p.isChoke = true
	return nil
}

func (m *Manager) handleUnchoke(p *Peer) error {
	p.isChoke = false
	if !m.interest[p.addr] {
		if !m.isInterest(p) {
			return nil
		}
		m.interest[p.addr] = true
		p.SendMessage(context.TODO(), new(messageInterest))
	}
	index := m.selectPiece(p)
	if index == -1 {
		logger.Fatalln("interest but no piece select")
	}
	m.requestPiece(p, index)
	return nil
}

func (m *Manager) requestPiece(p *Peer, index int) {
	if p.haveReq {
		return
	}
	r, ok := m.requests[index]
	if ok {
		if r.Begin+r.Length >= m.pieceLength {
			delete(m.requests, index)
			goto requestNew
		}
		m.nextRequest(&r)
		p.SendMessage(context.TODO(), &messageRequest{
			index:  r.Index,
			begin:  r.Begin,
			length: maxRequestLength,
		})
		m.requests[index] = r
		p.haveReq = true
		return
	}
	r, ok = m.rejectRequest[index]
	if ok {
		p.SendMessage(context.TODO(), &messageRequest{
			index:  r.Index,
			begin:  r.Begin,
			length: maxRequestLength,
		})
		delete(m.rejectRequest, index)
		r.createAt = time.Now()
		m.requests[index] = r
		p.haveReq = true
		return
	}
requestNew:
	r = Request{
		BlockInfo: common.BlockInfo{
			Begin:  0,
			Index:  index,
			Length: maxRequestLength,
		},
		createAt: time.Now(),
	}
	p.SendMessage(context.TODO(), &messageRequest{
		index:  index,
		begin:  0,
		length: maxRequestLength,
	})
	m.requests[index] = r
	p.haveReq = true
}

func (m *Manager) nextRequest(r *Request) {
	if r.Begin+r.Length >= m.pieceLength {
		logger.Fatalln("this is last request")
	}
	r.Begin = r.Begin + r.Length
	if r.Begin+r.Length < m.pieceLength-maxRequestLength {
		r.Length = maxRequestLength
	} else {
		r.Length = m.pieceLength - r.Begin - r.Length
	}
}

func (m *Manager) handleInterset(p *Peer) error {
	p.isInterest = true
	return nil
}

func (m *Manager) handleNoInterest(p *Peer) error {
	p.isInterest = false
	return nil
}

func (m *Manager) handleHave(p *Peer, index int) error {
	if index < 0 || index > m.pieceNum {
		p.Close()
		return nil
	}
	p.Bitmap.SetBitOn(int(index))
	if p.isChoke {
		return nil
	}
	if !m.interest[p.addr] && m.need.MustGetBit(index) && !p.isChoke {
		m.interest[p.addr] = true
		p.SendMessage(context.TODO(), new(messageInterest))
		m.requestPiece(p, index)
	}
	return nil
}

func (m *Manager) handleField(p *Peer, field *bitmap.Bitmap) error {
	if field.Len() != m.pieceNum {
		if field.Len()/8 == (m.pieceNum-1)/8+1 && field.MustCountBitOn(m.pieceNum, field.Len()) == 0 {
			field.Truncate(m.pieceNum)
		} else {
			p.Close()
			return nil
		}
	}
	p.Bitmap.Assign(field)
	if m.isInterest(p) {
		m.interest[p.addr] = true
		p.SendMessage(context.TODO(), new(messageInterest))
	}
	return nil
}

func (m *Manager) handlePiece(p *Peer, b *common.Block) error {
	select {
	case m.datac <- b:
	case <-m.closec:
	}
	return nil
}

func (m *Manager) handleReq(p *Peer, r *Request) (err error) {
	if r.Index < 0 || r.Index > m.pieceNum {
		jww.TRACE.Printf("bad request, piece:%v, but piece num:%v\n", r.Index, m.pieceNum)
		p.Close()
		return
	}
	for _, v := range m.unchoke {
		if v == p.addr {
			if !m.finish.MustGetBit(r.Index) {
				break
			}
			m.peerRequest[r.hash()] = append(m.peerRequest[r.hash()], p)
			select {
			case m.requestc <- *r:
			case <-m.closec:
			}
			return
		}
	}
	p.SendMessage(context.TODO(), &messageRejectRequest{
		index:  r.Index,
		begin:  r.Begin,
		length: r.Length,
	})
	return
}

func (m *Manager) handleCancel(p *Peer, r *Request) (err error) {
	if r.Index < 0 || r.Index > m.pieceNum {
		p.Close()
		return
	}
	h := r.hash()
	if _, ok := m.peerRequest[h]; !ok {
		p.Close()
	}
	delete(m.peerRequest, h)
	return
}

func (m *Manager) handelSuggestPiece(p *Peer, index int) (err error) {
	if index < 0 || index > m.pieceNum {
		return
	}
	if p.Bitmap.MustGetBit(index) {
		m.requestPiece(p, index)
	}
	return
}

func (m *Manager) handleRejectReq(p *Peer, r *Request) (err error) {
	if r.Index < 0 || r.Index > m.pieceNum {
		return
	}
	if _, ok := m.requests[r.Index]; !ok {
		p.Close()
		return
	}
	m.rejectRequest[r.Index] = *r
	return
}

func (m *Manager) handleAllowedFast(p *Peer, index int) (err error) {
	if index < 0 || index > m.pieceNum {
		return
	}
	if p.Bitmap.MustGetBit(index) {
		m.requestPiece(p, index)
	}
	return
}

func (m *Manager) selectPiece(p *Peer) int {
	if m.state != taskRunning {
		return -1
	}
	for i := range m.rejectRequest {
		if p.Bitmap.MustGetBit(i) {
			return i
		}
	}
	pos := rand.Intn(m.pieceNum)
	for i := pos; i-pos < m.pieceNum; i++ {
		index := i % m.pieceNum
		if r, ok := m.requests[index]; ok {
			if time.Now().Sub(r.createAt) > requestTimeOut {
				delete(m.requests, index)
				m.rejectRequest[index] = r
				return index
			}
			continue
		}
		if m.need.MustGetBit(index) && p.Bitmap.MustGetBit(index) {
			return index
		}
	}
	return -1
}

func (m *Manager) isInterest(p *Peer) bool {
	interest := m.need.And(p.Bitmap)
	return interest.MustCountBitOn(0, interest.Len()) > 0
}

//according to http://www.bittorrenps.org/beps/bep_0006.html
func (m *Manager) generateFastSet(p *Peer) {
	ip, _, err := net.SplitHostPort(p.addr)
	if err != nil {
		panic(err)
	}
	i := int(binary.BigEndian.Uint32(net.ParseIP(ip)))
	i &= 0xffffff00
	x := make([]byte, uint32Len)
	binary.BigEndian.PutUint32(x, uint32(i))
	x = append(x, m.hash[:]...)
	for len(p.fastSet) < fastSetSize {
		h := sha1.Sum(x)
		x = h[:]
		for i := 0; i < 5 && len(p.fastSet) < fastSetSize; i += 4 {
			y := x[i : i+4]
			index := binary.BigEndian.Uint32(y)
			index = index % uint32(m.pieceLength)
			have := false
			for _, v := range p.fastSet {
				if v == int(index) {
					have = true
					break
				}
			}
			if !have {
				p.fastSet = append(p.fastSet, int(index))
			}
		}
	}
}

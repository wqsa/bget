package peer

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/google/logger"

	"github.com/wqsa/bget/common/bitmap"
	"github.com/wqsa/bget/filesystem"
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
	errCreateServer = errors.New("create peer server fail")
	errNotFound     = errors.New("not find")
	errUnknowMsaage = errors.New("unknow message")
)

type request struct {
	index    int
	begin    int
	length   int
	createAt time.Time
}

func newRequest(data []byte) *request {
	if len(data) != 12 {
		return nil
	}
	index, offset, _ := unpackUint32(data, 0)
	begin, offset, _ := unpackUint32(data, offset)
	length, offset, _ := unpackUint32(data, offset)
	return &request{int(index), int(begin), int(length), time.Now()}
}

func (r *request) hash() string {
	return fmt.Sprintf("%v%v%v", r.index, r.begin, r.length)
}

//Status is downlod stautus
type Status struct {
	PeerNum int
	Status  *tracker.DownloadStatus
}

//Manager manage peers of the task
type Manager struct {
	hash          meta.Hash
	unchoke       [maxUnchoke]string
	interest      map[string]bool
	peers         map[string]*peer
	fs            *filesystem.FileSystem
	requests      map[int]request
	rejectRequest map[int]request
	peerRequest   map[string][]*peer
	pieceNum      int
	pieceLength   int
	download      int64
	upload        int64
	left          int64
	state         int
	stateLock     sync.RWMutex
	need          *bitmap.Bitmap
	finish        *bitmap.Bitmap
	blockc        chan *filesystem.Block
	getStatus     chan chan<- *Status
	peerc         chan *peer
	messagec      chan *messageWithID
	save          chan struct{}
	closing       chan chan error
	closec        chan struct{}
	memc          chan int64
	lastSave      time.Time
}

//NewManager return a Manager instance
func NewManager(torrent *meta.Torrent, path string, memc chan int64) *Manager {
	pieceNum := len(torrent.Info.Pieces) / 20
	fs := filesystem.NewFileSystem(torrent, path)
	if fs == nil {
		return nil
	}
	need := fs.Need()
	m := &Manager{
		hash:          torrent.Info.Hash,
		pieceNum:      pieceNum,
		pieceLength:   torrent.Info.PieceLength,
		need:          need,
		finish:        bitmap.NewEmptyBitmap(pieceNum),
		fs:            fs,
		peers:         make(map[string]*peer),
		interest:      make(map[string]bool),
		requests:      make(map[int]request),
		rejectRequest: make(map[int]request),
		peerRequest:   make(map[string][]*peer),
		blockc:        make(chan *filesystem.Block),
		getStatus:     make(chan chan<- *Status),
		peerc:         make(chan *peer),
		messagec:      make(chan *messageWithID),
		save:          make(chan struct{}),
		closing:       make(chan chan error),
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
			if p.getState() == stateReady {
				logger.Infof("add peer %v\n", p.addr)
				m.peers[p.addr] = p
				m.initPeer(p)
				p.stateLock.Lock()
				p.state = stateRunning
				go p.recvMessage(m.messagec)
				p.stateLock.Unlock()
			} else {
				go func() {
					err := p.handshake(m.hash)
					if err != nil {
						return
					}
					p.setState(stateReady)
					select {
					case m.peerc <- p:
					case <-m.closing:
						return
					}
				}()
			}
		case msg := <-m.messagec:
			p, ok := m.peers[msg.PeerID]
			if !ok {
				logger.Fatalf("peer %v can;t find\n", msg.PeerID)
			}
			if ok && (p.getState() == stateRunning) {
				err = m.handleMessage(p, msg)
				if err != nil {
					m.close()
					return
				}
			} else {
				//logger.Fatalf("%v,state:%v but recive message\n", p.addr, p.getState())
			}
		case b := <-m.blockc:
			r := request{index: b.Index, begin: b.Begin, length: len(b.Data)}
			peers, ok := m.peerRequest[r.hash()]
			if !ok {
				return
			}
			for _, p := range peers {
				p.piece(b.Index, b.Begin, b.Data)
				p.Upload += int64(len(b.Data))
				m.upload += int64(len(b.Data))
			}
		case statusc := <-m.getStatus:
			logger.Infoln("send status to tracker")
			n := 0
			for _, p := range m.peers {
				if p.getState() == stateRunning {
					n++
				}
			}
			statusc <- &Status{
				PeerNum: n,
				Status: &tracker.DownloadStatus{
					Download: m.download,
					Upload:   m.upload,
					Left:     m.left,
				},
			}
			logger.Infoln("send status end")
		case <-m.save:
			l, err := m.fs.Dump()
			select {
			case m.memc <- -l:
			case errc := <-m.closing:
				errc <- err
				return err
			}
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
		case errc := <-m.closing:
			errc <- err
			return
		}
	}
}

func (m *Manager) update() {
	if time.Now().Sub(m.lastSave) > maxSavetimeval {
		go func() {
			l, err := m.fs.Dump()
			if err != nil {
				m.Close()
			}
			select {
			case m.memc <- l:
				m.lastSave = time.Now()
			case <-m.closec:
				return
			}
		}()
	}
}

//AddPeer add peer to peers
func (m *Manager) AddPeer(ctx context.Context, conn net.Conn, extension [reserved]byte) {
	if m.getState() != taskRunning {
		return
	}
	p := peer{
		conn:          conn,
		addr:          conn.RemoteAddr().String(),
		fastExtension: extension[7]&0x04 != 0,
		isInterest:    false,
		isChoke:       true,
		state:         stateRunning,
		lastActive:    time.Now(),
		closing:       make(chan struct{}),
	}
	select {
	case m.peerc <- &p:
	case <-m.closing:
		return
	}
}

//AddAddrPeer add peer from addr
func (m *Manager) AddAddrPeer(addrs []string) {
	if m.getState() != taskRunning {
		return
	}
	var wg sync.WaitGroup
	for _, addr := range addrs {
		wg.Add(1)
		addr := addr
		go func() {
			p := newPeer(nil, addr)
			defer wg.Done()
			if p == nil {
				logger.Errorf("peer %v init fail\n", addr)
				return
			}
			err := p.handshake(m.hash)
			if err != nil {
				logger.Warningf("%v handshake fail, %v\n", p.addr, err)
				return
			}
			err = p.recvHandshake(m.hash)
			if err != nil {
				logger.Warningf("%v check handshake fail, %v\n", p.addr, err)
				return
			}
			p.setState(stateReady)
			select {
			case m.peerc <- p:
			case <-m.closing:
				return
			}
		}()
	}
	wg.Wait()
}

//GetStatus return downalod status
func (m *Manager) GetStatus() *Status {
	if m.getState() != taskRunning {
		return nil
	}
	statusc := make(chan *Status)
	select {
	case m.getStatus <- statusc:
		select {
		case status := <-statusc:
			return status
		case <-m.closec:
			return nil
		}
	case <-m.closec:
		return nil
	}
}

//Save save data from memory to disk
func (m *Manager) Save() {
	if m.getState() != taskRunning {
		return
	}
	select {
	case m.save <- struct{}{}:
	case <-m.closec:
		return
	}
	return
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
	logger.Errorln("peer manager closing")
	for _, p := range m.peers {
		if p.getState() == stateRunning {
			p.Close()
		}
	}
	close(m.closing)
	m.setState(taskInit)
	return nil
}

//Close close then manager
func (m *Manager) Close() error {
	if m.getState() != taskRunning {
		return nil
	}
	errc := make(chan error)
	m.closing <- errc
	err := <-errc
	m.close()
	return err
}

func (m *Manager) initPeer(p *peer) {
	p.Bitmap = bitmap.NewEmptyBitmap(m.pieceNum)
	if p.fastExtension {
		m.generateFastSet(p)
		for _, v := range p.fastSet {
			p.allowedFast(v)
		}
		if m.download < int64(m.pieceLength) {
			p.haveNone()
		} else if m.left == 0 {
			p.haveAll()
		} else {
			p.bitfield(m.finish)
		}
	} else {
		p.bitfield(m.finish)
	}
}

func (m *Manager) have(index int) {
	for _, p := range m.peers {
		p.have(index)
	}
}

func (m *Manager) updateUnchoke() {
	//too foolish
	ps := []*peer{}
	for k := range m.interest {
		p, ok := m.peers[k]
		if !ok {
			logger.Fatalln("can't find the peer")
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
			p.unchoke()
		} else {
			copy(m.unchoke[:], m.unchoke[:h])
			copy(m.unchoke[:h], m.unchoke[h+1:])
			t--
		}
	}
	for i := 0; i < t; i++ {
		if p, ok := m.peers[m.unchoke[i]]; ok {
			p.choke()
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
				p.unchoke()
				m.unchoke[maxUnchoke-1] = k
			}
			return
		}
	}
}

func (m *Manager) handleMessage(p *peer, msg *messageWithID) (err error) {
	logger.Infof("handle message:%#v from %v\n", msg.ID, p.addr)
	switch msg.ID {
	case msgChoke:
		err = m.handleChoke(p)
	case msgUnchoke:
		err = m.handleUnchoke(p)
	case msgBitfield:
		err = m.handleField(p, msg.Data.(*bitmap.Bitmap))
	case msgPiece:
		err = m.handlePiece(p, msg.Data.(*filesystem.Block))
	case msgRequest:
		err = m.handleReq(p, msg.Data.(*request))
	case msgCancel:
		err = m.handleCancel(p, msg.Data.(*request))
	case msgHave:
		err = m.handleHave(p, msg.Data.(int))
	case msgHaveALL:
		b := bitmap.NewEmptyBitmap(m.pieceNum)
		b.SetRangeBitOn(0, b.Len())
		err = m.handleField(p, b)
	case msgHaveNone:
		b := bitmap.NewEmptyBitmap(m.pieceNum)
		err = m.handleField(p, b)
	case msgSuggestPiece:
		err = m.handelSuggestPiece(p, msg.Data.(int))
	case msgRejectRequest:
		err = m.handleRejectReq(p, msg.Data.(*request))
	case msgAllowedFast:
		err = m.handleAllowedFast(p, msg.Data.(int))
	default:
		logger.Errorln("unknow message", msg.ID)
		err = errUnknowMsaage
	}
	return
}

func (m *Manager) handleChoke(p *peer) error {
	p.isChoke = true
	return nil
}

func (m *Manager) handleUnchoke(p *peer) error {
	p.isChoke = false
	if !m.interest[p.addr] {
		if !m.isInterest(p) {
			return nil
		}
		m.interest[p.addr] = true
		p.interest()
	}
	index := m.selectPiece(p)
	if index == -1 {
		logger.Fatalln("interest but no piece select")
	}
	m.requestPiece(p, index)
	return nil
}

func (m *Manager) requestPiece(p *peer, index int) {
	if p.haveReq {
		return
	}
	r, ok := m.requests[index]
	if ok {
		if r.begin+r.length >= m.pieceLength {
			delete(m.requests, index)
			goto requestNew
		}
		m.nextRequest(&r)
		p.request(&r)
		m.requests[index] = r
		p.haveReq = true
		return
	}
	r, ok = m.rejectRequest[index]
	if ok {
		p.request(&r)
		delete(m.rejectRequest, index)
		r.createAt = time.Now()
		m.requests[index] = r
		p.haveReq = true
		return
	}
requestNew:
	r = request{index, 0, maxRequestLength, time.Now()}
	p.request(&r)
	m.requests[index] = r
	p.haveReq = true
}

func (m *Manager) nextRequest(r *request) {
	if r.begin+r.length >= m.pieceLength {
		logger.Fatalln("this is last request")
	}
	r.begin = r.begin + r.length
	if r.begin+r.length < m.pieceLength-maxRequestLength {
		r.length = maxRequestLength
	} else {
		r.length = m.pieceLength - r.begin - r.length
	}
}

func (m *Manager) handleInterset(p *peer) error {
	p.isInterest = true
	return nil
}

func (m *Manager) handleNoInterest(p *peer) error {
	p.isInterest = false
	return nil
}

func (m *Manager) handleHave(p *peer, index int) error {
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
		p.interest()
		m.requestPiece(p, index)
	}
	return nil
}

func (m *Manager) handleField(p *peer, field *bitmap.Bitmap) error {
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
		p.interest()
	}
	return nil
}

func (m *Manager) handlePiece(p *peer, b *filesystem.Block) error {
	l, err := m.fs.SavePiece(b.Index, b.Begin, b.Data)
	if err != nil {
		if err == filesystem.ErrInvaildIndex || err == filesystem.ErrWrongOrder {
			p.Close()
			return nil
		}
		if err == filesystem.ErrPieceCheck {
			select {
			case m.memc <- int64(-m.pieceLength):
			case <-m.closec:
				return nil
			}
			delete(m.requests, b.Index)
			return nil
		}
		return err
	}
	select {
	case m.memc <- int64(l):
	case <-m.closec:
		return nil
	}
	m.download += int64(l)
	if b.Begin+len(b.Data) >= m.pieceLength {
		m.finish.SetBitOn(b.Index)
		m.need.SetBitOff(b.Index)
	}
	m.requestPiece(p, b.Index)
	return nil
}

func (m *Manager) handleReq(p *peer, r *request) (err error) {
	if r.index < 0 || r.index > m.pieceNum {
		p.Close()
		return
	}
	if !m.finish.MustGetBit(r.index) {
		p.rejectRequest(r)
		return
	}
	find := false
	for _, v := range m.unchoke {
		if v == p.addr {
			find = true
		}
	}
	if !find {
		p.rejectRequest(r)
		return
	}

	h := r.hash()
	if _, ok := m.peerRequest[h]; ok {
		m.peerRequest[r.hash()] = append(m.peerRequest[r.hash()], p)
	} else {
		m.peerRequest[r.hash()] = []*peer{p}
	}
	go m.fs.RequestData(r.index, r.begin, r.length, m.blockc)
	return
}

func (m *Manager) handleCancel(p *peer, r *request) (err error) {
	if r.index < 0 || r.index > m.pieceNum {
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

func (m *Manager) handelSuggestPiece(p *peer, index int) (err error) {
	if index < 0 || index > m.pieceNum {
		return
	}
	if p.Bitmap.MustGetBit(index) {
		m.requestPiece(p, index)
	}
	return
}

func (m *Manager) handleRejectReq(p *peer, r *request) (err error) {
	if r.index < 0 || r.index > m.pieceNum {
		return
	}
	if _, ok := m.requests[r.index]; !ok {
		p.Close()
		return
	}
	m.rejectRequest[r.index] = *r
	return
}

func (m *Manager) handleAllowedFast(p *peer, index int) (err error) {
	if index < 0 || index > m.pieceNum {
		return
	}
	if p.Bitmap.MustGetBit(index) {
		m.requestPiece(p, index)
	}
	return
}

func (m *Manager) selectPiece(p *peer) int {
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

func (m *Manager) isInterest(p *peer) bool {
	interest := m.need.And(p.Bitmap)
	return interest.MustCountBitOn(0, interest.Len()) > 0
}

//according to http://www.bittorrenps.org/beps/bep_0006.html
func (m *Manager) generateFastSet(p *peer) {
	ip, _, err := net.SplitHostPort(p.addr)
	if err != nil {
		panic(err)
	}
	i, _, _ := unpackUint32(net.ParseIP(ip), 0)
	i &= 0xffffff00
	x := packUint32([]byte{}, i)
	x = append(x, m.hash[:]...)
	for len(p.fastSet) < fastSetSize {
		h := sha1.Sum(x)
		x = h[:]
		for i := 0; i < 5 && len(p.fastSet) < fastSetSize; i += 4 {
			y := x[i : i+4]
			index, _, _ := unpackUint32(y, 0)
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

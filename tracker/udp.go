package tracker

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/pkg/errors"

	"github.com/wqsa/bget/common"
	"github.com/wqsa/bget/meta"
)

const (
	protocolID int64 = 0x41727101980

	//connectRequest.action and connectResponse.action
	actionConnect  int32 = 0
	actionAnnounce int32 = 1

	//num of udp tracker return
	udpPeerNum = 74

	retransmitMax = 8

	peerLen = 6

	udpAnnounceRespHeadLen = 20

	udpBaseTimeout = 15
	udpWaitMaxTime = 60
	udpTryMaxTime  = 8
)

var (
	errBaseLen    = errors.New("insufficient data for base length type")
	errUDPResp    = errors.New("udp tracker resp error")
	errUDPNet     = errors.New("reach max retransmit")
	errUDPTimeOut = errors.New("udp connect timeout")
	errHTTPResp   = errors.New("http resp error")
	errAnnPara    = errors.New("announce parameter error")
)

//udpTracker is a tracker use UDP
type udpTracker struct {
	*url.URL
	connID     int64
	conn       net.Conn
	nextTime   time.Time
	RecvStatus chan AnnounceStats
}

type connectRequest struct {
	ProtocolID    int64
	Action        int32
	TransactionID int32
}

type connectResponse struct {
	Action    int32
	TranID    int32
	ConnectID int64
}

type udpAnnounceRequest struct {
	ConnectID int64
	Action    int32
	TranID    int32
	InfoHash  [sha1.Size]byte
	PeerID    [20]byte
	Download  int64
	Left      int64
	Upload    int64
	Event     int32
	IP        [4]byte
	Key       int32
	Want      int32
	Port      uint16
}

type announceResponse struct {
	Action   int32
	TranID   int32
	Interval int32
	Leechers int32
	Seeders  int32
}

func newudpTracker(u *url.URL) *udpTracker {
	return &udpTracker{URL: u}
}

func (t *udpTracker) connect() error {
	var err error
	t.conn, err = net.Dial("udp", t.Host)
	if err != nil {
		return err
	}
	tranID := rand.Int31()
	req := connectRequest{protocolID, actionConnect, tranID}
	data, err := t.request(req)
	if err != nil {
		return err
	}
	resp := new(connectResponse)
	r := bytes.NewReader(data)
	err = binary.Read(r, binary.BigEndian, resp)
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.Action != actionConnect || resp.TranID != tranID {
		return errUDPResp
	}
	t.connID = resp.ConnectID
	return nil
}

func (t *udpTracker) announce(hash meta.Hash, id []byte, addr string, event Event, stats *AnnounceStats) ([]string, error) {
	if t.conn == nil {
		err := t.connect()
		if err != nil {
			return nil, err
		}
	}
	ipStr, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	tranID := rand.Int31()
	req := udpAnnounceRequest{
		ConnectID: t.connID,
		InfoHash:  hash,
		Action:    actionAnnounce,
		TranID:    tranID,
		Download:  stats.Download,
		Left:      stats.Left,
		Upload:    stats.Upload,
		Event:     int32(event),
		Port:      uint16(port),
		Want:      -1,
	}
	copy(req.IP[:], net.ParseIP(ipStr))
	if req.Event == int32(EventNone) {
		if t.nextTime.After(time.Now()) {
			return nil, errWaitTime
		}
	}
	data, err := t.request(req)
	if err != nil {
		return nil, err
	}
	r := bytes.NewReader(data)
	resp := new(announceResponse)
	if err = binary.Read(r, binary.BigEndian, resp); err != nil {
		return nil, err
	}
	if resp.Action != actionAnnounce || resp.TranID != tranID {
		return nil, errUDPResp
	}
	t.nextTime = time.Now().Add(time.Duration(resp.Interval) * time.Second)
	if err != nil {
		return nil, err
	}
	addrs, err := common.ParseCompressIPV4Addrs(data[20:])
	if err != nil {
		return nil, errUDPResp
	}
	return addrs, nil
}

func (t *udpTracker) waitTime() time.Time {
	return t.nextTime
}

func (t *udpTracker) close() error {
	return t.conn.Close()
}

func (t *udpTracker) request(req interface{}) ([]byte, error) {
	err := binary.Write(t.conn, binary.BigEndian, req)
	if err != nil {
		return nil, err
	}
	timeout := udpBaseTimeout
	for {
		deadline := time.Now().Add(time.Duration(timeout) * time.Second)
		t.conn.SetReadDeadline(deadline)
		recv := make([]byte, 20+6*udpPeerNum)
		n, err := t.conn.Read(recv)
		if err != nil {
			if isTimeOut(err) {
				if timeout *= 2; timeout > udpWaitMaxTime {
					return nil, errUDPTimeOut
				}
				continue
			}
			return nil, errors.WithStack(err)
		}
		return recv[:n], nil
	}
}

func isTimeOut(err error) bool {
	netErr, ok := err.(net.Error)
	if !ok {
		return false
	}
	if netErr.Timeout() {
		return true
	}
	return false
}

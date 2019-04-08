package tracker

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"math/rand"
	"net"
	"net/url"
	"time"

	"../meta"
)

const (
	protocolID int64 = 0x41727101980

	//connectReq.action and connectResp.action
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

//UDPTracker is a tracker use UDP
type UDPTracker struct {
	url.URL
	connID     int64
	conn       net.Conn
	tranID     int32
	nextTime   time.Time
	RecvStatus chan DownloadStatus
}

type connectReq struct {
	ProtocolID    int64
	Action        int32
	TransactionID int32
}

type connectResp struct {
	Action    int32
	TranID    int32
	ConnectID int64
}

type annReq struct {
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

type annResp struct {
	Action   int32
	TranID   int32
	Interval int32
	Leechers int32
	Seeders  int32
	Peers    [udpPeerNum * 6]byte
}

//NewUDPTracker is used for create a udp tracker
func NewUDPTracker(rawurl string) *UDPTracker {
	url, err := url.Parse(rawurl)
	if err != nil {
		return nil
	}
	return &UDPTracker{URL: *url}
}

func (t *UDPTracker) getWaitTime() time.Time {
	return t.nextTime
}

//Announce is used to announce to tracker
func (t *UDPTracker) Announce(hash meta.Hash, id []byte, ip net.IP, port int, event int, status *DownloadStatus) ([]string, error) {
	if t.conn == nil {
		err := t.connect()
		if err != nil {
			return nil, err
		}
	}
	data, err := t.announce(hash, event, status)
	if err != nil {
		return nil, err
	}
	peers := convAddrs(data)
	if peers == nil {
		return nil, errUDPResp
	}
	return peers, nil
}

func (t *UDPTracker) close() error {
	return t.conn.Close()
}

func getTransID() int32 {
	return rand.Int31()
}

func (t *UDPTracker) connect() error {
	var err error
	t.conn, err = net.Dial("udp", t.Host)
	if err != nil {
		return err
	}
	t.tranID = getTransID()
	req := connectReq{protocolID, actionConnect, t.tranID}
	err = binary.Write(t.conn, binary.BigEndian, req)
	if err != nil {
		return err
	}

	var resp connectResp
	timeout := udpBaseTimeout
	for {
		deadline := time.Now().Add(time.Duration(timeout) * time.Second)
		t.conn.SetReadDeadline(deadline)
		err = binary.Read(t.conn, binary.BigEndian, &resp)
		if err == nil {
			break
		}
		if !isTimeOut(err) {
			return errUDPTimeOut
		}
		if timeout *= 2; timeout > udpWaitMaxTime {
			return err
		}
	}
	if resp.Action == actionConnect && t.tranID == resp.TranID {
		t.connID = resp.ConnectID
		return nil
	}
	return errUDPResp
}

func (t *UDPTracker) announce(hash meta.Hash, event int, status *DownloadStatus) ([]byte, error) {
	req := annReq{
		ConnectID: t.connID,
		Action:    actionAnnounce,
		TranID:    t.tranID,
		Key:       rand.Int31(),
		Download:  status.Download,
		Left:      status.Left,
		Upload:    status.Upload,
		Event:     int32(event),
		Want:      -1,
	}
	if req.Event == EventNone {
		if t.nextTime.After(time.Now()) {
			return nil, errWaitTime
		}
	}
	err := binary.Write(t.conn, binary.BigEndian, req)
	if err != nil {
		return nil, err
	}
	resp := annResp{}
	timeout := udpBaseTimeout
	for {
		deadline := time.Now().Add(time.Duration(timeout) * time.Second)
		t.conn.SetReadDeadline(deadline)
		err = binary.Read(t.conn, binary.BigEndian, &resp)
		if resp.TranID != 0 {
			break
		}
		if !isTimeOut(err) {
			return nil, err
		}
		if timeout *= 2; timeout > udpWaitMaxTime {
			return nil, errUDPTimeOut
		}
	}
	if resp.Action != actionAnnounce || resp.TranID != t.tranID {
		return nil, errUDPResp
	}
	t.nextTime = time.Now().Add(time.Duration(resp.Interval) * time.Second)
	return resp.Peers[:], nil
	//return peer.NewRawPeers(false, resp.Peers[:]), nil
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

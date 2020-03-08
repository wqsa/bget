package peer

import (
	"encoding/binary"
	"time"

	pool "github.com/libp2p/go-buffer-pool"
	"github.com/pkg/errors"
	"github.com/wqsa/bget/common/bitmap"
	"github.com/wqsa/bget/meta"
)

const (
	uint16Len = 2
	uint32Len = 4

	bitTorrent  = "BitTorrent protocol"
	protoLength = len(bitTorrent) + 1
	reserved    = 8

	maxRequestLength = 1024 * 16

	msgMaxLength        = 16*1024 + 9
	msgIDLen     uint32 = 1

	handshakeTimeOut = time.Second

	msgHeadLen = 5
)

const (
	msgChoke msgID = iota
	msgUnchoke
	msgInterest
	msgNoInterest
	msgHave
	msgBitfield
	msgRequest
	msgPiece
	msgCancel
	msgPort

	msgKeepAlive = -1

	//fast extension
	msgHaveAll       msgID = 0x0e
	msgHaveNone      msgID = 0x0f
	msgSuggestPiece  msgID = 0x0d
	msgRejectRequest msgID = 0x10
	msgAllowedFast   msgID = 0x11
)

var (
	handshakeHead [protoLength]byte
)

func init() {
	extension[7] |= 0x04
	//init stable message head
	copy(handshakeHead[:], append([]byte{byte(len(bitTorrent))}, []byte(bitTorrent)...))
}

type msgID byte

//handshakeMsg is used for handshake
type handshakeMsg struct {
	Head     [protoLength]byte
	Reserved [reserved]byte
	InfoHash meta.Hash
	PeerID   [PeerIDLen]byte
}

type messageHead struct {
	Length uint32
	ID     byte
}

type message struct {
	messageHead
	addr string
	body []byte
}

func unmarshalBinary(data []byte) (interface{}, error) {
	if len(data) == 0 {
		return nil, errors.New("bad message")
	}
	switch msgID(data[0]) {
	case msgChoke:
		return new(messageChoke), nil
	case msgUnchoke:
		return new(messageUnchoke), nil
	case msgInterest:
		return new(messageInterest), nil
	case msgNoInterest:
	case msgHave:
	case msgBitfield:
	case msgRequest:
	case msgPiece:
	case msgCancel:
	case msgPort:
	case msgHaveAll:
	case msgHaveNone:
	case msgSuggestPiece:
	case msgRejectRequest:
	case msgAllowedFast:
	default:
		return nil, errors.New("unknown message ID")
	}
	return nil, errors.New("unknown message ID")
}

type messageChoke struct {
}

func (m *messageChoke) marshalBinary() ([]byte, error) {
	return marshalIDMessage(msgChoke)
}

type messageUnchoke struct {
}

func (m *messageUnchoke) marshalBinary() ([]byte, error) {
	return marshalIDMessage(msgUnchoke)
}

type messageInterest struct {
}

func (m *messageInterest) marshalBinary() ([]byte, error) {
	return marshalIDMessage(msgInterest)
}

type messageNoInterest struct {
}

func (m *messageNoInterest) marshalBinary() ([]byte, error) {
	return marshalIDMessage(msgNoInterest)
}

type messageHave struct {
}

func (m *messageHave) marshalBinary() ([]byte, error) {
	return marshalIDMessage(msgHave)
}

type messageBitfield struct {
	bitfield *bitmap.Bitmap
}

func (m *messageBitfield) marshalBinary() ([]byte, error) {
	length := 1 + m.bitfield.Len()
	data := pool.Get(uint32Len + length)
	binary.BigEndian.PutUint32(data, uint32(length))
	copy(data[uint32Len:], m.bitfield.Bytes())
	return data, nil
}

func (m *messageBitfield) unmarshalBinary(data []byte) error {
	m.bitfield = bitmap.NewBitmap(data)
	return nil
}

type messageRequest struct {
	index  int
	begin  int
	length int
}

func (m *messageRequest) marshalBinary() ([]byte, error) {
	return marshalReqMessage(msgRequest, m.index, m.begin, m.length)
}

func (m *messageRequest) unmarshalBinary(data []byte) error {
	req := newRequest(data)
	m.index, m.begin, m.length = req.Index, req.Begin, req.Length
	pool.Put(data)
	return nil
}

type messagePiece struct {
	index int
	begin int
	data  []byte
}

func (m *messagePiece) marshalBinary() ([]byte, error) {
	length := 2*uint32Len + len(m.data)
	data := pool.Get(uint32Len + length)
	binary.BigEndian.PutUint32(data, uint32(length))
	binary.BigEndian.PutUint32(data, uint32(m.index))
	binary.BigEndian.PutUint32(data, uint32(m.begin))
	// binary.BigEndian.PutUint32(data, uint32(m.length))
	copy(data[3*uint32Len:], m.data)
	return data, nil
}

func (m *messagePiece) unmarshalBinary(data []byte) error {
	m.index = int(binary.BigEndian.Uint32(m.data))
	m.begin = int(binary.BigEndian.Uint32(m.data[uint32Len:]))
	m.data = data[2*uint32Len:]
	return nil
}

type messageCancel struct {
	index  int
	begin  int
	length int
}

func (m *messageCancel) marshalBinary() ([]byte, error) {
	return marshalReqMessage(msgCancel, m.index, m.begin, m.length)
}

func (m *messageCancel) unmarshalBinary(data []byte) error {
	req := newRequest(data)
	m.index, m.begin, m.length = req.Index, req.Begin, req.Length
	pool.Put(data)
	return nil
}

type messagePort struct {
	port int
}

func (m *messagePort) marshalBinary() ([]byte, error) {
	return marshalUint32Message(msgPort, uint32(m.port))
}

func (m *messagePort) unmarshalBinary(data []byte) error {
	m.port = int(binary.BigEndian.Uint32(data))
	return nil
}

type messageHaveAll struct {
}

func (m *messageHaveAll) marshalBinary() ([]byte, error) {
	return marshalIDMessage(msgHaveAll)
}

type messageHaveNone struct {
}

func (m *messageHaveNone) marshalBinary() ([]byte, error) {
	return marshalIDMessage(msgHaveNone)
}

type messageSuggestPiece struct {
	index int
}

func (m *messageSuggestPiece) marshalBinary() ([]byte, error) {
	return marshalUint32Message(msgSuggestPiece, uint32(m.index))
}

func (m *messageSuggestPiece) unmarshalBinary(data []byte) error {
	m.index = int(binary.BigEndian.Uint32(data))
	return nil
}

type messageRejectRequest struct {
	index  int
	begin  int
	length int
}

func (m *messageRejectRequest) unmarshalBinary(data []byte) error {
	req := newRequest(data)
	m.index, m.begin, m.length = req.Index, req.Begin, req.Length
	pool.Put(data)
	return nil
}

func (m *messageRejectRequest) marshalBinary() ([]byte, error) {
	return marshalReqMessage(msgRejectRequest, m.index, m.begin, m.length)
}

type messageAllowedFast struct {
	index int
}

func (m *messageAllowedFast) marshalBinary() ([]byte, error) {
	return marshalUint32Message(msgAllowedFast, uint32(m.index))
}

func (m *messageAllowedFast) unmarshalBinary(data []byte) error {
	m.index = int(binary.BigEndian.Uint32(data))
	return nil
}

func marshalIDMessage(id msgID) ([]byte, error) {
	switch id {
	case msgChoke, msgUnchoke, msgInterest, msgNoInterest, msgHave:
		data := pool.Get(uint32Len + 1)
		binary.BigEndian.PutUint32(data, 1)
		data[uint32Len] = byte(id)
		return data, nil
	}
	return nil, errors.New("unsupport msaage")
}

func marshalUint32Message(id msgID, value uint32) ([]byte, error) {
	switch id {
	case msgSuggestPiece, msgPort, msgAllowedFast:
		length := uint32Len
		data := pool.Get(uint32Len + length)
		binary.BigEndian.PutUint32(data, uint32(length))
		binary.BigEndian.PutUint32(data, value)
		return data, nil
	}
	return nil, errors.New("unsupport msaage")
}

func marshalReqMessage(id msgID, index, begin, length int) ([]byte, error) {
	switch id {
	case msgCancel, msgRequest, msgRejectRequest:
		length := uint32Len * 2
		data := pool.Get(uint32Len + length)
		binary.BigEndian.PutUint32(data, uint32(length))
		binary.BigEndian.PutUint32(data, uint32(index))
		binary.BigEndian.PutUint32(data, uint32(begin))
		binary.BigEndian.PutUint32(data, uint32(length))
		return data, nil

	}
	return nil, errors.New("unsupport message")
}

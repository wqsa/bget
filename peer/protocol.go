package peer

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"time"

	"github.com/wqsa/bget/common/bitmap"
	"github.com/wqsa/bget/filesystem"
	"github.com/wqsa/bget/meta"

	"github.com/gohugoio/hugo/bufferpool"
	"github.com/google/logger"
)

var (
	errPeerConnect   = errors.New("peer is disconnect")
	errProto         = errors.New("handshake fail, error protocol")
	errInfoHash      = errors.New("handshake fail, infoHash not match")
	errHandshakeResp = errors.New("peer handshake resp err")
	errMsgLength     = errors.New("message length is too long")
	errMsgID         = errors.New("unsupport message type")
	errChoke         = errors.New("peer is choke")
	errFastExtension = errors.New("not support fast extension")
	errType          = errors.New("unknow type")

	handshakeHead [protoLength]byte
	msgHead       = make(map[msgID][]byte)

	extension [reserved]byte
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
	msgHaveALL       msgID = 0x0e
	msgHaveNone      msgID = 0x0f
	msgSuggestPiece  msgID = 0x0d
	msgRejectRequest msgID = 0x10
	msgAllowedFast   msgID = 0x11
)

type msgID byte
type message struct {
	Length uint32
	ID     msgID
	body   []byte
}

func init() {
	extension[7] |= 0x04
	//init stable message head
	copy(handshakeHead[:], append([]byte{byte(len(bitTorrent))}, []byte(bitTorrent)...))

	buff := bufferpool.GetBuffer()
	defer bufferpool.PutBuffer(buff)
	binary.Write(buff, binary.BigEndian, uint32(1))
	binary.Write(buff, binary.BigEndian, msgChoke)
	msgHead[msgChoke] = append(msgHead[msgChoke], buff.Bytes()...)
	buff.Reset()
	binary.Write(buff, binary.BigEndian, uint32(1))
	binary.Write(buff, binary.BigEndian, msgUnchoke)
	msgHead[msgUnchoke] = append(msgHead[msgUnchoke], buff.Bytes()...)
	buff.Reset()
	binary.Write(buff, binary.BigEndian, uint32(1))
	binary.Write(buff, binary.BigEndian, msgInterest)
	msgHead[msgInterest] = append(msgHead[msgInterest], buff.Bytes()...)
	buff.Reset()
	binary.Write(buff, binary.BigEndian, uint32(1))
	binary.Write(buff, binary.BigEndian, msgNoInterest)
	msgHead[msgNoInterest] = append(msgHead[msgNoInterest], buff.Bytes()...)
	buff.Reset()
	binary.Write(buff, binary.BigEndian, uint32(5))
	binary.Write(buff, binary.BigEndian, msgHave)
	msgHead[msgHave] = append(msgHead[msgHave], buff.Bytes()...)
	buff.Reset()
	binary.Write(buff, binary.BigEndian, uint32(13))
	binary.Write(buff, binary.BigEndian, msgRequest)
	msgHead[msgRequest] = append(msgHead[msgRequest], buff.Bytes()...)
	buff.Reset()
	binary.Write(buff, binary.BigEndian, uint32(13))
	binary.Write(buff, binary.BigEndian, msgCancel)
	msgHead[msgCancel] = append(msgHead[msgCancel], buff.Bytes()...)
	buff.Reset()
	binary.Write(buff, binary.BigEndian, uint32(3))
	binary.Write(buff, binary.BigEndian, msgPort)
	msgHead[msgPort] = append(msgHead[msgPort], buff.Bytes()...)
	buff.Reset()
	binary.Write(buff, binary.BigEndian, uint32(1))
	binary.Write(buff, binary.BigEndian, msgHaveALL)
	msgHead[msgHaveALL] = append(msgHead[msgHaveALL], buff.Bytes()...)
	buff.Reset()
	binary.Write(buff, binary.BigEndian, uint32(1))
	binary.Write(buff, binary.BigEndian, msgHaveNone)
	msgHead[msgHaveNone] = append(msgHead[msgHaveNone], buff.Bytes()...)
	buff.Reset()
	binary.Write(buff, binary.BigEndian, uint32(0xd))
	binary.Write(buff, binary.BigEndian, msgRejectRequest)
	msgHead[msgRejectRequest] = append(msgHead[msgRejectRequest], buff.Bytes()...)
	buff.Reset()
	binary.Write(buff, binary.BigEndian, uint32(0xd))
	binary.Write(buff, binary.BigEndian, msgAllowedFast)
	msgHead[msgAllowedFast] = append(msgHead[msgAllowedFast], buff.Bytes()...)
}

//handshakeMsg is used for handshake
type handshakeMsg struct {
	Head     [protoLength]byte
	Reserved [reserved]byte
	InfoHash meta.Hash
	PeerID   [PeerIDLen]byte
}

//TODO handle the reserved field
func (p *peer) handshake(info meta.Hash) (err error) {
	logger.Infof("handshake with %v\n", p.addr)
	if p.conn == nil {
		p.conn, err = net.Dial("tcp", p.addr)
		if err != nil {
			return
		}
	}

	msg := handshakeMsg{
		handshakeHead,
		extension,
		info,
		p.id,
	}

	err = binary.Write(p.conn, binary.BigEndian, msg)
	if err != nil {
		return
	}
	return
}

func (p *peer) recvHandshake(info meta.Hash) error {
	resp := handshakeMsg{}
	err := binary.Read(p.conn, binary.BigEndian, &resp)
	if err != nil {
		return err
	}
	if !bytes.Equal(resp.Head[:], handshakeHead[:]) {
		return errProto
	}
	if !bytes.Equal(resp.InfoHash[:], info[:]) {
		return errInfoHash
	}
	if (resp.Reserved[7]&0x04 != 0) && (extension[7]&0x04 != 0) {
		p.fastExtension = true
	}
	copy(p.id[:], resp.PeerID[:])
	return nil
}

func (p *peer) keepAlive() {
	logger.Infof("send keep alive to %v\n", p.addr)
	p.sendMessage([]byte{byte(0)})
	return
}

func (p *peer) choke() {
	logger.Infof("send choke to %v\n", p.addr)
	p.sendMessage(msgHead[msgChoke])
	return
}

func (p *peer) unchoke() {
	logger.Infof("send unchoke to %v\n", p.addr)
	p.sendMessage(msgHead[msgUnchoke])
	return
}

func (p *peer) interest() {
	logger.Infof("send interest to %v\n", p.addr)
	p.sendMessage(msgHead[msgInterest])
	return
}

func (p *peer) noInterest() {
	logger.Infof("send no interest to %v\n", p.addr)
	p.sendMessage(msgHead[msgNoInterest])
	return
}

func (p *peer) have(index int) {
	logger.Infof("send have %v to %v\n", index, p.addr)
	p.sendMessage(packUint32(msgHead[msgHave], uint32(index)))
	return
}

func (p *peer) bitfield(b *bitmap.Bitmap) {
	body := b.Bytes()
	logger.Infof("send bitfield to %v\n", p.addr)
	p.sendMessage(message{msgIDLen + uint32(len(body)), msgBitfield, body})
	return
}

func (p *peer) request(r *request) {
	msg := packUint32(msgHead[msgRequest], uint32(r.index))
	msg = packUint32(msg, uint32(r.begin))
	msg = packUint32(msg, uint32(r.length))
	logger.Infof("send request %#v to %v\n", r, p.addr)
	p.sendMessage(msg)
	return
}

func (p *peer) piece(index, begin int, data []byte) {
	body := packUint32([]byte{}, uint32(index))
	body = packUint32(body, uint32(begin))
	body = append(body, data...)
	logger.Infof("send piece %v,%v to %v\n", index, begin, p.addr)
	p.sendMessage(message{msgIDLen + uint32(len(body)), msgCancel, body})
	return
}

func (p *peer) cancel(index, begin, length int) {
	msg := packUint32(msgHead[msgCancel], uint32(index))
	msg = packUint32(msg, uint32(begin))
	msg = packUint32(msg, uint32(length))
	logger.Infof("send cancel %v,%v to %v\n", index, begin, p.addr)
	p.sendMessage(msg)
}

func (p *peer) listenPort(port int) {
	msg := packUint32(msgHead[msgPort], uint32(port))
	logger.Infof("send port %v to %v\n", port, p.addr)
	p.sendMessage(msg)
}

func (p *peer) haveAll() {
	logger.Infof("send have all to %v\n", p.addr)
	p.sendMessage(msgHead[msgHaveALL])
}

func (p *peer) haveNone() {
	logger.Infof("send have none to %v\n", p.addr)
	p.sendMessage(msgHead[msgHaveNone])
}

func (p *peer) suggestPiece(index int) {
	logger.Infof("send suggest piece %v to %v\n", index, p.addr)
	p.sendMessage(packUint32(msgHead[msgSuggestPiece], uint32(index)))
}

func (p *peer) rejectRequest(r *request) {
	msg := packUint32(msgHead[msgRejectRequest], uint32(r.index))
	msg = packUint32(msg, uint32(r.begin))
	msg = packUint32(msg, uint32(r.length))
	logger.Infof("reject request piece %#v to %v\n", r, p.addr)
	p.sendMessage(msg)
}

func (p *peer) allowedFast(index int) {
	logger.Infof("send allowed fast %v to %v\n", index, p.addr)
	p.sendMessage(packUint32(msgHead[msgAllowedFast], uint32(index)))
}

func (p *peer) sendMessage(data interface{}) {
	go func() {
		var err error
		p.stateLock.RLock()
		if p.state != stateRunning {
			p.stateLock.RUnlock()
			return
		}
		switch v := data.(type) {
		case []byte:
			_, err = p.conn.Write(v)
		case message:
			msg := packUint32([]byte{}, v.Length)
			msg = append(msg, byte(v.ID))
			msg = append(msg, v.body...)
			_, err = p.conn.Write(msg)
		case io.WriterTo:
			_, err = v.WriteTo(p.conn)
		default:
			err = errType
		}
		p.stateLock.RUnlock()
		if err != nil {
			logger.Warningf("send message fail to %v with %v\n", err, p.addr)
			p.Close()
			return
		}
	}()
}

func (p *peer) readMessage() (msg message, err error) {
	length := uint32(0)
	err = binary.Read(p.conn, binary.BigEndian, &length)
	if err != nil {
		return
	}
	if length > msgMaxLength {
		logger.Warningf("message bad length:%v from %v\n", length, p.addr)
		return msg, errMsgLength
	}
	if length == 0 {
		msg = message{Length: 0}
		return
	}
	data := make([]byte, length)
	_, err = io.ReadFull(p.conn, data)
	if err != nil {
		logger.Warningf("recv message from %v with error:%v", p.addr, err)
		log.Printf("%v recv error:%v\n", p.addr, err)
		return msg, err
	}
	logger.Infof("recv message id:%v\n", msgID(data[0]))
	id, payload := msgID(data[0]), data[1:]
	msg = message{uint32(length), id, payload}
	return
}

func (p *peer) dispatch(m message) (msg messageWithID, err error) {
	if !p.fastExtension && msg.ID > msgPort {
		return msg, errFastExtension
	}
	var data interface{}
	switch m.ID {
	case msgChoke, msgUnchoke, msgInterest, msgNoInterest, msgHaveALL, msgHaveNone:
	case msgHave, msgSuggestPiece, msgAllowedFast:
		index, _, _ := unpackUint32(m.body, 0)
		data = int(index)
	case msgBitfield:
		data = bitmap.NewBitmap(m.body)
	case msgRequest, msgCancel, msgRejectRequest:
		data = newRequest(m.body)
	case msgPiece:
		data = newBlock(m.body)
	default:
		err = errMsgID
	}
	if err != nil {
		logger.Errorf("parse message %v error:%v from %v\n", m.ID, err, p.addr)
		return
	}
	msg = messageWithID{p.addr, m.ID, data}
	return
}

func packUint32(msg []byte, field uint32) []byte {
	return append(
		msg,
		byte(field>>24),
		byte(field>>16),
		byte(field>>8),
		byte(field),
	)
}

func newBlock(data []byte) *filesystem.Block {
	if len(data) < 12 {
		return nil
	}
	index, offset, _ := unpackUint32(data, 0)
	begin, offset, _ := unpackUint32(data, offset)
	return &filesystem.Block{int(index), int(begin), data[offset:]}
}

func unpackUint32(msg []byte, off int) (uint32, int, error) {
	if off+uint32Len > len(msg) {
		return 0, off, errBaseLen
	}
	v := uint32(msg[off])<<24 | uint32(msg[off+1])<<16 | uint32(msg[off+2])<<8 | uint32(msg[off+3])
	return v, off + uint32Len, nil
}

func unpackUint16(msg []byte, off int) (uint16, int, error) {
	if off+uint16Len > len(msg) {
		return 0, off, errBaseLen
	}
	v := uint16(uint32(msg[off])<<8 | uint32(msg[off+1]))
	return v, off + uint16Len, nil
}

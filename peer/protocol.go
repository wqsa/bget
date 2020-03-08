package peer

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"reflect"
	"time"

	pool "github.com/libp2p/go-buffer-pool"
	"github.com/pkg/errors"

	jww "github.com/spf13/jwalterweatherman"
)

var (
	errProto         = errors.New("handshake fail, error protocol")
	errInfoHash      = errors.New("handshake fail, infoHash not match")
	errHandshakeResp = errors.New("Peer handshake resp err")
	errMsgLength     = errors.New("message length is too long")
	errMsgID         = errors.New("unsupport message type")
	errFastExtension = errors.New("not support fast extension")

	extension [reserved]byte = [reserved]byte{0, 0, 0, 0, 0, 0, 0x04, 0}
)

type msgMarshaler interface {
	marshalBinary() ([]byte, error)
}

func (p *Peer) dial() error {
	var err error
	p.conn, err = net.Dial("tcp", p.addr)
	if err != nil {
		return errors.WithStack(err)
	}
	ctx := context.Background()
	if err := p.handshake(ctx); err != nil {
		return err
	}
	if err := p.recvHandshake(ctx); err != nil {
		return err
	}
	return nil
}

func (p *Peer) dialTimeout(timeout time.Duration) error {
	var err error
	deadline := time.Now().Add(timeout)
	p.conn, err = net.DialTimeout("tcp", p.addr, timeout)
	if err != nil {
		return errors.WithStack(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	if err := p.handshake(ctx); err != nil {
		return err
	}
	if err := p.recvHandshake(ctx); err != nil {
		return err
	}
	return nil
}

//TODO handle the reserved field
func (p *Peer) handshake(ctx context.Context) error {
	jww.TRACE.Printf("handshake with %v\n", p.addr)
	var err error

	if deadline, ok := ctx.Deadline(); ok {
		p.conn.SetDeadline(deadline)
	}
	err = binary.Write(p.conn, binary.BigEndian, handshakeMsg{
		Head:     handshakeHead,
		Reserved: extension,
		InfoHash: p.infoHash,
		PeerID:   p.id,
	})
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (p *Peer) recvHandshake(ctx context.Context) error {
	resp := handshakeMsg{}
	if deadline, ok := ctx.Deadline(); ok {
		p.conn.SetDeadline(deadline)
	}
	err := binary.Read(p.conn, binary.BigEndian, &resp)
	if err != nil {
		return err
	}
	if !bytes.Equal(resp.Head[:], handshakeHead[:]) {
		return errProto
	}
	if !bytes.Equal(resp.InfoHash[:], p.infoHash[:]) {
		return errInfoHash
	}
	if (resp.Reserved[7]&0x04 != 0) && (extension[7]&0x04 != 0) {
		p.fastExtension = true
	}
	copy(p.id[:], resp.PeerID[:])
	return nil
}

func (p *Peer) writeLoop() {
	for {
		select {
		case msg := <-p.writec:
			data, err := msg.marshalBinary()
			if err != nil {
				jww.FATAL.Panicf("marshal message failed, error:%v, message:%v\n", err, reflect.TypeOf(msg))
			}
			if _, err := p.conn.Write(data); err != nil {
				jww.TRACE.Printf("send message fail to %v with %v\n", err, p.addr)
				p.Close()
				pool.Put(data)
				return
			}
			pool.Put(data)
		case <-p.closing:
			return
		}
	}
}

func (p *Peer) readLoop() {
	for {
		length := uint32(0)
		h := new(messageHead)
		err := binary.Read(p.conn, binary.BigEndian, h)
		if err != nil {
			continue
		}
		if h.Length > msgMaxLength {
			jww.WARN.Printf("message bad length:%v from %v\n", length, p.addr)
			continue
		}
		if h.Length < 5 {
			continue
		}
		var msg message
		msg.body = pool.Get(int(msg.Length))
		_, err = io.ReadFull(p.conn, msg.body)
		if err != nil {
			jww.WARN.Printf("recv message from %v with error:%v", p.addr, err)
			continue
		}
		msg.addr = p.addr
		jww.INFO.Printf("recv message id:%v\n", msg.ID)
		select {
		case p.readC <- &msg:
		case <-p.closing:
			return
		}
	}
}
func (p *Peer) SendMessage(ctx context.Context, msg msgMarshaler) error {
	select {
	case p.writec <- msg:
		return nil
	case <-ctx.Done():
		return errors.New("cancel")
	}
}

func (p *Peer) ReadMessage(ctx context.Context) (*message, error) {
	select {
	case msg := <-p.readC:
		return msg, nil
	case <-ctx.Done():
		return nil, errors.New("cancel")
	}
}

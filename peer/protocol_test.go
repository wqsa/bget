package peer

import (
	"bytes"
	"io"
	"net"
	"testing"

	"github.com/wqsa/bget/meta"
)

var testID = [20]byte{1, 2, 3}

var handshakeTestCase = []struct {
	id    [20]byte
	hash  meta.Hash
	exten [reserved]byte
	err   error
}{
	{[20]byte{}, meta.Hash([20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9}), [reserved]byte{1, 2, 3}, nil},
	{testID, meta.Hash([20]byte{9, 5, 5, 4, 5, 6, 7, 8, 9}), [reserved]byte{3, 2, 3, 8}, nil},
	{[20]byte{1, 2, 3, 4}, meta.Hash([20]byte{9, 5, 5, 4, 5, 6, 7, 8, 9}), [reserved]byte{3, 2, 3, 8}, io.EOF},
}

func TestHandshake(t *testing.T) {
	serverAddr := "127.0.0.1:19960"
	server := NewServer(serverAddr, "0.0.1")
	server.id = testID
	ch := make(chan struct{})
	go func() {
		if err := server.Listen(); err != nil {
			t.Error("server listen fail", err)
		}
		ch <- struct{}{}
		for i, c := range handshakeTestCase {
			hash, conn, exten, err := server.Accept()
			if err != c.err {
				t.Errorf("%v: except:%v, but:%v\n", i, c.err, err)
			}
			if err != nil {
				continue
			}
			if !bytes.Equal(hash[:], c.hash[:]) {
				t.Errorf("%v: except:%v, but:%v\n", i, c.hash, hash)
			}
			if !bytes.Equal(exten[:], c.exten[:]) {
				t.Errorf("%v: except:%v, but:%v\n", i, c.exten, exten)
			}
			p := new(peer)
			p.conn = conn
			if err := p.handshake(c.hash); err != nil {
				t.Errorf("%v: send handshake fail:%v\n", i, err)
			}
		}
	}()
	<-ch
	for i, c := range handshakeTestCase {
		extension = c.exten
		p := new(peer)
		p.id = c.id
		p.addr = serverAddr
		if err := p.handshake(c.hash); err != nil {
			t.Errorf("%v: send handshake fail:%v\n", i, err)
		}
		if err := p.recvHandshake(c.hash); err != c.err {
			t.Errorf("%v: expect:%v, but:%v\n", i, err, c.err)
		}
	}
}

var noPayloadMsgCase = []struct {
	id  msgID
	err error
}{
	{msgInterest, nil},
	{msgChoke, nil},
	{msgNoInterest, nil},
	{msgUnchoke, nil},
	{msgHaveALL, nil},
	{msgHaveNone, nil},
}

func TestMessage(t *testing.T) {
	c1, c2 := net.Pipe()
	ch := make(chan struct{})
	go func() {
		p := new(peer)
		p.conn = c1
		//p.setState(stateRunning)
		for _, c := range noPayloadMsgCase {
			switch c.id {
			case msgInterest:
				p.interest()
			case msgNoInterest:
				p.noInterest()
			case msgChoke:
				p.choke()
			case msgUnchoke:
				p.unchoke()
			case msgHaveALL:
				p.haveAll()
			case msgHaveNone:
				p.haveNone()
			}
			ch <- struct{}{}
		}
	}()
	p := new(peer)
	p.conn = c2
	//.setState(stateRunning)
	for i, c := range noPayloadMsgCase {
		msg, err := p.readMessage()
		if err != c.err {
			t.Errorf("%v: expect:%v, but:%v\n", i, err, c.err)
		}
		if msg.ID != c.id {
			t.Errorf("%v: expect:%v, but:%v\n", i, msg.ID, c.id)
		}
		<-ch
	}
}

package dht

import (
	"bytes"
	"crypto/sha1"
	"math/rand"
	"net"

	"github.com/pkg/errors"

	"github.com/wqsa/bget/common/utils"
)

const (
	byteLen   = 8
	uint32Len = 4
	maxPort   = 655536

	nodeIDLen = sha1.Size
	nodeSize  = 26

	bucketK   = 8
	bucketNum = nodeIDLen * byteLen

	stateIdle = iota
	stateGood
	stateQuestionable
	stateBad
	stateUnknow
)

var (
	errNodeSize = errors.New("size of the data that will init node error")
)

type nodeID [nodeIDLen]byte

func newNodeID() nodeID {
	var id [nodeIDLen]byte
	for i := range id {
		id[i] = byte(rand.Intn(i << 7))
	}
	return id
}

func (id *nodeID) zero() bool {
	return bytes.Equal(id.slice(), bytes.Repeat([]byte{0}, nodeIDLen))
}

func (id *nodeID) equal(o *nodeID) bool {
	return bytes.Equal(id.slice(), o.slice())
}

func (id *nodeID) slice() []byte {
	b := [nodeIDLen]byte(*id)
	return b[:]
}

//MarshalBencode customize marshal nodeID in bencode
func (id *nodeID) MarshalBencode() ([]byte, error) {
	return append([]byte("20:"), id[:]...), nil
}

//UnmarshalBencode customize unmarshal nodeID in bencode
func (id *nodeID) UnmarshalBencode(data []byte) error {
	if len(data) != 20 {
		return errors.New("not nodeID")
	}
	copy(id[:], data[:])
	return nil
}

//node is a dht node
type node struct {
	id    nodeID
	addr  string
	cli   *krpcClient
	state int
}

//newNode preduce a dht node by IP and port
func newNode(id nodeID, addr string) (*node, error) {
	_, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	n := &node{
		id:    id,
		addr:  addr,
		state: stateIdle,
	}
	ch, err := dial(addr)
	if err != nil {
		return nil, err
	}
	n.cli = newKrpcClient(ch)
	return n, nil
}

func (n *node) checkStatus(me *node) {
	err := n.ping(&me.id)
	if err == nil {
		n.state = stateGood
	} else {
		switch n.state {
		case stateGood:
			n.state = stateQuestionable
		case stateQuestionable, stateUnknow:
			n.state = stateBad
		}
	}
}

func (n *node) distance(other *node) int {
	d := 0
	var id [nodeIDLen]byte
	for i := 0; i < nodeIDLen; i++ {
		id[i] = [nodeIDLen]byte(n.id)[i] ^ [nodeIDLen]byte(other.id)[i]
		for j := 0; j < 8; j++ {
			if (id[i] & (1 << uint(j))) != 0 {
				d++
			}
		}
	}
	return d
}

func uncompressNode(data []byte) (*node, error) {
	if len(data) == 0 || len(data) != 26 {
		return nil, nil
	}
	n := node{}
	copy(n.id[:], data[:nodeIDLen])
	addr, err := utils.ParseCompressIPV4Addr(data[nodeIDLen:])
	if err != nil {
		return nil, err
	}
	n.addr = addr
	return &n, err
}

func uncompressNodes(data []byte) ([]*node, error) {
	if len(data) == 0 || len(data)%26 != 0 {
		return nil, nil
	}
	nodes := make([]*node, len(data)/26)
	for i := 0; i < len(data); i += 26 {
		n, err := uncompressNode(data[i : i+26])
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (n *node) checkID(id *nodeID) error {
	if id == nil || id.zero() {
		return errors.New("id not match")
	}
	if n.id.zero() {
		n.id = *id
		return nil
	}
	if !n.id.equal(id) {
		return errors.New("id not match")
	}
	return nil
}

func (n *node) ping(ownID *nodeID) error {
	id, err := n.cli.ping(ownID)
	if err != nil {
		return err
	}
	return n.checkID(id)
}

func (n *node) findNode(ownID *nodeID, target *nodeID) ([]*node, error) {
	id, nodes, err := n.cli.findNode(ownID, target)
	if err != nil {
		return nil, err
	}
	err = n.checkID(id)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

func (n *node) getPeers(ownID *nodeID, infoHash [20]byte) ([]string, []*node, string, error) {
	return n.cli.getPeers(ownID, infoHash)
}

func (n *node) announcePeer(own *node, infoHash [20]byte, token string) (*nodeID, error) {
	return n.cli.announcePeer(own, infoHash, token)
}

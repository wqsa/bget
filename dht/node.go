package dht

import (
	"./bencode"
	"crypto/sha1"
	"errors"
	"math/rand"
	"net"
)

const (
	byteLen   = 8
	uint32Len = 4
	maxPort   = 655536

	nodeIDLen = sha1.Size
	nodeSize  = 26

	bucketK   = 8
	bucketNum = nodeIDLen * byteLen

	stateUnknow = iota
	stateGood
    stateQuestionable
	stateBad
)

var (
	errNodeSize = errors.NEW("size of the data that will init node error")
)

type nodeID [nodeIDLen]byte

func newNodeID() nodeID {
	var (
		id  nodeID
		pre [nodeIDLen / uint32Len]uint32
	)
	for i := range pre {
		pre[i] = rand.Uint32()
	}
	copy(id[:], pre[:])
}

//node is a dht node
type node struct {
	id    nodeID
	ip    net.IP
	port  int
	state int
}

//newNode preduce a dht node by IP and port
func newNode(ip string, port int) (n *node) {
	if port <= 0 || port > maxPort {
		return nil
	}
	n.ip = net.ParseIP(ip)
	if n.ip == nil {
		return nil
	}
	n.id = newNodeID()
	n.port = port
	n.state = stateNone
	return
}

func uncompressNode(data []byte) (node, error) {
	if len(data) != nodeSize {
		return node{}, errNodeSize
	}
    n := node{}
	copy(n.id[:], data[:20])
	copy(n.ip, data[20:24])
	//TODO n.port
	n.state = stateUnknow
	return n, nil
}

func (n *node) checkStatus(me) {
    if bytes.Equal(n.id, ping(me, n.id)) {
        n.state = n.stateGood
    } else {
        switch n.state {
        case stateGood:
            n.state = stateQuestionable
        case stateQuestionable, stateUnknow:
            n.state = b.statBad
        }
    }
}

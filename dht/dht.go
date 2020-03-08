package dht

import (
	"os"
	"time"

	"github.com/pkg/errors"

	"github.com/google/logger"
)

const (
	bootstrapTimeOut = 3 * time.Second
	minNodeNum       = 10
)

var (
	dhtLogger *logger.Logger

	defaultBootNode []*node
)

func init() {
	dhtLogger = logger.Init("DHT", false, false, os.Stdout)

	bootNodes := []string{
		"router.bittorrent.com:6881",
		"router.utorrent.com:6881",
		"router.bitcomet.com:6881",
	}

	defaultBootNode = make([]*node, 0, len(bootNodes))

	for _, n := range bootNodes {
		n, err := newNode(newNodeID(), n)
		if err == nil {
			defaultBootNode = append(defaultBootNode, n)
		}
	}
}

//DHT is Distributed Hash Table, see https://en.wikipedia.org/wiki/Distributed_hash_table
type DHT struct {
	own      *node
	table    *table
	bootNode []*node
	rawNodeC chan *node
	nodeCh   chan *node
	msgCh    chan []byte
	stopC    chan chan error
	reqC     chan peerRequest
}

//NewDHT DHT create a DHT instance
func NewDHT(m string, nodes []*node, bootNode []string) (*DHT, error) {
	var err error
	d := &DHT{
		rawNodeC: make(chan *node),
		nodeCh:   make(chan *node),
		msgCh:    make(chan []byte),
		stopC:    make(chan chan error),
		reqC:     make(chan peerRequest),
	}
	d.bootNode = defaultBootNode
	d.own, err = newNode(newNodeID(), m)
	if err != nil {
		return nil, err
	}
	d.table = newTable(d.own)
	for _, boot := range bootNode {
		for _, n := range defaultBootNode {
			if boot == n.addr {
				continue
			}
		}
		n, err := newNode(newNodeID(), boot)
		if err != nil {
			return nil, err
		}
		d.bootNode = append(d.bootNode, n)
	}
	if d.bootNode == nil {
		d.bootNode = defaultBootNode
	}
	for _, n := range nodes {
		d.addNode(n)
	}
	d.bootstrap()
	return d, nil
}

func (d *DHT) bootstrap() {
	if d.table.count >= minNodeNum {
		return
	}
	for _, n := range d.bootNode {
		ns, err := n.findNode(&d.own.id, &d.own.id)
		if err != nil {
			dhtLogger.Warningf("find node from %v, err: %v", n.addr, err)
			continue
		}
		dhtLogger.Infof("get %v node from %v", len(ns), n.addr)
		for _, n := range ns {
			d.addNode(n)
		}
	}
}

type peerRequest struct {
	infoHash [20]byte

	peers chan []string
}

//Run start a DHT instance
func (d *DHT) Run() {
	var err error
	for {
		select {
		case n := <-d.rawNodeC:
			go func() {
				err := n.ping(&d.own.id)
				if err != nil {
					dhtLogger.Warning("check node fail, err:", err)
					return
				}
				d.nodeCh <- n
			}()
		case n := <-d.nodeCh:
			if n != nil {
				d.table.addNode(n)
			}
		case req := <-d.reqC:
			go d.getPeers(req)
		case errC := <-d.stopC:
			errC <- err
		}
	}
}

func (d *DHT) getPeers(req peerRequest) {
	var f func(n *node) error
	//stop the recrusion?
	f = func(n *node) error {
		if n == nil {
			return errors.New("emptyt node")
		}
		peers, nodes, token, err := n.getPeers(&d.own.id, req.infoHash)
		if err != nil {
			return err
		}
		if len(peers) != 0 {
			req.peers <- peers
			n.announcePeer(d.own, req.infoHash, token)
		}
		if len(nodes) != 0 {
			// d.nodeCh <- nodes
			for _, n := range nodes {
				d.nodeCh <- n
				f(n)
			}
		}
		return nil
	}
	d.table.foreach(f)
}

func (d *DHT) addNode(n *node) error {
	go func() {
		err := n.ping(&d.own.id)
		dhtLogger.Infof("ping %v, result:%v", n.addr, err)
		if err == nil {
			d.rawNodeC <- n
		}
	}()
	return nil
}

//GetPeers query peers from DHT by infohash
func (d *DHT) GetPeers(infoHash [20]byte, peerC chan []string) {
	d.reqC <- peerRequest{infoHash, peerC}
}

//AddNode add node addr to DHT
func (d *DHT) AddNode(addrs []string) {
	nodes := []*node{}
	for _, addr := range addrs {
		n, err := newNode(newNodeID(), addr)
		if err != nil {
			return
		}
		nodes = append(nodes, n)

	}
	for _, n := range nodes {
		d.nodeCh <- n
	}
}

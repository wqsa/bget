package dht

import (
    "net"
    "sync"
    "time"
)

const (
    bootstrapTimeOut = 3 * time.Second
)

var (
    bootstrap = []Node{
        {ip:"router.bittorrent.com", port:6881},
        {ip:"router.utorrent.com", port:6881},
        {ip:"router.bitcomet.com", port:6881},
    }
)

type DHT struct {
    me Node
    table route
    bootNode []Node
}

func NewDHT(m Node, nodes []Node, bootNode []Node) *DHT {
    var wg sync.WaitGroup
    for _, n := range nodes {
        wg.Add(1)
        go func() {
            n.checkStatus()
            wg.Done()
        }()
    }
    wg.Wait()
    d := &DHT{me:me, bootNode:bootNode}
    for _, n := range nodes {
        if n.state == statGood {
            d.addNode(n)
        }
    }
    d.bootstrap()
    return d
}

func (d *DHT) bootstrap() {
    randID := newNodeId()
    var wg sync.WaitGroup
    nsCh := make(chan []Node)
    for _, n := range d.bootNode {
        n := n
        wg.Add(1)
        go func() {
            defer wg.Done()
            nodes, err := n.findNode(d.me.ID, randID)
            if err != nil {
                return
            }
            nsCh <- nodes:
        }()
    }
    go func() {
        wg.Wait()
        close(nsCh)
    }()
    nCh := make(chan Node)
    for ns := range nodes {
        for _, n := range n {
            n := n
            wg.Add(1)
            go func() {
                defer wg.Done()
                n.checkState()
                if(n.state != stateGood) {
                    return
                }
                nCh <- n
            }()
        }
    }
    go func() {
        wg.Wait()
        close(nCh)
    }()
    for n := range nCh {
        d.addNode(n)
    }
}

func (d* DHT) addNode(n *Node) bool {
    if n.stat != statGood {
        return false
    }
    d.table.set(d.me.distance(n), n)
    return true
}

func (d* DHT) GetPeers(ctx context.Context, infoHash meta.Hash, peersCh chan <- []peer.Peers) {
    nodes := d.table.getAll()
    nodeCh := make(chan []Node)
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()
    f := func(n Node){
        ps, ns, token, err := n.getPeers(me.ID, infoHash)
        if err != nil {
            d.table.remove(n)
        }
        if len(ps) != 0 {
            select {
            case peersCh <- ps:
                go n.announce(d.node, d.impilePort, token)
            case <-ctx.Done():
                return
            }
        }
        if len(ns) != 0 {
            select {
            case nodeCh <- ns:
            case <- ctx.Done():
                return
            }
        }
    }
    for _, n := range nodes {
        go f(n)
    }
    for {
        select {
        case ns := <-nodeCh:
            for _, n := range ns {
                go f(n)
                go func() {
                    if n.ping(me.ID) {
                        n.state = stateGood
                        d.addNode(n)
                    }
                }()
            }
        case <-cxt.Done():
            return
        }

    }
}

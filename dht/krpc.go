package dht

const (
    descQuery = "q"
    descResponse = "r"
    descError = "e"

    queryPing = "ping"
    queryFindNode = "find_node"
    queryGetPeers = "get_peers"
    queryAnnouncePeer = "announce_peer"
)

type (
    channel struct {
        ip net.IP
        port int
        conn net.Conn
    }

	head struct {
		TransID  uint16  `bencode:"t"`
		Describe string `bencode:"y"`
		Version  string `bencode:"v,omitpty"`
	}

	request struct {
        head
        Query string `bencode:"q"`
		Body interface{} `bencode:"a"`
	}

	response struct {
        head
		Boby interface{} `"bencode:"r"`
	}

	pingReq struct {
		ID nodeID `bencode:"id"`
	}
	pingResp struct {
		ID nodeID `bencode:"id"`
	}

	findNodeReq struct {
		ID     nodeID `bencode:"id"`
		Target nodeID `bencode:"target"`
	}
	findNodeResp struct {
		ID    nodeID         `bencode:"id"`
		Nodes []byte `bencode:"nodes"`
	}

	getPeersReq struct {
		ID       nodeID    `bencode:"id"`
		InfoHash meta.Hash `bencode:"info_hash"`
	}
	getPeersResp struct {
		ID     nodeID `bencode:"id"`
		Token  string `bencode:"token"`
		Values []byte `bencode:"values, omitpty"`
		Nodes  []byte `bencode:"nodes, omitpty"`
	}

	announcePeerReq struct {
		ID          nodeID    `bencode:"id"`
		InfoHash    meta.Hash `bencode:"info_hash"`
		ImpliedPort int       `bencode:"implied_port"`
		Port        int       `bencode:"port"`
		Token       string    `bencode:"token"`
	}
	announcePeerResp struct {
		ID nodeID `bencode:"id"`
	}
)

func newChannel(ip net.IP, port int) (channel, error) {
    c := channel{ip:ip, port:port}
    var err error
    c.conn, err = net.Dial("udp", ip.String()+":"+strconv.Itoa(port))
}

func (c *channel) request(req request, resp *response) error {

}

func newTranID() uint16 {
    return uint16(rand.Uint32())
}

func ping(me nodeID, recive Node, version string) (id nodeID, err error) {
    c, err := newChannel(recive.ip, recive,port)
    if err != nil {
        return
    }
    h := head{
        TransID: newTranID(),
        Describe:descQuery,
        Query:queryPing,
        Version:version,
    }
    req := request{h, pingReq{me}}
    resp.Body = pingResp{}
    err = c.request(req, &resp)
    if err != nil {
        return
    }
    b := resp.Body.(pingResp)
    return b.ID
}

func findNode(me nodeID, recive Node, target nodeID, version string) (id nodeID, nodes node, err error) {
    c, err := newChannel(recive.ip, recive.port)
    if err != nil {
        return
    }
    h := head{
        TransID: newTranID(),
        Describe:descQuery,
        Query:queryFindNode,
        Version:version,
    }
    req := request{h, findNodeReq{me, target}}
    resp.Body = findNodeResp{}
    err = c.request(req, &resp)
    if err != nil {
        return
    }
    r := resp.Body.(findNodeResp)
    id = r.ID
    for i := 0; i < len(r.Nodes); i+=nodeSize {
        n, err := uncompressNode(r.Nodes[i:i+nodeSize])
        if err != nil {
            return
        }
        nodes = append(nodes, n)
    }
    return
}

func getPeers(me nodeID, recive Node, infoHash meat.Hash, version string) (resp response, err error) {
    c, err := newChannel(recive.ip, recive.port)
    if err != nil {
        return
    }
    h := head{
        TransID: newTranID(),
        Describe:descQuery,
        Query:queryGetPeers,
        Version:version,
    }
    req := request{h, findNodeReq{me, infoHash}}
    resp.Body = getPeersResp{}
    err = c.request(req, &resp)
    if err != nil {
        return
    }
    return response
}

func announcePeer(me nodeID, recive Node, infoHash meta.Hash, ip net,IP, port int, impilePort bool) (id nodeID, err error) {
    c, err := newChannel(recive.ip, recive.port)
    if err != nil {
        return
    }
    h := head{
        TransID: newTranID(),
        Describe:descQuery,
        Query:queryAnnouncePeer,
        Version:version,
    }
    req := request{h, announcePeerReq{me, }}
    resp.Body = announcePeerResp{me, infoHash, impilePort, ip, port}
    err = c.request(req, &resp)
    if err != nil {
        return
    }
    r := response.Body.(announcePeerResp)
    id = r.ID
    return
}

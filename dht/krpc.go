package dht

import (
	"errors"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/wqsa/bget/bencode"
	"github.com/wqsa/bget/common/utils"
)

const (
	descQuery    = "q"
	descResponse = "r"
	descError    = "e"

	queryPing         = "ping"
	queryFindNode     = "find_node"
	queryGetPeers     = "get_peers"
	queryAnnouncePeer = "announce_peer"

	keepConnectTime = 10 * time.Second
)

type channel struct {
	addr  string
	conn  net.Conn
	timer *time.Timer
}

func dial(addr string) (*channel, error) {
	c := &channel{addr: addr}
	if err := c.doDial(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *channel) doDial() error {
	var err error
	c.conn, err = net.Dial("udp", c.addr)
	if err != nil {
		return err
	}
	c.timer = time.AfterFunc(keepConnectTime, func() {
		c.conn.Close()
	})
	return nil
}

func (c *channel) request(req request, resp interface{}) error {
	data, err := bencode.Marshal(req)
	if err != nil {
		return err
	}
	dhtLogger.Info("send msg:", string(data))
	_, err = c.conn.Write(data)
	if err != nil {
		return err
	}
	buff := getBuffer()
	defer putBuffer(buff)
	c.conn.SetDeadline(time.Now().Add(10 * time.Second))
	_, err = buff.ReadFrom(c.conn)
	dhtLogger.Info("recv msg:", buff.String())
	if err != nil {
		return err
	}

	if !c.timer.Stop() {
		c.doDial()
	} else {
		c.timer.Reset(keepConnectTime)
	}

	msg := new(response)
	err = bencode.Unmarshal(buff.Bytes(), msg)
	if err != nil {
		return err
	}

	if msg.TransID != req.head.TransID || msg.Describe != "r" {
		return errors.New("response error")
	}
	err = bencode.Unmarshal(msg.Body, resp)
	return err
}

type (
	head struct {
		TransID  string `bencode:"t"`
		Describe string `bencode:"y"`
		Version  string `bencode:"v,omitempty"`
	}

	kError struct {
		Code        int
		Description string
	}

	request struct {
		head
		Query string      `bencode:"q"`
		Body  interface{} `bencode:"a"`
		Error *kError     `bencode:"e,omitempty"`
	}

	response struct {
		head
		Body  bencode.RawMessage `bencode:"r"`
		Error *kError            `bencode:"e,omitempty"`
	}

	pingReq struct {
		ID *nodeID `bencode:"id"`
	}
	pingResp struct {
		ID *nodeID `bencode:"id"`
	}

	findNodeReq struct {
		ID     *nodeID `bencode:"id"`
		Target *nodeID `bencode:"target"`
	}
	findNodeResp struct {
		ID    *nodeID `bencode:"id"`
		Nodes []byte  `bencode:"nodes"`
	}

	getPeersReq struct {
		ID       *nodeID `bencode:"id"`
		InfoHash string  `bencode:"info_hash"`
	}
	getPeersResp struct {
		ID     *nodeID  `bencode:"id"`
		Token  string   `bencode:"token"`
		Values []string `bencode:"values, omitempty"`
		Nodes  []byte   `bencode:"nodes, omitempty"`
	}

	announcePeerReq struct {
		ID          *nodeID `bencode:"id"`
		InfoHash    string  `bencode:"info_hash"`
		ImpliedPort int     `bencode:"implied_port"`
		Port        int     `bencode:"port"`
		Token       string  `bencode:"token"`
	}
	announcePeerResp struct {
		ID *nodeID `bencode:"id"`
	}
)

func newTranID() string {
	b := []byte{byte(rand.Intn(128)), byte(rand.Intn(128))}
	return string(b)
}

//MarshalBencode customize marshal kError in bencode
func (e *kError) MarshalBencode() ([]byte, error) {
	m := make(map[string]interface{})
	m["e"] = []interface{}{
		e.Code,
		e.Description,
	}
	return bencode.Marshal(m)
}

//UnmarshalBencode customize unmarshal kError in bencode
func (e *kError) UnmarshalBencode(data []byte) error {
	l := []interface{}{}
	err := bencode.Unmarshal(data, &l)
	if err != nil {
		return err
	}
	if len(l) != 2 {
		return errors.New("format error")
	}
	var ok bool
	if e.Code, ok = l[0].(int); !ok {
		return errors.New("format error")
	}
	if e.Description, ok = l[1].(string); !ok {
		return errors.New("format error")
	}
	return nil
}

type krpcClient struct {
	ch      *channel
	version string
}

func newKrpcClient(ch *channel) *krpcClient {
	return &krpcClient{ch, ""}
}

func (c *krpcClient) ping(ownID *nodeID) (*nodeID, error) {
	h := head{
		TransID:  newTranID(),
		Describe: descQuery,
		Version:  c.version,
	}
	req := request{head: h, Query: queryPing, Body: pingReq{ownID}}
	resp := new(pingResp)
	err := c.ch.request(req, resp)
	if err != nil {
		return nil, err
	}
	return resp.ID, nil
}

func (c *krpcClient) findNode(ownID *nodeID, target *nodeID) (*nodeID, []*node, error) {
	h := head{
		TransID:  newTranID(),
		Describe: descQuery,
		Version:  c.version,
	}
	req := request{head: h, Query: queryFindNode, Body: findNodeReq{ownID, target}}
	resp := new(findNodeResp)
	err := c.ch.request(req, resp)
	if err != nil {
		return nil, nil, err
	}
	nodes, err := uncompressNodes(resp.Nodes)
	if err != nil {
		return nil, nil, err
	}
	return resp.ID, nodes, nil
}

func (c *krpcClient) getPeers(ownID *nodeID, infoHash [20]byte) ([]string, []*node, string, error) {
	h := head{
		TransID:  newTranID(),
		Describe: descQuery,
		Version:  c.version,
	}

	req := request{head: h, Query: queryGetPeers, Body: getPeersReq{ownID, string(infoHash[:])}}
	resp := new(getPeersResp)
	err := c.ch.request(req, resp)
	if err != nil {
		return nil, nil, "", err
	}
	if len(resp.Token) == 0 {
		return nil, nil, "", errNodeSize
	}

	if len(resp.Values) != 0 {
		addrs := make([]string, 0, len(resp.Values))
		for _, p := range resp.Values {
			a, err := utils.ParseIPV4Addr([]byte(p))
			if err != nil {
				return nil, nil, "", err
			}
			addrs = append(addrs, a)
		}
		return addrs, nil, resp.Token, err
	}
	if len(resp.Nodes) == 0 {
		return nil, nil, "", errNodeSize
	}
	nodes, err := uncompressNodes(resp.Nodes)
	return nil, nodes, resp.Token, err
}

func (c *krpcClient) announcePeer(own *node, infoHash [20]byte, token string) (*nodeID, error) {
	h := head{
		TransID:  newTranID(),
		Describe: descQuery,
		Version:  c.version,
	}
	_, port, err := net.SplitHostPort(own.addr.String())
	if err != nil {
		return nil, err
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}
	req := request{
		head:  h,
		Query: queryAnnouncePeer,
		Body:  announcePeerReq{&own.id, string(infoHash[:]), 1, p, token},
	}
	resp := new(announcePeerResp)
	err = c.ch.request(req, resp)
	if err != nil {
		return nil, err
	}
	return resp.ID, nil
}

// func ping(me nodeID, recive *node, version string) (id nodeID, err error) {
// 	c, err := newChannel(recive.addr.String())
// 	if err != nil {
// 		return
// 	}
// 	h := head{
// 		TransID:  newTranID(),
// 		Describe: descQuery,
// 		Version:  version,
// 	}
// 	req := request{h, queryPing, pingReq{&me}}
// 	resp := new(pingResp)
// 	err = c.request(req, &resp)
// 	if err != nil {
// 		return
// 	}
// 	return *resp.ID, nil
// }

// func findNode(me nodeID, recive *node, target nodeID, version string) (id *nodeID, nodes []*node, err error) {
// 	c, err := newChannel(recive.addr.String())
// 	if err != nil {
// 		return
// 	}
// 	h := head{
// 		TransID:  newTranID(),
// 		Describe: descQuery,
// 		Version:  version,
// 	}
// 	req := request{h, queryFindNode, findNodeReq{&me, &target}}
// 	resp := new(findNodeResp)
// 	err = c.request(req, &resp)
// 	if err != nil {
// 		return
// 	}
// 	id = resp.ID
// 	for i := 0; i < len(resp.Nodes); i += nodeSize {
// 		n, err := uncompressNode(resp.Nodes[i : i+nodeSize])
// 		if err != nil {
// 			return id, nodes, err
// 		}
// 		nodes = append(nodes, n)
// 	}
// 	return
// }

// func getPeers(me nodeID, recive *node, infoHash [20]byte, version string) ([]string, []*node, string, error) {
// 	if recive == nil || recive.addr == nil {
// 		fmt.Println(recive)
// 	}
// 	c, err := newChannel(recive.addr.String())
// 	if err != nil {
// 		return nil, nil, "", err
// 	}
// 	h := head{
// 		TransID:  newTranID(),
// 		Describe: descQuery,
// 		Version:  version,
// 	}

// 	req := request{h, queryGetPeers, getPeersReq{&me, string(infoHash[:])}}
// 	resp := new(getPeersResp)
// 	logger.Info("start get peers to", recive.addr.String())
// 	err = c.request(req, &resp)
// 	if err != nil {
// 		logger.Info("get peers fail, err:", err)
// 		return nil, nil, "", err
// 	}
// 	logger.Info("get peers succeed", len(resp.Values), len(resp.Nodes), len(resp.Token))
// 	if len(resp.Token) == 0 {
// 		return nil, nil, "", errNodeSize
// 	}

// 	if len(resp.Values) != 0 {
// 		addrs := make([]string, 0, len(resp.Values))
// 		for _, p := range resp.Values {
// 			a, err := utils.ParseIPV4Addr([]byte(p))
// 			if err != nil {
// 				return nil, nil, "", err
// 			}
// 			addrs = append(addrs, a)
// 		}
// 		return addrs, nil, resp.Token, err
// 	}
// 	if len(resp.Nodes) == 0 {
// 		return nil, nil, "", errNodeSize
// 	}
// 	nodes, err := uncompressNodes(resp.Nodes)
// 	return nil, nodes, resp.Token, err
// }

// func announcePeer(me *node, recive *node, infoHash [20]byte, token string) (id nodeID, err error) {
// 	c, err := newChannel(recive.addr.String())
// 	if err != nil {
// 		return
// 	}
// 	h := head{
// 		TransID:  newTranID(),
// 		Describe: descQuery,
// 	}
// 	_, port, err := net.SplitHostPort(me.addr.String())
// 	if err != nil {
// 		return
// 	}
// 	p, err := strconv.Atoi(port)
// 	if err != nil {
// 		return
// 	}
// 	req := request{h, queryAnnouncePeer, announcePeerReq{&me.id, string(infoHash[:]), 1, p, token}}
// 	resp := new(announcePeerResp)
// 	err = c.request(req, &resp)
// 	if err != nil {
// 		return
// 	}
// 	id = *resp.ID
// 	return
// }

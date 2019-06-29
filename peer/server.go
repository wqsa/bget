package peer

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"

	"github.com/wqsa/bget/meta"
)

const (
	serverIP   = ""
	serverPort = 6881
)

var (
	idStyle  = styAzureus
	idPrefix = "DG"
	version  = "1.0.0"
)

//Server is a local server for other peer
type Server struct {
	id      [PeerIDLen]byte
	addr    string
	fd      net.Listener
	version string
}

//NewServer create a peer server
func NewServer(ip string, port int, version string) *Server {
	id := getID(idStyle, idPrefix, version)
	return &Server{id: id, addr: net.JoinHostPort(ip, strconv.Itoa(port)), version: version}
}

//Listen start a listen
func (s *Server) Listen() (err error) {
	s.fd, err = net.Listen("tcp", s.addr)
	return err
}

//Accept accept a vaild handshake message
func (s *Server) Accept() (info meta.Hash, conn net.Conn, extension [reserved]byte, err error) {
	for {
		conn, err = s.fd.Accept()
		if err != nil {
			return
		}
		resp := handshakeMsg{}
		err = binary.Read(conn, binary.BigEndian, &resp)
		if err == nil && bytes.Equal(resp.Head[:], handshakeHead[:]) && s.checkID(resp.PeerID) {
			info = resp.InfoHash
			extension = resp.Reserved
			return
		}
		conn.Close()
	}
}

//Close close the Server
func (s *Server) Close() error {
	return s.fd.Close()
}

//ID return the server id
func (s *Server) ID() [PeerIDLen]byte {
	return s.id
}

//Addr return ther server addr
func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) checkID(id [PeerIDLen]byte) bool {
	vaild := true
	for _, v := range id {
		if v != 0 {
			vaild = false
		}
	}
	if vaild {
		return vaild
	}
	return bytes.Equal(s.id[:], id[:])
}

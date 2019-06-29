package dht

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/logger"
)

func TestBootNode(t *testing.T) {
	bootNodes := []string{
		"router.bittorrent.com:6881",
		"router.utorrent.com:6881",
		"router.bitcomet.com:6881",
		"dht.transmissionbt.com:6881",
	}
	var wg sync.WaitGroup
	for _, n := range bootNodes {
		wg.Add(1)
		n := n
		go func() {
			defer wg.Done()
			conn, err := net.Dial("udp", n)
			if err != nil {
				t.Error("dial boot node fail,", n, err)
				return
			}
			conn.SetReadDeadline(time.Now().Add(20 * time.Second))
			if _, err = conn.Write([]byte("d1:ad2:id20:abcdefghij0123456789e1:q4:ping1:t2:aa1:y1:qe")); err != nil {
				t.Error("send to boot node fail,", n, err)
			}
			buf := make([]byte, 1024)
			if _, err = conn.Read(buf); err != nil || len(buf) == 0 {
				t.Error("recv from boot node fail", n, err, len(buf))
			}
		}()
	}
	wg.Wait()
}
func TestDHT(t *testing.T) {
	logger.Init("krpc", true, false, os.Stdout)
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:8006")
	if err != nil {
		t.Fatal(err)
	}
	m, err := newNode(nodeID([20]byte{1, 3, 4}), addr)
	if err != nil {
		t.Fatal(err)
	}
	d := NewDHT(m, nil, nil)
	go d.Run()
	infoHash, err := hex.DecodeString("c16796a74dc24cc7c6df2f7b66b861e22dec69b1")
	if err != nil {
		t.Fatal(err)
	}
	var h [20]byte
	copy(h[:], infoHash)
	pc := make(chan []string)
	d.GetPeers(h, pc)
	go func() {
		for ps := range pc {
			fmt.Println(ps)
		}
	}()
	time.Sleep(27 * time.Second)
}

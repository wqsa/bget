package tracker

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"
)

func TestHTTPTracker(t *testing.T) {
	u := HTTPTracker{`http://torrent.ubuntu.com:6969/announce`, 0, nil, time.Now(), true}
	ctx := context.Background()
	infoHash := [20]byte{}
	hex.Decode(infoHash[:], []byte("56061fb42e24c9ded06173888ba0b46c06be6088"))
	ctx = context.WithValue(ctx, StateKey("info hash"), infoHash)
	ctx = context.WithValue(ctx, StateKey("peer id"), infoHash)
	ctx = context.WithValue(ctx, StateKey("download"), uint64(0))
	ctx = context.WithValue(ctx, StateKey("left"), uint64(1502208))
	ctx = context.WithValue(ctx, StateKey("upload"), uint64(0))
	ctx = context.WithValue(ctx, StateKey("event"), httpEventStart)
	ctx = context.WithValue(ctx, StateKey("want"), 50)
	ctx = context.WithValue(ctx, StateKey("ip"), "")
	ctx = context.WithValue(ctx, StateKey("port"), 1080)
	peers, err := u.Announce(ctx)
	if err != nil {
		t.Error(err)
	}
	wg := sync.WaitGroup{}
	for _, p := range peers {
		wg.Add(1)
		peer := p
		go func() {
			err = peer.Handshake(infoHash)
			if err != nil {
				t.Error(err)
			} else {
				peer.Interest()
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func TestUDPTracker(t *testing.T) {
	url, _ := url.Parse(`udp://tracker.internetwarriors.net:1337`)
	u := UDPTracker{*url, 0, nil, 0, time.Now()}
	ctx := context.Background()
	infoHash := [20]byte{}
	hex.Decode(infoHash[:], []byte("A753EA13F243EF9C4006D103DCBDBC7CABAD8A01"))
	ctx = context.WithValue(ctx, StateKey("info hash"), infoHash)
	ctx = context.WithValue(ctx, StateKey("peer id"), infoHash)
	ctx = context.WithValue(ctx, StateKey("download"), uint64(0))
	ctx = context.WithValue(ctx, StateKey("left"), uint64(1502208))
	ctx = context.WithValue(ctx, StateKey("upload"), uint64(0))
	ctx = context.WithValue(ctx, StateKey("event"), udpEventStart)
	ctx = context.WithValue(ctx, StateKey("want"), -1)
	ctx = context.WithValue(ctx, StateKey("ip"), "")
	ctx = context.WithValue(ctx, StateKey("port"), 1080)
	peers, err := u.Announce(ctx)
	if err != nil {
		t.Error(err)
	}
	fmt.Println(peers)
}

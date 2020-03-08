package tracker

import (
	"encoding/hex"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wqsa/bget/meta"
)

const (
	httpTrackerURL = `http://torrent.ubuntu.com:6969/announce`
	udpTrackerURL  = `udp://tracker.internetwarriors.net:1337`
)

func TestHTTPTracker(t *testing.T) {
	u, err := url.Parse(httpTrackerURL)
	assert.Nilf(t, err, "%+v", err)
	tracker := newhttpTracker(u)
	infoHash := [20]byte{}
	_, err = hex.Decode(infoHash[:], []byte("28f0bb34b542af1609641fcf4188afed73a03f64"))
	assert.Nil(t, err)
	_, err = tracker.announce(meta.Hash(infoHash), make([]byte, 20), "127.0.0.1:6882", EventStart, &AnnounceStats{})
	assert.Nilf(t, err, "%+v", err)
}

func TestUDPTracker(t *testing.T) {
	u, err := url.Parse(udpTrackerURL)
	assert.Nilf(t, err, "%+v", err)
	tracker := newudpTracker(u)
	infoHash := [20]byte{}
	_, err = hex.Decode(infoHash[:], []byte("F47279403014580AFB851A344D91BE39015CEC96"))
	assert.Nil(t, err)
	_, err = tracker.announce(meta.Hash(infoHash), make([]byte, 20), "127.0.0.1:6882", EventStart, &AnnounceStats{})
	assert.Nilf(t, err, "%+v", err)
}

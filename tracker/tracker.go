package tracker

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/wqsa/bget/meta"
)

//announce event
const (
	EventNone     = 0
	EventComplete = 1
	EventStart    = 2
	EventStop     = 3
)

var (
	errWaitTime = errors.New("should wait")
)

//DownloadStatus is the download status to announce tracker
type DownloadStatus struct {
	Download int64
	Upload   int64
	Left     int64
}

//Tracker is a tracker interface
type Tracker interface {
	Announce(hash meta.Hash, id []byte, ip net.IP, port int, event int, status *DownloadStatus) ([]string, error)
	getWaitTime() time.Time
	close() error
}

//NewTracker return a Tracker instance
func NewTracker(t string) Tracker {
	u, err := url.Parse(t)
	if err != nil {
		return nil
	}
	switch u.Scheme {
	case "udp":
		return NewUDPTracker(t)
	case "http", "https":
		return NewHTTPTracker(t)
	}
	return nil
}

func convAddrs(data []byte) []string {
	if len(data) < 6 || len(data)%6 != 0 {
		return nil
	}
	addrs := make([]string, len(data)/6)
	for i := 0; i+6 < len(data); i += 6 {
		p := (uint16(data[i+4]) << 8) | uint16(data[i+5])
		addrs[i/6] = net.JoinHostPort(net.IP(data[i:i+4]).String(), strconv.Itoa(int(p)))
	}
	return addrs
}

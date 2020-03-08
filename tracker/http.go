package tracker

import (
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/wqsa/bget/bencode"
	"github.com/wqsa/bget/common"
	"github.com/wqsa/bget/meta"
)

//httpTracker is a tracker use HTTP
type httpTracker struct {
	serverURL *url.URL
	trackerID int
	nextTime  time.Time
	compress  bool
}

type httpTrackerResponse struct {
	FailReason  string             `bencode:"failure reason,omitpty"`
	Warning     string             `bencode:"warning message,omitpty"`
	WaitTime    time.Duration      `bencode:"interval,omitpty"`
	MinInterval time.Duration      `bencode:"min interval"`
	TrackerID   int                `bencode:"tracker id,omitpty"`
	complete    int                `bencode:"complete,omitpty"`
	incomplete  int                `bencode:"incomplete,omitpty"`
	downloaded  int                `bencode:"downloaded,omitpty"`
	Peers       bencode.RawMessage `bencode:"peers,omitpty"`
}

//NewhttpTracker create a http tracker by url
func newhttpTracker(u *url.URL) *httpTracker {
	return &httpTracker{serverURL: u, compress: true}
}

//Announce is used for announce to http tracker
func (t *httpTracker) announce(hash meta.Hash, id []byte, addr string, event Event, status *AnnounceStats) ([]string, error) {
	resp, err := t.request(hash, id, addr, event, status)
	if err != nil {
		return nil, err
	}
	addrs, err := common.ParseCompressIPV4Addrs([]byte(resp.Peers))
	if err != nil {
		return nil, err
	}
	return addrs, nil
}

func (t *httpTracker) waitTime() time.Time {
	return t.nextTime
}

func (t *httpTracker) request(hash meta.Hash, id []byte, addr string, event Event, status *AnnounceStats) (*httpTrackerResponse, error) {
	if event == EventNone {
		return nil, errors.New("time is not up")
	}
	query := t.serverURL.Query()
	query.Set("peer_id", string(id))
	ip, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	query.Set("ip", ip)
	query.Set("port", port)
	query.Set("info_hash", string(hash[:]))
	if t.compress {
		query.Set("compact", "1")
	} else {
		query.Set("compact", "0")
	}
	query.Set("downloaded", strconv.FormatUint(uint64(status.Download), 10))
	query.Set("left", strconv.FormatUint(uint64(status.Left), 10))
	query.Set("uploaded", strconv.FormatUint(uint64(status.Upload), 10))
	query.Set("event", event.String())
	t.serverURL.RawQuery = query.Encode()
	httpResp, err := http.Get(t.serverURL.String())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer httpResp.Body.Close()
	body, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	resp := new(httpTrackerResponse)
	err = bencode.Unmarshal(body, resp)
	return resp, err
}

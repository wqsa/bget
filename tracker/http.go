package tracker

import (
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"../bencode"
	"../meta"
)

//HTTPTracker is a tracker use HTTP
type HTTPTracker struct {
	url       string
	trackerID int
	nextTime  time.Time
	compress  bool
}

type httpTrackerReq url.Values

func (req *httpTrackerReq) set(key, value string) {
	(*url.Values)(req).Set(key, value)
}

type httpTrackerResp struct {
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

//NewHTTPTracker create a http tracker by url
func NewHTTPTracker(url string) *HTTPTracker {
	return &HTTPTracker{url: url, compress: true}
}

func (t *HTTPTracker) getWaitTime() time.Time {
	return t.nextTime
}

//Announce is used for announce to http tracker
func (t *HTTPTracker) Announce(hash meta.Hash, id []byte, ip net.IP, port int, event int, status *DownloadStatus) ([]string, error) {
	if event == EventNone {
		time.Sleep(t.nextTime.Sub(time.Now()))
	}
	req := httpTrackerReq{}
	req.set("peer_id", string(id))
	req.set("ip", ip.String())
	req.set("port", strconv.FormatUint(uint64(port), 10))
	req.set("info_hash", string(hash[:]))
	if t.compress {
		req.set("compact", "1")
	} else {
		req.set("compact", "0")
	}
	req.set("downloaded", strconv.FormatUint(uint64(status.Download), 10))
	req.set("left", strconv.FormatUint(uint64(status.Left), 10))
	req.set("uploaded", strconv.FormatUint(uint64(status.Upload), 10))
	switch event {
	case EventStart:
		req.set("event", "started")
	case EventComplete:
		req.set("event", "completed")
	case EventStop:
		req.set("event", "stopped ")
	}
	reqURL := t.url + "?" + (url.Values)(req).Encode()
	//fmt.Println(reqURL)
	httpResp, err := http.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	body, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	resp := httpTrackerResp{}
	err = bencode.Unmarshal(body, &resp)
	if err != nil {
		return nil, err
	}
	if resp.FailReason != "" {
		return nil, errors.New("announce fail " + resp.FailReason)
	}
	t.nextTime = time.Now().Add(time.Duration(resp.WaitTime))
	peers := convAddrs([]byte(resp.Peers))
	if peers == nil {
		return nil, errHTTPResp
	}
	return peers, nil
}

func (t *HTTPTracker) close() error {
	return nil
}

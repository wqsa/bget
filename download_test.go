package bget

import (
	"os"
	"testing"
	"time"

	"github.com/google/logger"
)

func init() {
	logger.Init("Debug", false, false, os.Stdout)
}

func TestDownload(t *testing.T) {
	cfg := NewConfig()
	d, err := New(cfg)
	if err != nil {
		t.Error(err)
		return
	}
	id, err := d.Add("./test/ubuntu-19.04-desktop-amd64.iso.torrent", "./")
	if err != nil {
		t.Error(err)
		return
	}
	d.Start(id)
	time.Sleep(time.Second * 10)
	d.Close()
}

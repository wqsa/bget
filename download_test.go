package bittorrent

import (
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/google/logger"
)

func init() {
	logger.Init("Debug", false, false, os.Stdout)
}

func TestDownload(t *testing.T) {
	home := homeDir()
	if home == "" {
		defaultDir = "./Downloads"
	}
	defaultDir = home + "Downloads"

	validVersion = regexp.MustCompile(versionExpr)

	d, err := New()
	if err != nil {
		t.Error(err)
		return
	}
	id, err := d.Add("E:/下载/kubuntu-18.10-desktop-amd64.iso.torrent", "./")
	if err != nil {
		t.Error(err)
		return
	}
	d.Start(id)
	time.Sleep(time.Second * 10)
	d.Close()
}

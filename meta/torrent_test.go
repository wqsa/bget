package meta

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"../bencode"
)

func TestTorrent(t *testing.T) {
	torrent := new(Torrent)
	file, err := os.Open(`test.torrent`)

	if err != nil {
		t.Error("can't open the file", err)
		return
	}

	data, err := ioutil.ReadAll(file)

	if err != nil {
		t.Error("can't read the file", err)
		return
	}

	err = bencode.Unmarshal(data, torrent)

	if err != nil {
		t.Error("prase error", err)
	}
	//fmt.Println(torrent.Announce)
	//fmt.Println(torrent.AnnounceList)
	fmt.Println(torrent.Info.Hash)
}

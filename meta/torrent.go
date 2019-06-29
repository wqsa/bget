package meta

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"

	"github.com/wqsa/bget/bencode"
)

var (
	errFile     = errors.New("no this file")
	errIPFormet = errors.New("invild ip")
)

//An UnmarshalError describes a fail unmarshal
type UnmarshalError struct {
	Struct string
	Field  string
}

func (e *UnmarshalError) Error() string {
	return "fail to unmarshal " + e.Struct + "." + e.Field
}

type Node struct {
	IP   net.IP
	Port int
}

func (n *Node) UnmarshalBencode(data []byte) error {
	l := make([]interface{}, 10)
	err := bencode.Unmarshal(data, &l)
	if err != nil || len(l) != 2 {
		return err
	}
	ip, ok := l[0].(string)
	if !ok {
		return &UnmarshalError{"ip", "string"}
	}
	n.IP = net.ParseIP(ip)
	if n.IP == nil {
		return errIPFormet
	}
	n.Port, ok = l[1].(int)
	if !ok {
		return &UnmarshalError{"ip", "string"}
	}
	return nil
}

type ed2k [16]byte

func (e *ed2k) UnmarshalBencode(data []byte) error {
	if len(data) != 16 {
		return &UnmarshalError{"FileInfo", "ed2k"}
	}
	copy(e[:], data[:])
	return nil
}

type fileHash [sha1.Size]byte

func (h *fileHash) UnmarshalBencode(data []byte) error {
	if len(data) != sha1.Size {
		return &UnmarshalError{"FileInfo", "fileHash"}
	}
	copy(h[:], data[:])
	return nil
}

type FileInfo struct {
	Path     []string `bencode:"path"`
	PathUTF8 []string `bencode:"path.utf-8,omitempty"`
	Length   int64    `bencode:"length"`
	Ed2k     ed2k     `bencode:"ed2k,omitempty"`
	Filehash fileHash `bencode:"filehash,omitempty"`
	Download bool     `bencode:"-"`
}

func (f *FileInfo) String() string {
	var p, load string
	if len(f.PathUTF8) != 0 {
		p = strings.Join(f.PathUTF8, "/")
	} else {
		p = strings.Join(f.Path, "/")
	}
	if f.Download {
		load = "Yes"
	} else {
		load = "No"
	}
	return p + "\t" + load
}

//Hash is sha1 hash for Info and piece
type Hash [sha1.Size]byte

func (h Hash) String() string {
	return fmt.Sprintf("%x", h[:])
}

type Info struct {
	Hash             Hash       `bencode:"-"`
	Files            []FileInfo `bencode:"files,omitempty"`
	PieceLength      int        `bencode:"piece length"`
	Pieces           []byte     `bencode:"pieces"`
	Name             string     `bencode:"name,omitempty"`
	Length           int64      `bencode:"length,omitempty"`
	Private          int        `bencode:"private,omitempty"`
	Publisher        string     `bencode:"publisher"`
	PublisherUTF8    string     `bencode:"publisher.utf-8"`
	PublisherURL     string     `bencode:"publisher-url"`
	PublisherURLUTF8 string     `bencode:"publisher-url.utf-8"`
}

//avoid recurice invoke UnmarshalBencode
type noUnmarshalInfo Info

func (i *Info) UnmarshalBencode(data []byte) error {
	i.Hash = sha1.Sum(data)
	return bencode.Unmarshal(data, (*noUnmarshalInfo)(i))
}

//FileList return a list of file will download
func (i *Info) FileList() []File {
	var (
		begin  int64
		offset int
	)
	files := make([]File, len(i.Files))
	for j, f := range i.Files {
		files[j] = File{f.Path, f.Length, begin, offset}
		offset = int(f.Length % int64(i.PieceLength))
		begin += f.Length / int64(i.PieceLength)
		if offset != 0 {
			begin++
		}
	}
	return files
}

//Torrent load a btorrent metofile from file to memory
type Torrent struct {
	Announce     string     `bencode:"announce,omitempty"`
	AnnounceList [][]string `bencode:"announce-list,omitempty"`
	Comment      string     `bencode:"comment,omitempty"`
	CommentUTF8  string     `bencode:"comment.utf-8,omitempty"`
	Created      string     `bencode:"created by,omitempty"`
	Date         uint       `bencode:"creation date,omitempty"`
	Encoding     string     `bencode:"encoding,omitempty"`
	Info         Info
	Nodes        []Node `bencode:"node,omitempty"`
}

//NewTorrent create a torrent
func NewTorrent(path string) (*Torrent, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(fd)
	if err != nil {
		return nil, err
	}
	t := new(Torrent)
	err = bencode.Unmarshal(data, t)
	if err != nil {
		return nil, err
	}

	//if single file
	if len(t.Info.Files) == 0 {
		t.Info.Files = []FileInfo{FileInfo{Path: []string{t.Info.Name}, Length: t.Info.Length}}
	}

	//default download all file
	for i := range t.Info.Files {
		t.Info.Files[i].Download = true
	}
	return t, nil
}

func (t *Torrent) SetFile(index int, download bool) error {
	if index < 0 || index >= len(t.Info.Files) {
		return errFile
	}
	t.Info.Files[index].Download = download
	return nil
}
func (t *Torrent) SetDownload(index int) error {
	return t.SetFile(index, true)
}

func (t *Torrent) SetIgnore(index int) error {
	return t.SetFile(index, false)
}

type torrentKey string

const (
	keyInfoHash    = torrentKey("info hash")
	keyPieces      = torrentKey("pieces")
	keyPieceLength = torrentKey("piece length")
	keyPieceNum    = torrentKey("piece number")
	keyFiles       = torrentKey("files")
	keyNodes       = torrentKey("nodes")
	keyTorrent     = torrentKey("announce list")
)

func (t *Torrent) ToContext(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, keyInfoHash, t.Info.Hash)
	ctx = context.WithValue(ctx, keyPieces, t.Info.Pieces)
	ctx = context.WithValue(ctx, keyPieceLength, int(t.Info.PieceLength))
	ctx = context.WithValue(ctx, keyFiles, t.Info.Files)
	ctx = context.WithValue(ctx, keyPieceNum, len(t.Info.Pieces)/sha1.Size)
	ctx = context.WithValue(ctx, keyNodes, t.Nodes)
	ctx = context.WithValue(ctx, keyTorrent, t.getAnnounce())
	return ctx
}

func FromContext(ctx context.Context, key string) interface{} {
	k := torrentKey(key)
	value := ctx.Value(k)
	return value
}

//GetAnnounce return all tracker
func (t *Torrent) getAnnounce() (trackers []string) {
	if t.Announce != "" {
		trackers = append(trackers, t.Announce)
	}
	for _, v := range t.AnnounceList {
		for _, a := range v {
			trackers = append(trackers, a)
		}
	}
	return
}

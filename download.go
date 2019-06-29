package bittorrent

import (
	"context"
	"errors"
	"net"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unicode"

	"github.com/google/logger"

	"github.com/wqsa/bget/meta"
	"github.com/wqsa/bget/peer"
)

const (
	maxTask           = 5
	maxMemory         = 256 * 1024 * 1024
	defaultVersion    = "0.1.0"
	defaultIP         = "127.0.0.1"
	defaultPort       = 6228
	defaultIdentifier = "GD"

	sendDHTTimeout = time.Second

	//see https://semver.org
	versionExpr = `([0-9]\.){2}[0-9](\-{1}([0-9A-Za-z]+\.)*[[0-9A-Za-z]+)?(\+{1}([0-9A-Za-z]+\.)*[[0-9A-Za-z]+)?`
)

var (
	defaultDir   string
	validVersion *regexp.Regexp
)

//Errors
var (
	ErrIdentFormat   = errors.New("Ident format error")
	ErrVersionFormat = errors.New("version format error")

	ErrCreateTaskRepeat = errors.New("repeat task, task has been created")
	ErrTaskNotFound     = errors.New("can't find this task")
)

func init() {
	home := homeDir()
	if home == "" {
		defaultDir = "./Downloads"
	} else {
		defaultDir = home + "Downloads"
	}

	validVersion = regexp.MustCompile(versionExpr)
}

func homeDir() string {
	switch runtime.GOOS {
	case "darwin":
		return os.Getenv("HOMEPATH") + "\\"
	case "linux":
		return os.Getenv("HOME") + "/"
	}
	return ""
}

//Download is top-level instance
type Download struct {
	tasks      map[string]*task
	server     *peer.Server
	defaultDir string
	memory     int64
	memc       chan int64
	ctx        context.Context
	cancel     context.CancelFunc
}

//New creates an instance of Download by default config.
func New() (d *Download, err error) {
	return NewDownload(defaultIP, defaultIP, defaultDir, defaultIdentifier,
		defaultVersion, defaultPort, defaultPort)
}

//NewDownload creates an instance of Download by detailed parameters
func NewDownload(peerIP, dhtIP, dir, brand, version string, peerPort, dhtPort int) (d *Download, err error) {
	if len(brand) > 2 || !isStringPrint(brand) {
		return nil, ErrIdentFormat
	}
	if validVersion.FindString(version) != version {
		return nil, ErrVersionFormat
	}
	d = new(Download)
	d.tasks = make(map[string]*task)
	err = d.setDefaultDir(dir)
	if err != nil {
		return
	}
	d.ctx = context.Background()
	d.ctx, d.cancel = context.WithCancel(d.ctx)
	//dht.Run(ctx, dhtIP, dhtPort, version)
	d.server = peer.NewServer(peerIP, peerPort, version)
	if d.server == nil {
		panic("ss")
	}
	d.memc = make(chan int64, 10)
	go func() {
		for {
			select {
			case m := <-d.memc:
				d.memory += m
				logger.Infoln("use memory:", d.memory)
				if d.memory > maxMemory {
					var wg sync.WaitGroup
					for _, t := range d.tasks {
						wg.Add(1)
						go func() {
							t.save()
							wg.Done()
						}()
					}
					wg.Wait()
				}
			case <-d.ctx.Done():
				return
			}
		}
		return
	}()
	go d.runServer(d.ctx)
	return d, nil
}

func (d *Download) runServer(ctx context.Context) {
	err := d.server.Listen()
	if err != nil {
		panic(err)
	}
	for {
		hash, conn, reserved, err := d.server.Accept()
		if err != nil {
			panic(err)
		}
		if t, ok := d.tasks[hash.String()]; ok {
			logger.Infoln("accept peer", conn.RemoteAddr().String())
			t.manager.AddPeer(ctx, conn, reserved)
		} else {
			conn.Close()
		}
	}
}

//Add add a new task to the instance
func (d *Download) Add(task, path string) (id string, err error) {
	torrent, err := meta.NewTorrent(task)

	if torrent == nil {
		return "", err
	}
	id = torrent.Info.Hash.String()
	if _, ok := d.tasks[id]; ok {
		return id, ErrCreateTaskRepeat
	}
	addr := d.server.Addr()
	ip, port, err := net.SplitHostPort(addr)
	if err != nil {
		logger.Fatalln(err)
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		logger.Fatalln(err)
	}
	d.tasks[id] = newTask(torrent, path, d.server.ID(), ip, p, d.memc)
	return
}

//GetFileList get all files of the task
func (d *Download) GetFileList(task string) ([]meta.FileInfo, error) {
	if _, ok := d.tasks[task]; !ok {
		return nil, ErrTaskNotFound
	}
	return d.tasks[task].torrent.Info.Files, nil
}

//SetFile set all files of the task download or not by option
func (d *Download) SetFile(task string, option map[int]bool) {
	if _, ok := d.tasks[task]; !ok {
		return
	}
	for index, load := range option {
		d.tasks[task].torrent.SetFile(index, load)
	}
}

//Start start a task, it will block until task finish or occur errors
func (d *Download) Start(task string) {
	t, ok := d.tasks[task]
	if !ok {
		return
	}
	t.start()
}

//Stop stop a task
func (d *Download) Stop(task string) {
	t, ok := d.tasks[task]
	if !ok {
		return
	}
	t.stop()
}

//Close close the download instance
func (d *Download) Close() {
	for _, t := range d.tasks {
		t.stop()
	}
	d.cancel()
}

func (d *Download) setDefaultDir(path string) error {
	err := os.MkdirAll(path, os.ModeIrregular)
	if err != nil {
		return err
	}
	d.defaultDir = path
	return nil
}

func isStringPrint(s string) bool {
	for _, c := range s {
		if !unicode.IsPrint(c) {
			return false
		}
	}
	return true
}

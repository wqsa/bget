package bget

import (
	"context"
	"net"
	"os"
	"regexp"
	"strconv"
	"unicode"

	"github.com/google/logger"
	"github.com/pkg/errors"

	"github.com/wqsa/bget/dht"
	"github.com/wqsa/bget/meta"
	"github.com/wqsa/bget/peer"
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
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	if home == "" {
		defaultDir = "./Downloads"
	} else {
		defaultDir = home + "Downloads"
	}

	validVersion = regexp.MustCompile(versionExpr)
}

//Download is top-level instance
type Download struct {
	tasks  map[string]*task
	server *peer.Server
	dht    *dht.DHT
	cfg    *Configuration
	memc   chan int64
	ctx    context.Context
	cancel context.CancelFunc
}

//New creates an instance of Download
func New(cfg *Configuration) (*Download, error) {
	d := new(Download)
	d.tasks = make(map[string]*task)
	d.ctx = context.Background()
	d.ctx, d.cancel = context.WithCancel(d.ctx)
	var err error
	d.server, err = peer.NewServer(net.JoinHostPort(cfg.Options.IP, strconv.Itoa(cfg.Options.Port)))
	if err != nil {
		return nil, err
	}
	d.cfg = cfg
	d.dht, err = dht.NewDHT(net.JoinHostPort(cfg.Options.IP, strconv.Itoa(cfg.Options.DHTPort)), nil, nil)
	if err != nil {
		return nil, err
	}
	go d.dht.Run()
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

func isStringPrint(s string) bool {
	for _, c := range s {
		if !unicode.IsPrint(c) {
			return false
		}
	}
	return true
}

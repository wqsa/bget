package bget

import (
	"os"
	"path"
	"time"
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

type configWrapper interface {
	Ident() string
	Version() string
	ConfigPath() string
	Folder() string
	// SetFolder(fld FolderConfiguration) error
}

//Configuration is Configuration of Download
type Configuration struct {
	Version     string
	Ident       string
	IP          string
	Port        int
	DHTPort     int
	Folder      string
	Cache       int64
	MaxDownload int
	Options     OptionsConfiguration
}

//NewConfig create a default configure
func NewConfig() *Configuration {
	cfg := &Configuration{
		Version:     defaultVersion,
		Ident:       defaultIdentifier,
		IP:          "0.0.0.0",
		Port:        6881,
		DHTPort:     6881,
		Cache:       maxMemory,
		MaxDownload: maxTask,
	}
	home, err := os.UserHomeDir()
	if err == nil {
		cfg.Folder = path.Join(home, "Downloads")
	}
	return cfg
}

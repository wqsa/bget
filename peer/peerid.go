package peer

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/wqsa/bget/common"
)

var clientPrefix = map[string]string{
	"7T": "aTorrent for Android",
	"AB": "AnyEvent::BitTorrent",
	"AG": "Ares",
	"A~": "Ares",
	"AR": "Arctic",
	"AV": "Avicora",
	"AT": "Artemis",
	"AX": "BitPump",
	"AZ": "Azureus",
	"BB": "BitBuddy",
	"BC": "BitComet",
	"BE": "Baretorrent",
	"BF": "Bitflu",
	"BG": "BTG (uses Rasterbar libtorrent)",
	"BL": "BitCometLite (uses 6 digit version number)",
	//"BL": "BitBlinder",
	"BP": "BitTorrent Pro (Azureus + spyware)",
	"BR": "BitRocket",
	"BS": "BTSlave",
	"BT": "mainline BitTorrent (versions >= 7.9)",
	//"BT": "BBtor",
	"Bt": "Bt",
	"BW": "BitWombat",
	"BX": "~Bittorrent X",
	"CD": "Enhanced CTorrent",
	"CT": "CTorrent",
	"DE": "DelugeTorrent",
	"DP": "Propagate Data Client",
	"EB": "EBit",
	"ES": "electric sheep",
	"FC": "FileCroc",
	"FD": "Free Download Manager (versions >= 5.1.12)",
	"FT": "FoxTorrent",
	"FX": "Freebox BitTorrent",
	"GS": "GSTorrent",
	"HK": "Hekate",
	"HL": "Halite",
	"HM": "hMule (uses Rasterbar libtorrent)",
	"HN": "Hydranode",
	"IL": "iLivid",
	"JS": "Justseed.it client",
	"JT": "JavaTorrent",
	"KG": "KGet",
	"KT": "KTorrent",
	"LC": "LeechCraft",
	"LH": "LH:ABC",
	"LP": "Lphant",
	"LT": "libtorrent",
	"lt": "libTorrent",
	"LW": "LimeWire",
	"MK": "Meerkat",
	"MO": "MonoTorrent",
	"MP": "MooPolice",
	"MR": "Miro",
	"MT": "MoonlightTorrent",
	"NB": "Net::BitTorrent",
	"NX": "Net Transport",
	"OS": "OneSwarm",
	"OT": "OmegaTorrent",
	"PB": "Protocol::BitTorrent",
	"PD": "Pando",
	"PI": "PicoTorrent",
	"PT": "PHPTracker",
	"qB": "qBittorrent",
	"QD": "QQDownload",
	"QT": "Qt 4 Torrent example",
	"RT": "Retriever",
	"RZ": "RezTorrent",
	"S~": "Shareaza alpha/beta",
	"SB": "~Swiftbit",
	"SD": "Thunder (aka XùnLéi)",
	"SM": "SoMud",
	"SP": "BitSpirit",
	"SS": "SwarmScope",
	"ST": "SymTorrent",
	"st": "sharktorrent",
	"SZ": "Shareaza",
	"TB": "Torch",
	"TE": "terasaur Seed Bank",
	"TL": "Tribler (versions >= 6.1.0)",
	"TN": "TorrentDotNET",
	"TR": "Transmission",
	"TS": "Torrentstorm",
	"TT": "TuoTu",
	"UL": "uLeecher!",
	"UM": "µTorrent for Mac",
	"UT": "µTorrent",
	"VG": "Vagaa",
	"WD": "WebTorrent Desktop",
	"WT": "BitLet",
	"WW": "WebTorrent",
	"WY": "FireTorrent",
	"XF": "Xfplay",
	"XL": "Xunlei",
	"XS": "XSwifter",
	"XT": "XanTorrent",
	"XX": "Xtorrent",
	"ZT": "ZipTorrent",
}

type peerStyle int

//peerID style
const (
	styleAzureus peerStyle = iota
	styleShadow
)

const (
	//see https://semver.org
	versionExpr = `([0-9]\.){2}[0-9](\-{1}([0-9A-Za-z]+\.)*[[0-9A-Za-z]+)?(\+{1}([0-9A-Za-z]+\.)*[[0-9A-Za-z]+)?`

	idLength = 20
)

var (
	validVersion *regexp.Regexp
)

func init() {
	validVersion = regexp.MustCompile(versionExpr)
}

type peerIDError struct {
	style peerStyle
	msg   string
}

type peerID [idLength]byte

func (e *peerIDError) Error() string {
	switch e.style {
	case styleAzureus:
		return e.msg + "for Azureus style peer ID"
	case styleShadow:
		return e.msg + "for Shadow style peer ID"
	}
	return e.msg + "for unknow style peer ID"
}

//newID return a new peer id
func newID(style peerStyle, client, version string) (peerID, error) {
	switch style {
	case styleAzureus:
		return newAzureusID(client, version)
	case styleShadow:
		return newShadowID(client, version)
	}
	return peerID{}, errors.New("unknow style")
}

//newAzureusID return Azureus style peer id, for detail:https://wiki.theory.org/index.php/BitTorrentSpecification#peer_id
func newAzureusID(client, version string) (peerID, error) {
	if !bytes.Equal(validVersion.Find([]byte(version)), []byte(version)) {
		return peerID{}, &peerIDError{styleAzureus, "invaild version"}
	}
	if len(client) > 2 {
		client = client[:2]
	}
	v := strings.Split(version, ".")
	if len(v) > 3 {
		v = v[:3]
	}
	identifier := "-" + client + strings.Join(v, "") + "-"
	id := [idLength]byte{}
	copy(id[:], []byte(identifier))
	b := common.RandBytes(idLength - len(identifier))
	copy(id[len(identifier):], b)
	return peerID(id), nil
}

//newShadowID return Shadow style peer id, for detail:https://wiki.theory.org/index.php/BitTorrentSpecification#peer_id
func newShadowID(client, version string) (peerID, error) {
	if !bytes.Equal(validVersion.Find([]byte(version)), []byte(version)) {
		return peerID{}, &peerIDError{styleShadow, "invaild version"}
	}
	if len(client) > 1 {
		client = client[:1]
	}
	v := strings.Split(version, ".")
	if len(v) > 3 {
		v = v[:3]
	}
	identifier := client
	for i := range v {
		n, err := strconv.Atoi(v[i])
		if err == nil {
			panic(err)
		}
		if n < 10 {
			identifier += v[i]
		} else if n < 36 {
			identifier += string(byte('A') + byte(n) - 10)
		} else if n < 62 {
			identifier += string(byte('a') + byte(n) - 36)
		} else if n == 62 {
			identifier += "."
		} else if n == 63 {
			identifier += "-"
		} else {
			return peerID{}, &peerIDError{styleShadow, "version reach max"}
		}
	}
	id := [idLength]byte{}
	copy(id[:], []byte(identifier))
	b := common.RandBytes(idLength - len(identifier))
	copy(id[len(identifier):], b)
	return peerID(id), nil
}

func newRandomID() peerID {
	b := common.RandBytes(idLength)
	id := [PeerIDLen]byte{}
	copy(id[:], b[:idLength])
	return peerID(id)
}

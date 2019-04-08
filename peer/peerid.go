package peer

import (
	"math/rand"
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

const (
	styAzureus peerStyle = iota
	styShadow
)

type peerIDError struct {
	style peerStyle
	msg   string
}

func (e *peerIDError) Error() string {
	switch e.style {
	case styAzureus:
		return e.msg + "for Azureus style peer ID"
	case styShadow:
		return e.msg + "for Shadow style peer ID"
	}
	return e.msg + "for unknow style peer ID"
}

func getID(style peerStyle, prefix, version string) [PeerIDLen]byte {
	switch style {
	case styAzureus:
		return getAzureusID(prefix, version)
	}
	return [PeerIDLen]byte{}
}

func getAzureusID(prefix, version string) [PeerIDLen]byte {
	id := [PeerIDLen]byte{}
	copy(id[:], []byte(prefix))
	index := len(prefix)
	for i := range version {
		if version[i] != '.' {
			id[index] = version[i]
			index++
		}
	}
	for index < 20 {
		id[index] = byte(rand.Int() % 10)
		index++
	}
	return id
}

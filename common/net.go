package common

import (
	"encoding/binary"
	"net"
	"strconv"

	"github.com/pkg/errors"
)

const (
	uint32Len = 4
	uint16Len = 2
)

var (
	errBaseLen = errors.New("insufficient data for base length type")
)

//ParseCompressIPV4Addr parse [6]byte to IPV4 addr
func ParseCompressIPV4Addr(data []byte) (string, error) {
	if len(data) == 0 || len(data) != 6 {
		return "", errors.Errorf("format error, data len:%v", len(data))
	}
	host := net.IP(data[:4]).String()
	port := binary.BigEndian.Uint16(data[4:])
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

//ParseCompressIPV4Addrs parse many addrs for []byte
//handle nil ?
func ParseCompressIPV4Addrs(data []byte) ([]string, error) {
	if len(data) == 0 || len(data)%6 != 0 {
		return nil, errors.Errorf("format error, data len:%v", len(data))
	}
	addrs := make([]string, 0, len(data)/6)
	for i := 0; i < len(data); i += 6 {
		addr, err := ParseCompressIPV4Addr(data[i : i+6])
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

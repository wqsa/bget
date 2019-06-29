package utils

import (
	"errors"
	"net"
	"strconv"
)

const (
	uint32Len = 4
	uint16Len = 2
)

var (
	errBaseLen = errors.New("insufficient data for base length type")

	errFormat = errors.New("format error")
)

//ParseIPV4Addr parse [6]byte to IPV4 addr
func ParseIPV4Addr(data []byte) (string, error) {
	if len(data) == 0 || len(data) != 6 {
		return "", errFormat
	}
	addr := net.IP(data[:4]).String() + ":"
	port, _, _ := unpackUint16(data, 4)
	addr += strconv.Itoa(int(port))
	return addr, nil
}

//ParseIPV4Addrs parse many addrs for []byte
//handle nil ?
func ParseIPV4Addrs(data []byte) ([]string, error) {
	if len(data) == 0 || len(data)%6 != 0 {
		return nil, errFormat
	}
	addrs := make([]string, 0, len(data)/6)
	for i := 0; i < len(data); i += 6 {
		addr, err := ParseIPV4Addr(data[i : i+6])
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

func unpackUint32(msg []byte, off int) (uint32, int, error) {
	if off+uint32Len > len(msg) {
		return 0, off, errBaseLen
	}
	v := uint32(msg[off])<<24 | uint32(msg[off+1])<<16 | uint32(msg[off+2])<<8 | uint32(msg[off+3])
	return v, off + uint32Len, nil
}

func unpackUint16(msg []byte, off int) (uint16, int, error) {
	if off+uint16Len > len(msg) {
		return 0, off, errBaseLen
	}
	v := uint16(uint32(msg[off])<<8 | uint32(msg[off+1]))
	return v, off + uint16Len, nil
}

package common

import "math/rand"

//RandBytes produce a size length byte slice randomly
func RandBytes(size int) []byte {
	b := make([]byte, size)
	for i := 0; i < size/4; i++ {
		n := rand.Uint64()
		for j := uint(0); j < 64 && i+int(j) < size; j += 8 {
			b[i+int(j)] = byte(uint(n) & (0xff << j) >> j)
		}
	}
	return b
}

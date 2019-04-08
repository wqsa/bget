package bitmap

import (
	"bytes"
	"testing"
)

var testCase = []struct {
	size int
	pos  int
	val  []byte
	err  error
}{
	{10, 1, []byte{1 << 6, 0}, nil},
	{7, 6, []byte{1 << 1}, nil},
	{0, 0, nil, errOutOfRange},
	{16, 16, []byte{0, 0}, errOutOfRange},
	{16, 17, []byte{0, 0}, errOutOfRange},
	{30, 5, []byte{1 << 2, 0, 0, 0}, nil},
	{60, 19, []byte{0, 0, 1 << 4, 0, 0, 0, 0, 0}, nil},
	{70, 69, []byte{0, 0, 0, 0, 0, 0, 0, 0, 1 << 2}, nil},
}

func TestBitmap(t *testing.T) {
	for i, v := range testCase {
		b := NewEmptyBitmap(v.size)
		if b == nil {
			t.Error("bitmap init fail")
		}
		err := b.SetBitOn(v.pos)
		if err != v.err {
			t.Errorf("%v: %s,but want %s\n", i, err, v.err)
		}
		if !bytes.Equal(b.val, v.val) {
			t.Errorf("%v: %v,but want %v\n", i, b.val, v.val)
		}
		if err != nil {
			continue
		}
		on, err := b.GetBit(v.pos)
		if !on {
			t.Errorf("%v: pos is %v, but want %v\n", i, on, true)
		}
		err = b.SetBitOff(v.pos)
		if err != nil {
			t.Errorf("%v: setbitoff fail\n", i)
		}
		on, err = b.GetBit(v.pos)
		if on {
			t.Errorf("%v: pos is %v, but want %v\n", i, on, false)
		}
	}
}

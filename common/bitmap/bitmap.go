package bitmap

import (
	"errors"
	"io"
)

var (
	errOutOfRange = errors.New("out of range")
	errAssignSize = errors.New("size not match")
)

type Bitmap struct {
	val  []byte
	size int
}

func NewEmptyBitmap(size int) *Bitmap {
	if size == 0 {
		return new(Bitmap)
	}
	return &Bitmap{
		val:  make([]byte, (size-1)/8+1),
		size: size,
	}
}

func NewBitmap(data []byte) *Bitmap {
	b := new(Bitmap)
	b.val = data
	b.size = len(data) * 8
	return b
}

func (b *Bitmap) Len() int {
	return b.size
}

func (b *Bitmap) Truncate(size int) {
	if size > b.size {
		panic(errOutOfRange)
	}
	if (b.size-size)/8 > 0 {
		val := make([]byte, (size-1)/8+1)
		copy(val, b.val)
		b.val = val
	}
	b.size = size
}

func (b *Bitmap) Assign(o *Bitmap) error {
	if o.size != b.size {
		return errAssignSize
	}
	copy(b.val, o.val)
	return nil
}

func (b *Bitmap) SetBitOn(pos int) error {
	if pos < 0 || pos >= b.size {
		return errOutOfRange
	}
	b.val[pos/8] |= (1 << (7 - uint(pos)%8))
	return nil
}

func (b *Bitmap) SetBitOff(pos int) error {
	if pos < 0 || pos >= b.size {
		return errOutOfRange
	}
	b.val[pos/8] &= ^(1 << (7 - uint(pos)%8))
	return nil
}

func (b *Bitmap) SetRangeBitOn(begin, end int) error {
	if begin < 0 || end > b.size {
		return errOutOfRange
	}
	for i := begin; i < end; i++ {
		b.val[i/8] |= (1 << (7 - uint(i)%8))
	}
	return nil
}

func (b *Bitmap) GetBit(pos int) (bool, error) {
	if pos < 0 || pos >= b.size {
		return false, errOutOfRange
	}
	if b.val[pos/8]&(1<<(7-uint(pos)%8)) == 0 {
		return false, nil
	}
	return true, nil
}

func (b *Bitmap) MustGetBit(pos int) bool {
	on, err := b.GetBit(pos)
	if err != nil {
		panic("MustGetBit")
	}
	return on
}

func (b *Bitmap) CountBitOn(begin, end int) (int, error) {
	if begin < 0 || end > b.size {
		return 0, errOutOfRange
	}
	count := 0
	for i := begin; i < end; i++ {
		if b.MustGetBit(i) {
			count++
		}
	}
	return count, nil
}
func (b *Bitmap) MustCountBitOn(begin, end int) int {
	count, err := b.CountBitOn(begin, end)
	if err != nil {
		panic(err.Error())
	}
	return count
}

func (b *Bitmap) Bytes() []byte {
	return b.val
}

func (b *Bitmap) Not() *Bitmap {
	r := NewEmptyBitmap(b.size)
	for i := range b.val {
		r.val[i] = ^b.val[i]
	}
	return r
}

func (b *Bitmap) And(o *Bitmap) *Bitmap {
	if b.size != o.size {
		return nil
	}
	r := NewEmptyBitmap(b.size)
	for i := range b.val {
		r.val[i] = b.val[i] & o.val[i]
	}
	return r
}

func (b *Bitmap) XOR(o *Bitmap) *Bitmap {
	if b.size != o.size {
		return nil
	}
	r := NewEmptyBitmap(b.size)
	for i := range b.val {
		r.val[i] = b.val[i] ^ o.val[i]
	}
	return r
}

func (b *Bitmap) ReadFrom(r io.Reader) error {
	_, err := r.Read(b.val)
	return err
}

func (b *Bitmap) WriteTo(w io.Writer) error {
	_, err := w.Write(b.val)
	return err
}

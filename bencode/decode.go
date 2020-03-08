package bencode

import (
	"bytes"
	"encoding"
	"fmt"
	"github.com/pkg/errors"
	"reflect"
	"strconv"
)

// Unmarshal parses the b-encoded data and stores the result
// in the value pointed to by v. If v is nil or not a pointer,
// Unmarshal returns an InvalidUnmarshalError.
func Unmarshal(data []byte, v interface{}) error {
	var d decodeState
	err := checkValid(data, &d.scan)
	if err != nil {
		return err
	}

	d.init(data)
	return d.unmarshal(v)
}

// Unmarshaler is the interface implemented by types
// that can unmarshal a Bencode description of themselves.
// The input can be assumed to be a valid encoding of
// a Bencode value. UnmarshalBencode must copy the Bencode data
// if it wishes to retain the data after returning.
//
// By convention, to approximate the behavior of Unmarshal itself,
// Unmarshalers implement UnmarshalBencode([]byte("null")) as a no-op.
type Unmarshaler interface {
	UnmarshalBencode([]byte) error
}

// An UnmarshalTypeError describes a Bencode value that was
// not appropriate for a value of a specific Go type.
type UnmarshalTypeError struct {
	Value  string       // description of Bencode value - "bool", "array", "number -5"
	Type   reflect.Type // type of Go value it could not be assigned to
	Offset int64        // error occurred after reading Offset bytes
	Struct string       // name of the struct type containing the field
	Field  string       // name of the field holding the Go value
}

func (e *UnmarshalTypeError) Error() string {
	if e.Struct != "" || e.Field != "" {
		return "Bencode: cannot unmarshal " + e.Value + " into Go struct field " + e.Struct + "." + e.Field + " of type " + e.Type.String()
	}
	return "Bencode: cannot unmarshal " + e.Value + " into Go value of type " + e.Type.String()
}

// An InvalidUnmarshalError describes an invalid argument passed to Unmarshal.
// (The argument to Unmarshal must be a non-nil pointer.)
type InvalidUnmarshalError struct {
	Type reflect.Type
}

func (e *InvalidUnmarshalError) Error() string {
	if e.Type == nil {
		return "bencode: Unmarshal(nil)"
	}

	if e.Type.Kind() != reflect.Ptr {
		return "bencode: Unmarshal(non-pointer " + e.Type.String() + ")"
	}
	return "bencode: Unmarshal(nil " + e.Type.String() + ")"
}

func (d *decodeState) unmarshal(v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return &InvalidUnmarshalError{reflect.TypeOf(v)}
	}

	d.scan.reset()
	d.scanWhile(scanContinue)
	err := d.value(rv)
	if err != nil {
		return d.addErrorContext(err)
	}
	return d.savedError
}

type decodeState struct {
	data         []byte
	off          int
	opcode       int
	scan         scanner
	errorContext struct {
		Struct reflect.Type
		Field  string
	}
	savedError            error
	useNumber             bool
	disallowUnknownFields bool
}

func (d *decodeState) readIndex() int {
	return d.off - 1
}

// phasePanicMsg is used as a panic message when we end up with something that
// shouldn't happen. It can indicate a bug in the Bencode decoder, or that
// something is editing the data slice while the decoder executes.
const phasePanicMsg = "Bencode decoder out of sync - data changing underfoot?"

func (d *decodeState) init(data []byte) *decodeState {
	d.data = data
	d.off = 0
	d.savedError = nil
	d.errorContext.Struct = nil
	d.errorContext.Field = ""
	return d
}

func (d *decodeState) saveError(err error) {
	if d.savedError == nil {
		d.savedError = d.addErrorContext(err)
	}
}

func (d *decodeState) addErrorContext(err error) error {
	if d.errorContext.Struct != nil || d.errorContext.Field != "" {
		switch err := err.(type) {
		case *UnmarshalTypeError:
			err.Struct = d.errorContext.Struct.Name()
			err.Field = d.errorContext.Field
			return err
		}
	}
	return err
}

func (d *decodeState) skip() {
	s, data, i := &d.scan, d.data, d.off
	depth := len(s.parseState)
	for {
		op := s.step(s, data[i])
		i += int(s.stepSize)
		if len(s.parseState) < depth {
			d.off = i
			d.opcode = op
			return
		}
	}
}

func (d *decodeState) scanNext() {
	if d.off < len(d.data) {
		d.opcode = d.scan.step(&d.scan, d.data[d.off])
		d.off += int(d.scan.stepSize)
	} else {
		d.opcode = d.scan.eof()
		d.off = len(d.data) + 1
	}
}

func (d *decodeState) scanWhile(op int) {
	s, data, i := &d.scan, d.data, d.off
	for i < len(data) {
		newOp := s.step(s, data[i])
		i += int(s.stepSize)
		if newOp != op {
			d.opcode = newOp
			d.off = i
			return
		}
	}

	d.off = len(data) + 1
	d.opcode = d.scan.eof()
}

func (d *decodeState) value(v reflect.Value) error {
	switch d.opcode {
	default:
		panic(phasePanicMsg)

	case scanBeginList:
		if v.IsValid() {
			if err := d.list(v); err != nil {
				return err
			}
		} else {
			d.skip()
		}
		d.scanNext()

	case scanBeginDict:
		if v.IsValid() {
			if err := d.dict(v); err != nil {
				return err
			}
		} else {
			d.skip()
		}
		d.scanNext()

	case scanBeginString:
		d.scanWhile(scanContinue)
		start := d.readIndex() + 1
		d.scanWhile(scanContinue)

		if v.IsValid() {
			if err := d.stringStore(d.data[start:d.readIndex()], v); err != nil {
				return err
			}
		}

	case scanBeginInt:
		start := d.readIndex() + 1
		d.scanWhile(scanContinue)

		if v.IsValid() {
			if err := d.intStore(d.data[start:d.readIndex()], v); err != nil {
				return err
			}
		}
		d.scanNext()
	}
	return nil
}

// indirect walks down v allocating pointers as needed,
// until it gets to a non-pointer.
// if it encounters an Unmarshaler, indirect stops and returns that.
func indirect(v reflect.Value) (Unmarshaler, encoding.TextUnmarshaler, reflect.Value) {
	v0 := v
	haveAddr := false

	// If v is a named type and is addressable,
	// start with its address, so that if the type has pointer methods,
	// we find them.
	if v.Kind() != reflect.Ptr && v.Type().Name() != "" && v.CanAddr() {
		haveAddr = true
		v = v.Addr()
	}
	for {
		// Load value from interface, but only if the result will be
		// usefully addressable.
		if v.Kind() == reflect.Interface && !v.IsNil() {
			e := v.Elem()
			if e.Kind() == reflect.Ptr && !e.IsNil() && e.Elem().Kind() == reflect.Ptr {
				haveAddr = false
				v = e
				continue
			}
		}

		if v.Kind() != reflect.Ptr {
			break
		}

		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		if v.Type().NumMethod() > 0 && v.CanInterface() {
			if u, ok := v.Interface().(Unmarshaler); ok {
				return u, nil, reflect.Value{}
			}
			if u, ok := v.Interface().(encoding.TextUnmarshaler); ok {
				return nil, u, reflect.Value{}
			}
		}

		if haveAddr {
			v = v0 // restore original value after round-trip Value.Addr().Elem()
			haveAddr = false
		} else {
			v = v.Elem()
		}
	}
	return nil, nil, v
}

func (d *decodeState) list(v reflect.Value) error {
	u, ut, pv := indirect(v)
	if u != nil {
		start := d.readIndex()
		d.skip()
		return u.UnmarshalBencode(d.data[start:d.off])
	}
	if ut != nil {
		d.saveError(&UnmarshalTypeError{Value: "list", Type: v.Type(), Offset: int64(d.off)})
		d.skip()
		return nil
	}
	v = pv

	switch v.Kind() {
	case reflect.Interface:
		if v.NumMethod() == 0 {
			ai := d.listInterface()
			v.Set(reflect.ValueOf(ai))
			return nil
		}
		fallthrough
	default:
		d.saveError(&UnmarshalTypeError{Value: "list", Type: v.Type(), Offset: int64(d.off)})
		d.skip()
		return nil
	case reflect.Array, reflect.Slice:
		break
	}

	d.scanWhile(scanContinue)

	i := 0
	for {
		if d.opcode == scanEndList {
			break
		}

		if v.Kind() == reflect.Slice {
			if i >= v.Cap() {
				newcap := v.Cap() + v.Cap()/2
				if newcap < 4 {
					newcap = 4
				}
				newv := reflect.MakeSlice(v.Type(), v.Len(), newcap)
				reflect.Copy(newv, v)
				v.Set(newv)
			}
			if i >= v.Len() {
				v.SetLen(i + 1)
			}
		}

		if i < v.Len() {
			if err := d.value(v.Index(i)); err != nil {
				return err
			}
		} else {
			if err := d.value(reflect.Value{}); err != nil {
				return err
			}
		}
		i++

		if d.opcode == scanEndList {
			break
		}
	}

	if i < v.Len() {
		if v.Kind() == reflect.Array {
			z := reflect.Zero(v.Type().Elem())
			for ; i < v.Len(); i++ {
				v.Index(i).Set(z)
			}
		} else {
			v.SetLen(i)
		}
	}
	if i == 0 && v.Kind() == reflect.Slice {
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	}
	return nil
}

func (d *decodeState) dict(v reflect.Value) error {
	u, ut, pv := indirect(v)
	if u != nil {
		start := d.readIndex()
		d.skip()
		return u.UnmarshalBencode(d.data[start:d.off])
	}
	if ut != nil {
		d.saveError(&UnmarshalTypeError{Value: "object", Type: v.Type(), Offset: int64(d.off)})
		d.skip()
		return nil
	}
	v = pv
	t := v.Type()

	if v.Kind() == reflect.Interface && v.NumMethod() == 0 {
		oi := d.dictInterface()
		v.Set(reflect.ValueOf(oi))
		return nil
	}

	var fields []field

	switch v.Kind() {
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			d.saveError(&UnmarshalTypeError{Value: "dict", Type: t, Offset: int64(d.off)})
			d.skip()
			return nil
		}
		if v.IsNil() {
			v.Set(reflect.MakeMap(t))
		}
	case reflect.Struct:
		fields = cachedTypeFields(t)
	default:
		d.saveError(&UnmarshalTypeError{Value: "dict", Type: t, Offset: int64(d.off)})
		d.skip()
		return nil
	}

	var mapElem reflect.Value
	originalErrorContext := d.errorContext

	d.scanWhile(scanContinue)

	for {
		if d.opcode == scanEndDict {
			break
		}
		if d.opcode != scanBeginString {
			panic(phasePanicMsg)
		}

		d.scanWhile(scanContinue)

		start := d.readIndex() + 1
		d.scanWhile(scanContinue)
		key := d.data[start:d.readIndex()]

		var subv reflect.Value

		if v.Kind() == reflect.Map {
			elemType := t.Elem()
			if !mapElem.IsValid() {
				mapElem = reflect.New(elemType).Elem()
			} else {
				mapElem.Set(reflect.Zero(elemType))
			}
			subv = mapElem
		} else {
			var f *field
			for i := range fields {
				ff := &fields[i]
				if bytes.Equal(ff.nameBytes, key) {
					f = ff
					break
				}
				if f == nil && ff.equalFold(ff.nameBytes, key) {
					f = ff
				}
			}
			if f != nil {
				subv = v
				for _, i := range f.index {
					if subv.Kind() == reflect.Ptr {
						if subv.IsNil() {
							if !subv.CanSet() {
								d.saveError(fmt.Errorf("bencode: cannot set embedded pointer to unexported struct: %v", subv.Type().Elem()))
								subv = reflect.Value{}
								break
							}
							subv.Set(reflect.New(subv.Type().Elem()))
						}
						subv = subv.Elem()
					}
					subv = subv.Field(i)
				}
				d.errorContext.Field = f.name
				d.errorContext.Struct = t
			} else if d.disallowUnknownFields {
				d.saveError(fmt.Errorf("bencode: unknown field %q", key))
			}
		}

		if err := d.value(subv); err != nil {
			return err
		}

		if v.Kind() == reflect.Map {
			kv := reflect.ValueOf(string(key))
			if kv.IsValid() {
				v.SetMapIndex(kv, subv)
			}
		}

		if d.opcode == scanEndDict {
			break
		}

		d.errorContext = originalErrorContext
	}
	return nil
}

func (d *decodeState) stringStore(item []byte, v reflect.Value) error {
	u, ut, pv := indirect(v)
	if u != nil {
		return u.UnmarshalBencode(item)
	}
	if ut != nil {
		return ut.UnmarshalText(item)
	}

	v = pv

	s := item
	switch v.Kind() {
	default:
		d.saveError(&UnmarshalTypeError{Value: "string", Type: v.Type(), Offset: int64(d.readIndex())})
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.Uint8 {
			d.saveError(&UnmarshalTypeError{Value: "string", Type: v.Type(), Offset: int64(d.readIndex())})
			break
		}
		v.SetBytes(s)
	case reflect.String:
		v.SetString(string(s))
	case reflect.Interface:
		if v.NumMethod() == 0 {
			v.Set(reflect.ValueOf(string(s)))
		} else {
			d.saveError(&UnmarshalTypeError{Value: "string", Type: v.Type(), Offset: int64(d.readIndex())})
		}
	}
	return nil
}

func (d *decodeState) intStore(item []byte, v reflect.Value) error {
	if len(item) == 0 {
		d.saveError(fmt.Errorf("bencode: invalid use of ,trying to unmarshal \"\" into %v", v.Type()))
		return nil
	}
	u, ut, pv := indirect(v)
	if u != nil {
		return u.UnmarshalBencode(item)
	}
	if ut != nil {
		d.saveError(&UnmarshalTypeError{Value: "number", Type: v.Type(), Offset: int64(d.readIndex())})
		return nil
	}

	v = pv

	if item[0] != '-' && (item[0] < '0' || item[0] > '9') {
		return fmt.Errorf("bencode: invalid number literal, trying to unmarshal %q into Number", item)
	}
	s := string(item)
	switch v.Kind() {
	default:
		d.saveError(&UnmarshalTypeError{Value: "number", Type: v.Type(), Offset: int64(d.readIndex())})
	case reflect.Interface:
		n, err := strconv.Atoi(s)
		if err != nil {
			d.saveError(err)
			break
		}
		if v.NumMethod() != 0 {
			d.saveError(&UnmarshalTypeError{Value: "number", Type: v.Type(), Offset: int64(d.readIndex())})
			break
		}
		v.Set(reflect.ValueOf(n))

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || v.OverflowInt(n) {
			d.saveError(&UnmarshalTypeError{Value: "number " + s, Type: v.Type(), Offset: int64(d.readIndex())})
			break
		}
		v.SetInt(n)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil || v.OverflowUint(n) {
			d.saveError(&UnmarshalTypeError{Value: "number " + s, Type: v.Type(), Offset: int64(d.readIndex())})
			break
		}
		v.SetUint(n)

	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(s, v.Type().Bits())
		if err != nil || v.OverflowFloat(n) {
			d.saveError(&UnmarshalTypeError{Value: "number " + s, Type: v.Type(), Offset: int64(d.readIndex())})
			break
		}
		v.SetFloat(n)
	}
	return nil
}

func (d *decodeState) valueInterface() (val interface{}) {
	switch d.opcode {
	default:
		panic(phasePanicMsg)
	case scanBeginList:
		val = d.listInterface()
		d.scanNext()
	case scanBeginDict:
		val = d.dictInterface()
		d.scanNext()
	case scanBeginString:
		val = d.stringInterface()
	case scanBeginInt:
		val = d.intInterface()
		d.scanNext()
	}
	return
}

func (d *decodeState) listInterface() []interface{} {
	var v = make([]interface{}, 0)
	d.scanNext()
	for {
		if d.opcode == scanEndList {
			break
		}

		v = append(v, d.valueInterface())

		if d.opcode == scanEndList {
			break
		}
	}
	return v
}

func (d *decodeState) dictInterface() map[string]interface{} {
	m := make(map[string]interface{})
	d.scanNext()
	for {
		if d.opcode == scanEndDict {
			break
		}
		if d.opcode != scanBeginString {
			panic(phasePanicMsg)
		}
		d.scanWhile(scanContinue)
		start := d.readIndex() + 1
		d.scanWhile(scanContinue)
		item := d.data[start:d.readIndex()]
		key := string(item)

		m[key] = d.valueInterface()

		if d.opcode == scanEndDict {
			break
		}
	}
	return m
}

func (d *decodeState) stringInterface() interface{} {
	d.scanWhile(scanContinue)
	start := d.readIndex() + 1
	d.scanWhile(scanContinue)

	item := d.data[start:d.readIndex()]

	return string(item)
}

func (d *decodeState) intInterface() interface{} {
	start := d.readIndex() + 1
	d.scanWhile(scanContinue)

	item := d.data[start:d.readIndex()]

	if item[0] != '-' && (item[0] < '0' || item[0] > '9') {
		return fmt.Errorf("bencode: invalid number literal, trying to unmarshal %q into Number", item)
	}

	n, err := strconv.Atoi(string(item))
	if err != nil {
		d.saveError(err)
	}
	return n
}

// RawMessage is a raw encoded bencode value.
type RawMessage []byte

// MarshalBencode returns m as the JSON encoding of m.
func (m RawMessage) MarshalBencode() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}

// UnmarshalBencode sets *m to a copy of data.
func (m *RawMessage) UnmarshalBencode(data []byte) error {
	if m == nil {
		return errors.New("json.RawMessage: UnmarshalJSON on nil pointer")
	}
	*m = append((*m)[0:0], data...)
	return nil
}

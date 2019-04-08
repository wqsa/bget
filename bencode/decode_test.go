package bencode

import (
	"reflect"
	"testing"
)

type T1 struct {
	A   int
	Bc  string
	Def []int
}

type T2 struct {
	A   int    `bencode:"e"`
	Bc  string `bencode:"de,omitpty"`
	Def []int8
}

type unmarshalTest struct {
	in  string
	ptr interface{}
	out interface{}
	err error
}

var unmarshalTests = []unmarshalTest{
	{in: "i0e", ptr: new(int), out: 0},
	{in: "i1024e", ptr: new(int), out: 1024},
	{in: "i-2048e", ptr: new(int), out: -2048},
	{in: "i100e", ptr: new(uint32), out: uint32(100)},
	{in: "i100e", ptr: new(uint64), out: uint64(100)},
	{in: "i100e", ptr: new(int64), out: int64(100)},
	{in: "i1152921504606846976e", ptr: new(int64), out: int64(1152921504606846976)},

	{in: "1:a", ptr: new(string), out: "a"},
	{in: "5:abcde", ptr: new(string), out: "abcde"},

	{in: "d1:a2:bc3:def4:ghije", ptr: &map[string]string{}, out: map[string]string{"a": "bc", "def": "ghij"}},
	{in: "d1:ai12e3:defi23ee", ptr: &map[string]int{}, out: map[string]int{"a": 12, "def": 23}},

	{in: "li10e3:abci8ee", ptr: new([]interface{}), out: []interface{}{10, "abc", 8}},

	{in: "d1:Ai100e2:Bc3:qwe3:Defli10ei8ei9eee", ptr: new(T1), out: T1{A: 100, Bc: "qwe", Def: []int{10, 8, 9}}},
	{in: "d1:ei100e2:de3:qwe3:Defli3ei100eee", ptr: new(T2), out: T2{A: 100, Bc: "qwe", Def: []int8{3, 100}}},
	//TODO keep consist unmarshal with marshal for the struct with omitpty field?
	//{in: "d1:ei100e3:Defli3ei100eee", ptr: new(T2), out: T2{A: 100, Def: []int8{3, 100}}},
}

func TestUnmarshal(t *testing.T) {
	for i, v := range unmarshalTests {
		if err := Unmarshal([]byte(v.in), v.ptr); err != nil {
			t.Error(i, ":", err)
		}
		v1 := reflect.ValueOf(v.ptr).Elem().Interface()
		if !reflect.DeepEqual(v1, v.out) {
			t.Errorf("#%d: %T,%#v, want %T, %#v", i, v1, v1, v.out, v.out)
		}

		data, err := Marshal(v.out)
		if err != nil {
			t.Error(i, ":", err)
		}
		if string(data) != v.in {
			t.Errorf("#%d: %v, want %v", i, string(data), v.in)
		}
	}
}

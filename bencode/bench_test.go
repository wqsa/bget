package bencode

import (
	"io/ioutil"
	"os"
	"testing"
)

var codeBencode []byte
var codeStruct []interface{}

func codeInit() {
	dir, err := os.Open("testdata")
	if err != nil {
		panic(err)
	}
	defer dir.Close()
	files, err := dir.Readdir(0)
	if err != nil {
		panic(err)
	}
	data := []byte{'l'}
	for _, file := range files {
		f, err := os.Open("testdata/" + file.Name())
		if err != nil {
			panic(err)
		}
		defer f.Close()
		content, err := ioutil.ReadAll(f)
		if err != nil {
			panic(err)
		}
		data = append(data, content...)
	}
	data = append(data, 'e')

	codeBencode = data

	if err := Unmarshal(codeBencode, &codeStruct); err != nil {
		panic("unmarshal code.bencode: " + err.Error())
	}

	if data, err = Marshal(&codeStruct); err != nil {
		panic("marshal code.bencode: " + err.Error())
	}
}

func BenchmarkCodeMarshal(b *testing.B) {
	if codeBencode == nil {
		b.StopTimer()
		codeInit()
		b.StartTimer()
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Marshal(&codeStruct); err != nil {
				b.Fatal("Marshal:", err)
			}
		}
	})
	b.SetBytes(int64(len(codeBencode)))
}

func BenchmarkCodeUnmarshal(b *testing.B) {
	if codeBencode == nil {
		b.StopTimer()
		codeInit()
		b.StartTimer()
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var r []interface{}
			if err := Unmarshal(codeBencode, &r); err != nil {
				b.Fatal("Unmarshal:", err)
			}
		}
	})
	b.SetBytes(int64(len(codeBencode)))
}

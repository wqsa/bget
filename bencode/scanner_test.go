package bencode

import (
	"testing"
)

var validTests = []struct {
	data string
	ok   bool
}{
	{"foo", false},
	{"ed", false},
	{"3:12", false},
	{"ie", false},
	{"i-0e", false},
	{"i01e", false},
	{"i0e", true},
	{"i-100e", true},
	{"de", true},
	{"le", true},
	{"3:foo", true},
	{"i100e", true},
	{"d3:foo3:bare", true},
	{"d3:fool2:ab2:cdee", true},
	{"d3:food2:ab2:cdee", true},
}

func TestValid(t *testing.T) {
	for _, tt := range validTests {
		if ok := Valid([]byte(tt.data)); ok != tt.ok {
			t.Errorf("Valid(%#q) = %v, want %v", tt.data, ok, tt.ok)
		}
	}
}

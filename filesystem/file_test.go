package filesystem

import "testing"

func TestFileCreate(t *testing.T) {
	f := file{path: "test", length: 1024 * 1024 * 1024}
	f.create()
}

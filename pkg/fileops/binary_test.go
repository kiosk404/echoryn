package fileops

import "testing"

func TestIsBinaryByExtension(t *testing.T) {
	if !IsBinary("x.png", nil) {
		t.Error(".png must be binary")
	}
	if IsBinary("x.go", nil) {
		t.Error(".go not binary")
	}
}

func TestIsBinaryByContentSample(t *testing.T) {
	// Use .dat (not in BinaryExtensions) so sampling takes effect.
	if !IsBinary("u.dat", []byte{0, 0, 0, 0, 0, 'h', 'i'}) {
		t.Error("NUL-heavy should be binary")
	}
	if IsBinary("u.dat", []byte("hello\n")) {
		t.Error("ASCII not binary")
	}
}

func TestIsImage(t *testing.T) {
	if !IsImage("a.png") || !IsImage("b.JPG") {
		t.Error("image ext")
	}
	if IsImage("c.txt") {
		t.Error(".txt not image")
	}
}

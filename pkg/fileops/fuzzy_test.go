package fileops

import "testing"

func TestFindBestMatchExact(t *testing.T) {
	idx, length, err := FindBestMatch("hello\nworld\nfoo\n", "world")
	if err != nil {
		t.Fatal(err)
	}
	if idx != 6 || length != 5 {
		t.Errorf("idx=%d length=%d", idx, length)
	}
}

func TestFindBestMatchTrimWhitespace(t *testing.T) {
	idx, length, err := FindBestMatch("a\n  target  \nb", "target")
	if err != nil {
		t.Fatal(err)
	}
	if idx < 0 || length < len("target") {
		t.Errorf("idx=%d length=%d", idx, length)
	}
}

func TestFindBestMatchLineEndings(t *testing.T) {
	idx, length, err := FindBestMatch("a\r\nb\r\nc\r\n", "b\nc")
	if err != nil {
		t.Fatal(err)
	}
	if idx < 0 {
		t.Errorf("idx=%d length=%d", idx, length)
	}
}

func TestFindBestMatchNoMatch(t *testing.T) {
	if _, _, err := FindBestMatch("abcdef", "zzz"); err == nil {
		t.Error("expected not-found")
	}
}

func TestFindAllExact(t *testing.T) {
	ranges, err := FindAllExact("foo bar foo baz foo", "foo")
	if err != nil {
		t.Fatal(err)
	}
	if len(ranges) != 3 {
		t.Errorf("got %d ranges", len(ranges))
	}
}

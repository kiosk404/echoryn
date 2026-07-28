package fileops

import (
	"path/filepath"
	"strings"
)

// BinaryExtensions lists common non-text extensions. Kept conservative --
// false negatives are fine (we fall back to content sampling); false positives
// prevent reading valid text.
var BinaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".bmp": true, ".ico": true, ".tif": true, ".tiff": true,
	".pdf": true, ".zip": true, ".gz": true, ".tar": true, ".bz2": true,
	".xz": true, ".7z": true, ".rar": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".o": true,
	".a": true, ".class": true, ".jar": true, ".war": true,
	".mp3": true, ".mp4": true, ".wav": true, ".flac": true, ".ogg": true,
	".avi": true, ".mov": true, ".mkv": true, ".webm": true,
	".sqlite": true, ".db": true, ".bin": true,
}

// ImageExtensions is a subset of BinaryExtensions we can return as base64.
var ImageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".webp": true, ".bmp": true, ".ico": true,
}

// IsBinary returns true if the file is likely binary. Extension lookup is the
// fast path; content sampling is the fallback when extension is inconclusive.
func IsBinary(path string, sample []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if BinaryExtensions[ext] {
		return true
	}
	if len(sample) == 0 {
		return false
	}
	n := len(sample)
	if n > 1024 {
		n = 1024
	}
	nonPrintable := 0
	for i := 0; i < n; i++ {
		c := sample[i]
		if c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
			nonPrintable++
		}
	}
	return float64(nonPrintable)/float64(n) > 0.30
}

// IsImage returns true if the extension is in ImageExtensions.
func IsImage(path string) bool {
	return ImageExtensions[strings.ToLower(filepath.Ext(path))]
}

package mime

import (
	gomime "mime"
	"path"
)

const defaultMimeDir = "httpd/unix-directory"

var mimes map[string]string

func init() {
	mimes = map[string]string{}
}

// RegisterMime is a package level function that registers
// a mime type with the given extension.
func RegisterMime(ext, mime string) {
	mimes[ext] = mime
}

// Detect returns the mimetype associated with the given filename.
func Detect(isDir bool, fn string) string {
	if isDir {
		return defaultMimeDir
	}

	ext := path.Ext(fn)

	mimeType := getCustomMime(ext)

	if mimeType == "" {
		mimeType = gomime.TypeByExtension(ext)
	}

	return mimeType
}

func getCustomMime(ext string) string {
	return mimes[ext]
}

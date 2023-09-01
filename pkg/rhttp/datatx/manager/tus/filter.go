package tus

import (
	"net/http"
	"strings"

	tusd "github.com/tus/tusd/pkg/handler"
)

type FilterResponseWriter struct {
	w      http.ResponseWriter
	header http.Header
}

const TusPrefix = "tus."
const CS3Prefix = "cs3."

func NewFilterResponseWriter(w http.ResponseWriter) *FilterResponseWriter {
	return &FilterResponseWriter{
		w:      w,
		header: http.Header{},
	}
}

func (fw *FilterResponseWriter) Header() http.Header {
	return fw.w.Header()
}

func (fw *FilterResponseWriter) Write(b []byte) (int, error) {
	return fw.w.Write(b)
}

func (fw *FilterResponseWriter) WriteHeader(statusCode int) {
	metadata := tusd.ParseMetadataHeader(fw.w.Header().Get("Upload-Metadata"))
	tusMetadata := map[string]string{}
	for k, v := range metadata {
		if strings.HasPrefix(k, TusPrefix) {
			tusMetadata[strings.TrimPrefix(k, TusPrefix)] = v
		}
	}

	fw.w.Header().Set("Upload-Metadata", tusd.SerializeMetadataHeader(tusMetadata))
	fw.w.WriteHeader(statusCode)
}

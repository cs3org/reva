package handlers

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/cernbox/reva/pkg/log"
)

// LogHandler is a logging middleware
func LogHandler(l *log.Logger, h http.Handler) http.Handler {
	return newLoggingHandler(l, h)
}

func newLoggingHandler(l *log.Logger, h http.Handler) http.Handler {
	return loggingHandler{l, h}
}

type loggingHandler struct {
	l       *log.Logger
	handler http.Handler
}

func (h loggingHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	t := time.Now()
	logger := makeLogger(w)
	url := *req.URL
	h.handler.ServeHTTP(logger, req)
	writeLog(h.l, req, url, t, logger.Status(), logger.Size())
}

func makeLogger(w http.ResponseWriter) loggingResponseWriter {
	var logger loggingResponseWriter = &responseLogger{w: w, status: http.StatusOK}
	if _, ok := w.(http.Hijacker); ok {
		logger = &hijackLogger{responseLogger{w: w, status: http.StatusOK}}
	}
	h, ok1 := logger.(http.Hijacker)
	c, ok2 := w.(http.CloseNotifier)
	if ok1 && ok2 {
		return hijackCloseNotifier{logger, h, c}
	}
	if ok2 {
		return &closeNotifyWriter{logger, c}
	}
	return logger
}

func writeLog(l *log.Logger, req *http.Request, url url.URL, ts time.Time, status, size int) {
	end := time.Now()
	host, _, err := net.SplitHostPort(req.RemoteAddr)

	if err != nil {
		host = req.RemoteAddr
	}

	uri := req.RequestURI

	if req.ProtoMajor == 2 && req.Method == "CONNECT" {
		uri = req.Host
	}
	if uri == "" {
		uri = url.RequestURI()
	}

	diff := end.Sub(ts).Nanoseconds()

	var b *log.Builder
	if status >= 400 {
		b = l.BuildError()
	} else {
		b = l.Build()
	}
	b.Str("host", host).Str("method", req.Method)
	b = b.Str("uri", uri).Str("proto", req.Proto).Int("status", status)
	b = b.Int("size", size)
	b = b.Str("start", ts.Format("02/Jan/2006:15:04:05 -0700"))
	b = b.Str("end", end.Format("02/Jan/2006:15:04:05 -0700")).Int("time_ns", int(diff))
	b.Msg(req.Context(), "HTTP request finished")
}

type loggingResponseWriter interface {
	commonLoggingResponseWriter
	http.Pusher
}

func (l *responseLogger) Push(target string, opts *http.PushOptions) error {
	p, ok := l.w.(http.Pusher)
	if !ok {
		return fmt.Errorf("responseLogger does not implement http.Pusher")
	}
	return p.Push(target, opts)
}

type commonLoggingResponseWriter interface {
	http.ResponseWriter
	http.Flusher
	Status() int
	Size() int
}

// responseLogger is wrapper of http.ResponseWriter that keeps track of its HTTP
// status code and body size
type responseLogger struct {
	w      http.ResponseWriter
	status int
	size   int
}

func (l *responseLogger) Header() http.Header {
	return l.w.Header()
}

func (l *responseLogger) Write(b []byte) (int, error) {
	size, err := l.w.Write(b)
	l.size += size
	return size, err
}

func (l *responseLogger) WriteHeader(s int) {
	l.w.WriteHeader(s)
	l.status = s
}

func (l *responseLogger) Status() int {
	return l.status
}

func (l *responseLogger) Size() int {
	return l.size
}

func (l *responseLogger) Flush() {
	f, ok := l.w.(http.Flusher)
	if ok {
		f.Flush()
	}
}

type hijackLogger struct {
	responseLogger
}

func (l *hijackLogger) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h := l.responseLogger.w.(http.Hijacker)
	conn, rw, err := h.Hijack()
	if err == nil && l.responseLogger.status == 0 {
		l.responseLogger.status = http.StatusSwitchingProtocols
	}
	return conn, rw, err
}

type closeNotifyWriter struct {
	loggingResponseWriter
	http.CloseNotifier
}

type hijackCloseNotifier struct {
	loggingResponseWriter
	http.Hijacker
	http.CloseNotifier
}

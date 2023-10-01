package http

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// SetTimeout sets a read and write deadline on a net.Conn.
func SetTimeout(conn net.Conn, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	conn.SetReadDeadline(deadline)
	conn.SetWriteDeadline(deadline)
}

// CopyBuffer is a helper function to copy data between two net.Conn objects.
func CopyBuffer(dst, src net.Conn, buf []byte) (int64, error) {
	return io.CopyBuffer(dst, src, buf)
}

type responseWriter struct {
	conn    net.Conn
	headers http.Header
	status  int
	written bool
}

func NewHTTPResponseWriter(conn net.Conn) http.ResponseWriter {
	return &responseWriter{
		conn:    conn,
		headers: http.Header{},
		status:  http.StatusOK,
	}
}

func (rw *responseWriter) Header() http.Header {
	return rw.headers
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	if rw.written {
		return
	}
	rw.status = statusCode
	rw.written = true

	statusText := http.StatusText(statusCode)
	if statusText == "" {
		statusText = fmt.Sprintf("status code %d", statusCode)
	}
	fmt.Fprintf(rw.conn, "HTTP/1.1 %d %s\r\n", statusCode, statusText)
	rw.headers.Write(rw.conn)
	rw.conn.Write([]byte("\r\n"))
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.conn.Write(data)
}

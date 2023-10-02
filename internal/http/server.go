package http

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"time"
)

type Server struct {
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) ServeConn(conn net.Conn) error {
	defer func() {
		_ = conn.Close()
	}()
	SetTimeout(conn, 5*time.Minute) // Set a timeout of 5 minutes

	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return err
	}

	return handleHTTP(conn, req, req.Method == http.MethodConnect)
}

func handleHTTP(conn net.Conn, req *http.Request, isConnectMethod bool) error {
	targetConn, err := net.Dial("tcp", req.URL.Host)
	if err != nil {
		http.Error(
			NewHTTPResponseWriter(conn),
			err.Error(),
			http.StatusServiceUnavailable,
		)
		return err
	}
	defer func() {
		_ = targetConn.Close()
	}()

	if isConnectMethod {
		_, err = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		if err != nil {
			return err
		}
	} else {
		err = req.Write(targetConn)
		if err != nil {
			return err
		}
	}

	errCh := make(chan error, 2)
	go proxy(conn, targetConn, errCh)
	go proxy(targetConn, conn, errCh)

	err = <-errCh
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

func proxy(src, dst net.Conn, errCh chan error) {
	_, err := CopyBuffer(dst, src, make([]byte, 32*1024))
	errCh <- err
}

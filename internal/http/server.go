package http

import (
	"bufio"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

type Server struct {
	auth Authenticator
}

func NewServer() *Server {
	return &Server{
		auth: NewBasicAuthenticator(),
	}
}

func (s *Server) ServeConn(conn net.Conn) {
	defer conn.Close()
	SetTimeout(conn, 5*time.Minute) // Set a timeout of 5 minutes

	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		log.Println(err)
		return
	}

	err = CheckAuth(s.auth, req)
	if err != nil {
		http.Error(
			NewHTTPResponseWriter(conn),
			err.Error(),
			http.StatusUnauthorized,
		)
		return
	}

	if req.Method == http.MethodConnect {
		handleHTTPConnect(conn, req)
	} else {
		handleHTTP(conn, req)
	}
}

func handleHTTPConnect(conn net.Conn, req *http.Request) {
	targetConn, err := net.Dial("tcp", req.URL.Host)
	if err != nil {
		http.Error(
			NewHTTPResponseWriter(conn),
			err.Error(),
			http.StatusServiceUnavailable,
		)
		return
	}
	defer targetConn.Close()

	conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	errCh := make(chan error, 2)
	buf1 := make([]byte, 32*1024)
	buf2 := make([]byte, 32*1024)
	go proxy(conn, targetConn, buf1, errCh)
	go proxy(targetConn, conn, buf2, errCh)

	err = <-errCh
	if err != nil && err != io.EOF {
		log.Println(err)
	}
}

func handleHTTP(conn net.Conn, req *http.Request) {
	targetConn, err := net.Dial("tcp", req.URL.Host)
	if err != nil {
		http.Error(
			NewHTTPResponseWriter(conn),
			err.Error(),
			http.StatusServiceUnavailable,
		)
		return
	}
	defer targetConn.Close()

	err = req.Write(targetConn)
	if err != nil {
		log.Println(err)
		return
	}

	errCh := make(chan error, 2)
	buf1 := make([]byte, 32*1024)
	buf2 := make([]byte, 32*1024)
	go proxy(conn, targetConn, buf1, errCh)
	go proxy(targetConn, conn, buf2, errCh)

	err = <-errCh
	if err != nil && err != io.EOF {
		log.Println(err)
	}
}

func proxy(src, dst net.Conn, buf []byte, errCh chan error) {
	_, err := CopyBuffer(dst, src, buf)
	errCh <- err
}

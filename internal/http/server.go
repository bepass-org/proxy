package http

import (
	"bufio"
	"github.com/bepass-org/proxy/internal/statute"
	"io"
	"net"
	"net/http"
	"strconv"
)

type Server struct {
	// UserConnectHandle gives the user control to handle the TCP CONNECT requests
	UserConnectHandle statute.UserConnectHandler
	// Logger error log
	Logger statute.Logger
}

func NewServer(options ...ServerOption) *Server {
	s := &Server{}
	for _, option := range options {
		option(s)
	}

	if s.Logger == nil {
		s.Logger = statute.DefaultLogger{}
	}

	return s
}

type ServerOption func(*Server)

func WithLogger(logger statute.Logger) ServerOption {
	return func(s *Server) {
		s.Logger = logger
	}
}

func WithConnectHandle(handler statute.UserConnectHandler) ServerOption {
	return func(s *Server) {
		s.UserConnectHandle = handler
	}
}

func (s *Server) ServeConn(conn net.Conn) error {
	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return err
	}

	return s.handleHTTP(conn, req, req.Method == http.MethodConnect)
}

func (s *Server) handleHTTP(conn net.Conn, req *http.Request, isConnectMethod bool) error {
	if s.UserConnectHandle == nil {
		return s.embedHandleHTTP(conn, req, isConnectMethod)
	}

	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return err
	}

	isConnect := req.Method == http.MethodConnect
	targetAddr := req.URL.Host

	if isConnect {
		_, err = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		if err != nil {
			return err
		}
	} else {
		cConn := &customConn{
			Conn: conn,
			req:  req,
		}
		conn = cConn
	}

	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		host = targetAddr
		if req.URL.Scheme == "https" || isConnect {
			portStr = "443"
		} else {
			portStr = "80"
		}
		targetAddr = net.JoinHostPort(host, portStr)
	}

	portInt, err := strconv.Atoi(portStr)
	if err != nil {
		return err // Handle the error if the port string is not a valid integer.
	}
	port := int32(portInt)

	proxyReq := &statute.ProxyRequest{
		Conn:        conn,
		Reader:      io.Reader(conn),
		Writer:      io.Writer(conn),
		Network:     "tcp",
		Destination: targetAddr,
		DestHost:    host,
		DestPort:    port,
	}

	return s.UserConnectHandle(proxyReq)
}

func (s *Server) embedHandleHTTP(conn net.Conn, req *http.Request, isConnectMethod bool) error {
	defer func() {
		_ = conn.Close()
	}()
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
	_, err := copyBuffer(dst, src, make([]byte, 32*1024))
	errCh <- err
}

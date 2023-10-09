package main

import (
	"bufio"
	"errors"
	"github.com/bepass-org/proxy/internal/http"
	"github.com/bepass-org/proxy/internal/socks4"
	"github.com/bepass-org/proxy/internal/socks5"
	"github.com/bepass-org/proxy/internal/statute"
	"io"
	"log"
	"net"
)

var (
	socks5Server *socks5.Server
	socks4Server *socks4.Server
	httpServer   *http.Server
)

func init() {
	socks5Server = socks5.NewServer(
		socks5.WithConnectHandle(generalHandler),
		socks5.WithAssociateHandle(generalHandler),
	)
	socks4Server = socks4.NewServer(
		socks4.WithConnectHandle(generalHandler),
	)
	httpServer = http.NewServer(
		http.WithConnectHandle(generalHandler),
	)
}

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:1080")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Listening on port 1080")
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go handleConnection(conn)
	}
}

// SwitchConn wraps a net.Conn and a bufio.Reader
type SwitchConn struct {
	net.Conn
	reader *bufio.Reader
}

// NewSwitchConn creates a new SwitchConn
func NewSwitchConn(conn net.Conn) *SwitchConn {
	return &SwitchConn{
		Conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

// Read reads data into p, first from the bufio.Reader, then from the net.Conn
func (c *SwitchConn) Read(p []byte) (n int, err error) {
	return c.reader.Read(p)
}

func handleConnection(conn net.Conn) {
	// Create a SwitchConn
	switchConn := NewSwitchConn(conn)

	// Read one byte to determine the protocol
	buf := make([]byte, 1)
	_, err := switchConn.Read(buf)
	if err != nil {
		log.Println(err)
	}

	// Unread the byte so it's available for the next read
	err = switchConn.reader.UnreadByte()
	if err != nil {
		log.Println(err)
	}

	switch {
	case buf[0] == 5:
		err = socks5Server.ServeConn(switchConn)
	case buf[0] == 4:
		err = socks4Server.ServeConn(switchConn)
	default:
		err = httpServer.ServeConn(switchConn)
	}

	if err != nil {
		log.Println(err)
	}
}

func generalHandler(req *statute.ProxyRequest) error {
	if req.Network == "tcp" {
		return tcpHandler(req)
	} else if req.Network == "udp" {
		return udpHandler(req)
	}
	return errors.New("unknown network proxy request")
}

func tcpHandler(req *statute.ProxyRequest) error {
	conn, err := net.Dial(req.Network, req.Destination)
	if err != nil {
		return err
	}
	go func() {
		_, err := io.Copy(conn, req.Conn)
		if err != nil {
			log.Println(err)
		}
	}()
	_, err = io.Copy(req.Conn, conn)
	return err
}

func udpHandler(req *statute.ProxyRequest) error {
	conn, err := net.Dial(req.Network, req.Destination)
	if err != nil {
		return err
	}
	go func() {
		_, err := copyBuffer(req.Conn, conn)
		if err != nil {
			log.Println(err)
		}
	}()
	_, err = copyBuffer(conn, req.Conn)
	return err
}

var ErrShortWrite = errors.New("short write")
var errInvalidWrite = errors.New("invalid write result")
var EOF = errors.New("EOF")

func copyBuffer(dst io.Writer, src io.Reader) (written int64, err error) {
	buf := make([]byte, 32*1024)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errInvalidWrite
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

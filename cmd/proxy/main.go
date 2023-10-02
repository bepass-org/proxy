package main

import (
	"bufio"
	"github.com/bepass-org/proxy/internal/http"
	"github.com/bepass-org/proxy/internal/socks4"
	"github.com/bepass-org/proxy/internal/socks5"
	"log"
	"net"
)

var (
	socks5Server *socks5.Server
	socks4Server *socks4.Server
	httpServer   *http.Server
)

func init() {
	socks5Server = socks5.NewServer()
	socks4Server = socks4.NewServer()
	httpServer = http.NewServer()
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
	defer func() {
		err := conn.Close()
		if err != nil {
			log.Println(err)
		}
	}()

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
		handleSocks5Connection(switchConn)
	case buf[0] == 4:
		handleSocks4Connection(switchConn)
	default:
		handleHttpConnection(switchConn)
	}
}

func handleSocks5Connection(conn net.Conn) {
	err := socks5Server.ServeConn(conn)
	if err != nil {
		log.Println(err)
	}
}

func handleSocks4Connection(conn net.Conn) {
	err := socks4Server.ServeConn(conn)
	if err != nil {
		log.Println(err)
	}
}

func handleHttpConnection(conn net.Conn) {
	err := httpServer.ServeConn(conn)
	if err != nil {
		log.Println(err)
	}
}

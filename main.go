package main

import (
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
	ln, err := net.Listen("tcp", ":1080")
	if err != nil {
		log.Fatal(err)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 3)
	_, err := conn.Read(buf)
	if err != nil {
		log.Println(err)
		return
	}

	switch {
	case buf[0] == 5:
		handleSocks5Connection(conn)
	case buf[0] == 4:
		handleSocks4Connection(conn)
	case buf[0] == 'C' && buf[1] == 'O' && buf[2] == 'N':
		handleHttpConnection(conn)
	default:
		log.Println("Unknown protocol")
	}
}

func handleSocks5Connection(conn net.Conn) {
	socks5Server.ServeConn(conn)
}

func handleSocks4Connection(conn net.Conn) {
	socks4Server.ServeConn(conn)
}

func handleHttpConnection(conn net.Conn) {
	httpServer.ServeConn(conn)
}

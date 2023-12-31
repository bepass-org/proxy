package mixed

import (
	"bufio"
	"context"
	"github.com/bepass-org/proxy/pkg/http"
	"github.com/bepass-org/proxy/pkg/socks4"
	"github.com/bepass-org/proxy/pkg/socks5"
	"github.com/bepass-org/proxy/pkg/statute"
	"net"
)

type userHandler func(request *statute.ProxyRequest) error

type Proxy struct {
	// bind is the address to listen on
	bind string
	// socks5Proxy is a socks5 server with tcp and udp support
	socks5Proxy *socks5.Server
	// socks4Proxy is a socks4 server with tcp support
	socks4Proxy *socks4.Server
	// httpProxy is a http proxy server with http and http-connect support
	httpProxy *http.Server
	// userConnectHandle is a user handler for tcp and udp requests(its general handler)
	userHandler userHandler
	// if user doesnt set userHandler, it can specify userTCPHandler for manual handling of tcp requests
	userTCPHandler userHandler
	// if user doesnt set userHandler, it can specify userUDPHandler for manual handling of udp requests
	userUDPHandler userHandler
	// overwrite dial functions of http, socks4, socks5
	userDialFunc statute.ProxyDialFunc
	// logger error log
	logger statute.Logger
	// ctx is default context
	ctx context.Context
}

func NewProxy(options ...Option) *Proxy {
	p := &Proxy{
		bind:         statute.DefaultBindAddress,
		socks5Proxy:  socks5.NewServer(),
		socks4Proxy:  socks4.NewServer(),
		httpProxy:    http.NewServer(),
		userDialFunc: statute.DefaultProxyDial(),
		logger:       statute.DefaultLogger{},
		ctx:          statute.DefaultContext(),
	}

	for _, option := range options {
		option(p)
	}

	return p
}

type Option func(*Proxy)

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

func (p *Proxy) ListenAndServe() error {
	p.logger.Debug("Serving on " + p.bind + " ...")
	// Create a new listener
	ln, err := net.Listen("tcp", p.bind)
	if err != nil {
		p.logger.Error("Error listening on " + p.bind + ", " + err.Error())
		return err // Return error if binding was unsuccessful
	}

	// ensure listener will be closed
	defer func() {
		_ = ln.Close()
	}()

	// Create a cancelable context based on p.Context
	ctx, cancel := context.WithCancel(p.ctx)
	defer cancel() // Ensure resources are cleaned up

	// Start to accept connections and serve them
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			conn, err := ln.Accept()
			if err != nil {
				p.logger.Error(err)
				continue
			}

			// Start a new goroutine to handle each connection
			// This way, the server can handle multiple connections concurrently
			go func() {
				err := p.handleConnection(conn)
				if err != nil {
					p.logger.Error(err) // Log errors from ServeConn
				}
			}()
		}
	}
}

func (p *Proxy) handleConnection(conn net.Conn) error {
	// Create a SwitchConn
	switchConn := NewSwitchConn(conn)

	// Read one byte to determine the protocol
	buf := make([]byte, 1)
	_, err := switchConn.Read(buf)
	if err != nil {
		return err
	}

	// Unread the byte so it's available for the next read
	err = switchConn.reader.UnreadByte()
	if err != nil {
		return err
	}

	switch {
	case buf[0] == 5:
		err = p.socks5Proxy.ServeConn(switchConn)
	case buf[0] == 4:
		err = p.socks4Proxy.ServeConn(switchConn)
	default:
		err = p.httpProxy.ServeConn(switchConn)
	}

	return err
}

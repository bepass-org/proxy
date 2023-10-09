package statute

import (
	"context"
	"fmt"
	"io"
	"net"
)

type Logger interface {
	Debug(v ...interface{})
	Error(v ...interface{})
}

type DefaultLogger struct{}

func (l DefaultLogger) Debug(v ...interface{}) {
	fmt.Println(v...)
}

func (l DefaultLogger) Error(v ...interface{}) {
	fmt.Println(v...)
}

type ProxyRequest struct {
	Conn        net.Conn
	Reader      io.Reader
	Writer      io.Writer
	Network     string
	Destination string
	DestHost    string
	DestPort    int32
}

// UserConnectHandler is used for socks5, socks4 and http
type UserConnectHandler func(request *ProxyRequest) error

// UserAssociateHandler is used for socks5
type UserAssociateHandler func(request *ProxyRequest) error

// ProxyDialFunc is used for socks5, socks4 and http
type ProxyDialFunc func(ctx context.Context, network string, address string) (net.Conn, error)

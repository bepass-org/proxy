package main

import (
	"fmt"
	"github.com/bepass-org/proxy/pkg/mixed"
	"github.com/bepass-org/proxy/pkg/statute"
	"io"
	"log"
	"net"
)

func main() {
	proxy := mixed.NewProxy(
		mixed.WithBinAddress("127.0.0.1:1080"),
		mixed.WithUserHandler(generalHandler),
	)
	_ = proxy.ListenAndServe()
}

func generalHandler(req *statute.ProxyRequest) error {
	fmt.Println("handling request to", req.Destination)
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

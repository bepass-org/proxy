package main

import "github.com/bepass-org/proxy/pkg/mixed"

func main() {
	myLogger, _ := NewLogger("proxy.log")

	proxy := mixed.NewProxy(
		mixed.WithBindAddress("127.0.0.1:1080"),
		mixed.WithLogger(myLogger),
	)
	_ = proxy.ListenAndServe()
}

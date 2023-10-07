package socks5

import (
	"bytes"
	"context"
	"fmt"
	"github.com/bepass-org/proxy/internal/statute"
	"io"
	"net"
)

// Server is accepting connections and handling the details of the SOCKS5 protocol
type Server struct {
	// ProxyDial specifies the optional proxyDial function for
	// establishing the transport connection.
	ProxyDial func(ctx context.Context, network string, address string) (net.Conn, error)
	// ProxyListen specifies the optional proxyListen function for
	// establishing the transport connection.
	ProxyListen func(context.Context, string, string) (net.Listener, error)
	// ProxyListenPacket specifies the optional proxyListenPacket function for
	// establishing the transport connection.
	ProxyListenPacket func(ctx context.Context, network string, address string) (net.PacketConn, error)
	// PacketForwardAddress specifies the packet forwarding address
	PacketForwardAddress func(ctx context.Context, destinationAddr string, packet net.PacketConn, conn net.Conn) (net.IP, int, error)
	// UserConnectHandle gives the user control to handle the TCP CONNECT requests
	UserConnectHandle statute.UserConnectHandler
	// UserAssociateHandle gives the user control to handle the UDP ASSOCIATE requests
	UserAssociateHandle statute.UserAssociateHandler
	// Logger error log
	Logger Logger
	// Context is default context
	Context context.Context
	// BytesPool getting and returning temporary bytes for use by io.CopyBuffer
	BytesPool BytesPool
}

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

func NewServer(options ...ServerOption) *Server {
	s := &Server{}
	for _, option := range options {
		option(s)
	}

	if s.Logger == nil {
		s.Logger = DefaultLogger{}
	}

	return s
}

type ServerOption func(*Server)

func WithLogger(logger Logger) ServerOption {
	return func(s *Server) {
		s.Logger = logger
	}
}

func (s *Server) ServeConn(conn net.Conn) error {
	version, err := readByte(conn)
	if err != nil {
		return err
	}
	if version != socks5Version {
		return fmt.Errorf("unsupported SOCKS version: %d", version)
	}

	req := &request{
		Version: socks5Version,
		Conn:    conn,
	}

	methods, err := readBytes(conn)
	if err != nil {
		return err
	}

	if bytes.IndexByte(methods, byte(noAuth)) != -1 {
		_, err := conn.Write([]byte{socks5Version, byte(noAuth)})
		if err != nil {
			return err
		}
	} else {
		_, err := conn.Write([]byte{socks5Version, byte(noAcceptable)})
		if err != nil {
			return err
		}
		return errNoSupportedAuth
	}

	var header [3]byte
	_, err = io.ReadFull(conn, header[:])
	if err != nil {
		return err
	}

	if header[0] != socks5Version {
		return fmt.Errorf("unsupported Command version: %d", header[0])
	}

	req.Command = Command(header[1])

	dest, err := readAddr(conn)
	if err != nil {
		if err == errUnrecognizedAddrType {
			err := sendReply(conn, addrTypeNotSupported, nil)
			if err != nil {
				return err
			}
		}
		return err
	}
	req.DestinationAddr = dest
	err = s.handle(req)
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) handle(req *request) error {
	switch req.Command {
	case ConnectCommand:
		return s.handleConnect(req)
	case AssociateCommand:
		return s.handleAssociate(req)
	default:
		if err := sendReply(req.Conn, commandNotSupported, nil); err != nil {
			return err
		}
		return fmt.Errorf("unsupported Command: %v", req.Command)
	}
}

func (s *Server) handleConnect(req *request) error {
	if s.UserConnectHandle == nil {
		return s.embedHandleConnect(req)
	}

	if err := sendReply(req.Conn, successReply, nil); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}
	host := req.DestinationAddr.IP.String()
	if req.DestinationAddr.Name != "" {
		host = req.DestinationAddr.Name
	}

	proxyReq := &statute.ProxyRequest{
		Conn:        req.Conn,
		Reader:      io.Reader(req.Conn),
		Writer:      io.Writer(req.Conn),
		Network:     "tcp",
		Destination: req.DestinationAddr.String(),
		DestHost:    host,
		DestPort:    int32(req.DestinationAddr.Port),
	}

	return s.UserConnectHandle(proxyReq)
}

func (s *Server) embedHandleConnect(req *request) error {
	defer func() {
		_ = req.Conn.Close()
	}()

	ctx := s.context()
	target, err := s.proxyDial(ctx, "tcp", req.DestinationAddr.Address())
	if err != nil {
		if err := sendReply(req.Conn, errToReply(err), nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return fmt.Errorf("connect to %v failed: %w", req.DestinationAddr, err)
	}
	defer func() {
		_ = target.Close()
	}()

	localAddr := target.LocalAddr()
	local, ok := localAddr.(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("connect to %v failed: local address is %s://%s", req.DestinationAddr, localAddr.Network(), localAddr.String())
	}
	bind := address{IP: local.IP, Port: local.Port}
	if err := sendReply(req.Conn, successReply, &bind); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}

	var buf1, buf2 []byte
	if s.BytesPool != nil {
		buf1 = s.BytesPool.Get()
		buf2 = s.BytesPool.Get()
		defer func() {
			s.BytesPool.Put(buf1)
			s.BytesPool.Put(buf2)
		}()
	} else {
		buf1 = make([]byte, 32*1024)
		buf2 = make([]byte, 32*1024)
	}
	return tunnel(ctx, target, req.Conn, buf1, buf2)
}

func (s *Server) handleAssociate(req *request) error {
	ctx := s.context()
	destinationAddr := req.DestinationAddr.String()
	udpConn, err := s.proxyListenPacket(ctx, "udp", destinationAddr)
	if err != nil {
		if err := sendReply(req.Conn, errToReply(err), nil); err != nil {
			return fmt.Errorf("failed to send reply: %v", err)
		}
		return fmt.Errorf("connect to %v failed: %w", req.DestinationAddr, err)
	}
	defer func() {
		_ = udpConn.Close()
	}()

	replyPacketForwardAddress := defaultReplyPacketForwardAddress
	if s.PacketForwardAddress != nil {
		replyPacketForwardAddress = s.PacketForwardAddress
	}
	ip, port, err := replyPacketForwardAddress(ctx, destinationAddr, udpConn, req.Conn)
	if err != nil {
		return err
	}
	bind := address{IP: ip, Port: port}
	if err := sendReply(req.Conn, successReply, &bind); err != nil {
		return fmt.Errorf("failed to send reply: %v", err)
	}

	if s.UserAssociateHandle == nil {
		return s.embedHandleAssociate(req, udpConn)
	}

	cConn := &udpCustomConn{
		PacketConn:   udpConn,
		assocTCPConn: req.Conn,
		frc:          make(chan bool),
	}

	cConn.asyncReadPackets()

	// wait for first packet so that target sender and receiver get known
	<-cConn.frc

	proxyReq := &statute.ProxyRequest{
		Conn:        cConn,
		Reader:      cConn,
		Writer:      cConn,
		Network:     "udp",
		Destination: cConn.targetAddr.String(),
		DestHost:    cConn.targetAddr.(*net.UDPAddr).IP.String(),
		DestPort:    int32(cConn.targetAddr.(*net.UDPAddr).Port),
	}

	return s.UserAssociateHandle(proxyReq)
}

func (s *Server) embedHandleAssociate(req *request, udpConn net.PacketConn) error {
	go func() {
		var buf [1]byte
		for {
			_, err := req.Conn.Read(buf[:])
			if err != nil {
				_ = udpConn.Close()
				break
			}
		}
	}()

	var (
		sourceAddr  net.Addr
		wantSource  string
		targetAddr  net.Addr
		wantTarget  string
		replyPrefix []byte
		buf         [maxUdpPacket]byte
	)

	for {
		n, addr, err := udpConn.ReadFrom(buf[:])
		if err != nil {
			return err
		}

		if sourceAddr == nil {
			sourceAddr = addr
			wantSource = sourceAddr.String()
		}

		gotAddr := addr.String()
		if wantSource == gotAddr {
			if n < 3 {
				continue
			}
			reader := bytes.NewBuffer(buf[3:n])
			addr, err := readAddr(reader)
			if err != nil {
				s.Logger.Debug(err)
				continue
			}
			if targetAddr == nil {
				targetAddr = &net.UDPAddr{
					IP:   addr.IP,
					Port: addr.Port,
				}
				wantTarget = targetAddr.String()
			}
			if addr.String() != wantTarget {
				s.Logger.Debug(fmt.Errorf("ignore non-target addresses %s", addr))
				continue
			}
			_, err = udpConn.WriteTo(reader.Bytes(), targetAddr)
			if err != nil {
				return err
			}
		} else if targetAddr != nil && wantTarget == gotAddr {
			if replyPrefix == nil {
				b := bytes.NewBuffer(make([]byte, 3, 16))
				err = writeAddrWithStr(b, wantTarget)
				if err != nil {
					return err
				}
				replyPrefix = b.Bytes()
			}
			copy(buf[len(replyPrefix):len(replyPrefix)+n], buf[:n])
			copy(buf[:len(replyPrefix)], replyPrefix)
			_, err = udpConn.WriteTo(buf[:len(replyPrefix)+n], sourceAddr)
			if err != nil {
				return err
			}
		}
	}
}

func (s *Server) proxyDial(ctx context.Context, network, address string) (net.Conn, error) {
	proxyDial := s.ProxyDial
	if proxyDial == nil {
		var dialer net.Dialer
		proxyDial = dialer.DialContext
	}
	return proxyDial(ctx, network, address)
}

func (s *Server) proxyListenPacket(ctx context.Context, network, address string) (net.PacketConn, error) {
	proxyListenPacket := s.ProxyListenPacket
	if proxyListenPacket == nil {
		var listener net.ListenConfig
		proxyListenPacket = listener.ListenPacket
	}
	return proxyListenPacket(ctx, network, address)
}

func (s *Server) context() context.Context {
	if s.Context == nil {
		return context.Background()
	}
	return s.Context
}

func sendReply(w io.Writer, resp reply, addr *address) error {
	_, err := w.Write([]byte{socks5Version, byte(resp), 0})
	if err != nil {
		return err
	}
	err = writeAddr(w, addr)
	return err
}

type request struct {
	Version         uint8
	Command         Command
	DestinationAddr *address
	Username        string
	Password        string
	Conn            net.Conn
}

func defaultReplyPacketForwardAddress(_ context.Context, destinationAddr string, packet net.PacketConn, conn net.Conn) (net.IP, int, error) {
	udpLocal := packet.LocalAddr()
	udpLocalAddr, ok := udpLocal.(*net.UDPAddr)
	if !ok {
		return nil, 0, fmt.Errorf("connect to %v failed: local address is %s://%s", destinationAddr, udpLocal.Network(), udpLocal.String())
	}

	tcpLocal := conn.LocalAddr()
	tcpLocalAddr, ok := tcpLocal.(*net.TCPAddr)
	if !ok {
		return nil, 0, fmt.Errorf("connect to %v failed: local address is %s://%s", destinationAddr, tcpLocal.Network(), tcpLocal.String())
	}
	return tcpLocalAddr.IP, udpLocalAddr.Port, nil
}

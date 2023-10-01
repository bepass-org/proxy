package socks4

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
)

var (
	errUserAuthFailed = errors.New("user authentication failed")
)

var (
	isSocks4a = []byte{0, 0, 0, 1}
	isNone    = []byte{0, 0, 0, 0}
)

const (
	socks4Version = 0x04
)

const (
	ConnectCommand Command = 0x01
	BindCommand    Command = 0x02
)

// Command is a SOCKS Command.
type Command byte

func (cmd Command) String() string {
	switch cmd {
	case ConnectCommand:
		return "socks connect"
	case BindCommand:
		return "socks bind"
	default:
		return "socks " + strconv.Itoa(int(cmd))
	}
}

const (
	grantedReply     reply = 0x5a
	rejectedReply    reply = 0x5b
	noIdentdReply    reply = 0x5c
	invalidUserReply reply = 0x5d
)

// reply is a SOCKS Command reply code.
type reply byte

func (code reply) String() string {
	switch code {
	case grantedReply:
		return "request granted"
	case rejectedReply:
		return "request rejected or failed"
	case noIdentdReply:
		return "request rejected becasue SOCKS server cannot connect to identd on the client"
	case invalidUserReply:
		return "request rejected because the client program and identd report different user-ids"
	default:
		return "unknown code: " + strconv.Itoa(int(code))
	}
}

// address is a SOCKS-specific address.
// Either Name or IP is used exclusively.
type address struct {
	Name string // fully-qualified domain name
	IP   net.IP
	Port int
}

func (a *address) Network() string { return "socks4" }

func (a *address) String() string {
	if a == nil {
		return "<nil>"
	}
	return a.Address()
}

// Address returns a string suitable to dial; prefer returning IP-based
// address, fallback to Name
func (a address) Address() string {
	port := strconv.Itoa(a.Port)
	if a.Name != "" {
		return net.JoinHostPort(a.Name, port)
	}
	return net.JoinHostPort(a.IP.String(), port)
}

type AddrAnfUser struct {
	address
	Username string
}

func readBytes(r io.Reader) ([]byte, error) {
	buf := []byte{}
	var data [1]byte
	for {
		_, err := r.Read(data[:])
		if err != nil {
			return nil, err
		}
		if data[0] == 0 {
			return buf, nil
		}
		buf = append(buf, data[0])
	}
}

func writeBytes(w io.Writer, b []byte) error {
	if len(b) != 0 {
		_, err := w.Write(b)
		if err != nil {
			return err
		}
	}
	_, err := w.Write([]byte{0})
	return err
}

func readByte(r io.Reader) (byte, error) {
	var buf [1]byte
	_, err := r.Read(buf[:])
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

func readAddrAndUser(r io.Reader) (*AddrAnfUser, error) {
	address := &AddrAnfUser{}
	var port [2]byte
	if _, err := io.ReadFull(r, port[:]); err != nil {
		return nil, err
	}
	address.Port = int(binary.BigEndian.Uint16(port[:]))
	ip := make(net.IP, net.IPv4len)
	if _, err := io.ReadFull(r, ip); err != nil {
		return nil, err
	}
	socks4a := bytes.Equal(ip, isSocks4a)

	username, err := readBytes(r)
	if err != nil {
		return nil, err
	}
	address.Username = string(username)
	if socks4a {
		hostname, err := readBytes(r)
		if err != nil {
			return nil, err
		}
		address.Name = string(hostname)
	} else {
		address.IP = ip
	}
	return address, nil
}

func writeAddrAndUser(w io.Writer, addr *AddrAnfUser) error {
	var port [2]byte
	binary.BigEndian.PutUint16(port[:], uint16(addr.Port))
	_, err := w.Write(port[:])
	if err != nil {
		return err
	}

	socks4a := false
	ip := addr.IP.To4()
	if ip == nil {
		if addr.Name != "" {
			socks4a = true
			_, err = w.Write(isSocks4a)
		} else {
			_, err = w.Write(isNone)
		}
	} else {
		_, err = w.Write(ip)
	}
	if err != nil {
		return err
	}

	err = writeBytes(w, []byte(addr.Username))
	if err != nil {
		return err
	}

	if socks4a {
		err = writeBytes(w, []byte(addr.Name))
		if err != nil {
			return err
		}
	}
	return nil
}

func readAddr(r io.Reader) (*address, error) {
	address := &address{}
	var port [2]byte
	if _, err := io.ReadFull(r, port[:]); err != nil {
		return nil, err
	}
	address.Port = int(binary.BigEndian.Uint16(port[:]))
	addr := make(net.IP, net.IPv4len)
	if _, err := io.ReadFull(r, addr); err != nil {
		return nil, err
	}
	address.IP = addr
	return address, nil
}

func writeAddr(w io.Writer, addr *address) error {
	var ip net.IP
	var port uint16
	if addr != nil {
		ip = addr.IP.To4()
		port = uint16(addr.Port)
	}
	var p [2]byte
	binary.BigEndian.PutUint16(p[:], port)
	_, err := w.Write(p[:])
	if err != nil {
		return err
	}

	if ip == nil {
		_, err = w.Write(isNone)
	} else {
		_, err = w.Write(ip)
	}
	return err
}

func writeAddrAndUserWithStr(w io.Writer, addr, username string) error {
	host, port, err := splitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip != nil {
		return writeAddrAndUser(w, &AddrAnfUser{address: address{IP: ip, Port: port}, Username: username})
	}
	return writeAddrAndUser(w, &AddrAnfUser{address: address{Name: host, Port: port}, Username: username})
}

func splitHostPort(address string) (string, int, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	portnum, err := strconv.Atoi(port)
	if err != nil {
		return "", 0, err
	}
	if 1 > portnum || portnum > 0xffff {
		return "", 0, errors.New("port number out of range " + port)
	}
	return host, portnum, nil
}

// isClosedConnError reports whether err is an error from use of a closed
// network connection.
func isClosedConnError(err error) bool {
	if err == nil {
		return false
	}

	str := err.Error()
	if strings.Contains(str, "use of closed network connection") {
		return true
	}

	if runtime.GOOS == "windows" {
		if oe, ok := err.(*net.OpError); ok && oe.Op == "read" {
			if se, ok := oe.Err.(*os.SyscallError); ok && se.Syscall == "wsarecv" {
				const WSAECONNABORTED = 10053
				const WSAECONNRESET = 10054
				if n := errno(se.Err); n == WSAECONNRESET || n == WSAECONNABORTED {
					return true
				}
			}
		}
	}
	return false
}

func errno(v error) uintptr {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Uintptr {
		return uintptr(rv.Uint())
	}
	return 0
}

// tunnel create tunnels for two io.ReadWriteCloser
func tunnel(ctx context.Context, c1, c2 io.ReadWriteCloser, buf1, buf2 []byte) error {
	ctx, cancel := context.WithCancel(ctx)
	var errs tunnelErr
	go func() {
		_, errs[0] = io.CopyBuffer(c1, c2, buf1)
		cancel()
	}()
	go func() {
		_, errs[1] = io.CopyBuffer(c2, c1, buf2)
		cancel()
	}()
	<-ctx.Done()
	errs[2] = c1.Close()
	errs[3] = c2.Close()
	errs[4] = ctx.Err()
	if errs[4] == context.Canceled {
		errs[4] = nil
	}
	return errs.FirstError()
}

type tunnelErr [5]error

func (t tunnelErr) FirstError() error {
	for _, err := range t {
		if err != nil {
			return err
		}
	}
	return nil
}

// BytesPool is an interface for getting and returning temporary
// bytes for use by io.CopyBuffer.
type BytesPool interface {
	Get() []byte
	Put([]byte)
}

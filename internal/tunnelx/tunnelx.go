package tunnelx

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"golang.org/x/crypto/ssh"
)

type Kind int

const (
	Local Kind = iota
	Remote
	Dynamic
)

type Forward struct {
	Kind   Kind
	Listen string
	Target string

	ln     net.Listener
	once   sync.Once
	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	rx, tx atomic.Int64
}

func StartLocal(client *ssh.Client, listen, target string) (*Forward, error) {
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, err
	}
	f := newForward(Local, listen, target, ln)
	go f.acceptLocal(client, target)
	return f, nil
}

func StartRemote(client *ssh.Client, listen, target string) (*Forward, error) {
	ln, err := client.Listen("tcp", listen)
	if err != nil {
		return nil, err
	}
	f := newForward(Remote, listen, target, ln)
	go f.acceptRemote(target)
	return f, nil
}

func StartSOCKS5(client *ssh.Client, listen string) (*Forward, error) {
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, err
	}
	f := newForward(Dynamic, listen, "", ln)
	go f.acceptSOCKS(client)
	return f, nil
}

func newForward(kind Kind, listen, target string, ln net.Listener) *Forward {
	return &Forward{
		Kind:   kind,
		Listen: listen,
		Target: target,
		ln:     ln,
		conns:  make(map[net.Conn]struct{}),
	}
}

func (f *Forward) Addr() string {
	if f.ln != nil {
		return f.ln.Addr().String()
	}
	return f.Listen
}

func (f *Forward) Bytes() (rx, tx int64) {
	return f.rx.Load(), f.tx.Load()
}

func (f *Forward) Close() error {
	var err error
	f.once.Do(func() {
		if f.ln != nil {
			err = f.ln.Close()
		}
		f.mu.Lock()
		conns := f.conns
		f.conns = nil
		f.mu.Unlock()
		for c := range conns {
			c.Close()
		}
	})
	return err
}

func (f *Forward) track(c net.Conn) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conns == nil {
		return false
	}
	f.conns[c] = struct{}{}
	return true
}

func (f *Forward) untrack(c net.Conn) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conns != nil {
		delete(f.conns, c)
	}
}

func (f *Forward) proxy(clientConn, remoteConn net.Conn, clientReader io.Reader) {
	defer f.untrack(clientConn)
	defer f.untrack(remoteConn)
	defer clientConn.Close()
	defer remoteConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(remoteConn, clientReader)
		f.tx.Add(n)
		closeWrite(remoteConn)
	}()
	go func() {
		defer wg.Done()
		n, _ := io.Copy(clientConn, remoteConn)
		f.rx.Add(n)
		closeWrite(clientConn)
	}()
	wg.Wait()
}

func (f *Forward) acceptLocal(client *ssh.Client, target string) {
	for {
		local, err := f.ln.Accept()
		if err != nil {
			return
		}
		if !f.track(local) {
			local.Close()
			return
		}
		go func() {
			remote, err := client.Dial("tcp", target)
			if err != nil {
				f.untrack(local)
				local.Close()
				return
			}
			if !f.track(remote) {
				f.untrack(local)
				local.Close()
				remote.Close()
				return
			}
			f.proxy(local, remote, local)
		}()
	}
}

func (f *Forward) acceptRemote(target string) {
	for {
		remote, err := f.ln.Accept()
		if err != nil {
			return
		}
		if !f.track(remote) {
			remote.Close()
			return
		}
		go func() {
			local, err := net.Dial("tcp", target)
			if err != nil {
				f.untrack(remote)
				remote.Close()
				return
			}
			if !f.track(local) {
				f.untrack(remote)
				remote.Close()
				local.Close()
				return
			}
			f.proxy(remote, local, remote)
		}()
	}
}

func (f *Forward) acceptSOCKS(client *ssh.Client) {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handleSOCKS(client, conn)
	}
}

func (f *Forward) handleSOCKS(client *ssh.Client, conn net.Conn) {
	defer conn.Close()

	br := bufio.NewReader(conn)
	ver, err := br.ReadByte()
	if err != nil || ver != 5 {
		return
	}
	n, err := br.ReadByte()
	if err != nil {
		return
	}
	methods := make([]byte, int(n))
	if _, err := io.ReadFull(br, methods); err != nil {
		return
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return
	}

	head := make([]byte, 4)
	if _, err := io.ReadFull(br, head); err != nil {
		return
	}
	if head[0] != 5 || head[1] != 1 {
		socksReply(conn, 7) // command not supported
		return
	}

	host, port, err := readSocksAddr(br, head[3])
	if err != nil {
		socksReply(conn, 8) // address type not supported
		return
	}

	remote, err := client.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		socksReply(conn, 5) // connection refused
		return
	}
	if !f.track(conn) {
		remote.Close()
		return
	}
	if !f.track(remote) {
		f.untrack(conn)
		remote.Close()
		return
	}

	socksReply(conn, 0)
	f.proxy(conn, remote, br)
}

func readSocksAddr(r *bufio.Reader, atyp byte) (string, int, error) {
	var host string

	switch atyp {
	case 1:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(r, ip); err != nil {
			return "", 0, err
		}
		host = net.IP(ip).String()
	case 3:
		n, err := r.ReadByte()
		if err != nil {
			return "", 0, err
		}
		name := make([]byte, int(n))
		if _, err := io.ReadFull(r, name); err != nil {
			return "", 0, err
		}
		host = string(name)
	case 4:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(r, ip); err != nil {
			return "", 0, err
		}
		host = net.IP(ip).String()
	default:
		return "", 0, fmt.Errorf("unsupported address type %d", atyp)
	}

	var pb [2]byte
	if _, err := io.ReadFull(r, pb[:]); err != nil {
		return "", 0, err
	}
	port := int(binary.BigEndian.Uint16(pb[:]))
	return host, port, nil
}

func socksReply(w io.Writer, rep byte) {
	_, _ = w.Write([]byte{5, rep, 0, 1, 0, 0, 0, 0, 0, 0})
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}

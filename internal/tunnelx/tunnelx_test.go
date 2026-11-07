package tunnelx

import (
	"bufio"
	"bytes"
	"testing"
)

func TestReadSocksAddr(t *testing.T) {
	ipv4 := []byte{127, 0, 0, 1, 0x1f, 0x90}
	host, port, err := readSocksAddr(bufio.NewReader(bytes.NewReader(ipv4)), 1)
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" || port != 8080 {
		t.Fatalf("ipv4 = %s:%d", host, port)
	}

	domain := append([]byte{9}, []byte("localhost")...)
	domain = append(domain, 0x00, 0x50)
	host, port, err = readSocksAddr(bufio.NewReader(bytes.NewReader(domain)), 3)
	if err != nil {
		t.Fatal(err)
	}
	if host != "localhost" || port != 80 {
		t.Fatalf("domain = %s:%d", host, port)
	}

	ipv6 := append(make([]byte, 15), 1)
	ipv6 = append(ipv6, 0x00, 0x16)
	host, port, err = readSocksAddr(bufio.NewReader(bytes.NewReader(ipv6)), 4)
	if err != nil {
		t.Fatal(err)
	}
	if host != "::1" || port != 22 {
		t.Fatalf("ipv6 = %s:%d", host, port)
	}
}

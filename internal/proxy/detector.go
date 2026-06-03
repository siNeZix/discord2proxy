package proxy

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

type ProxyInfo struct {
	Host   string
	Port   int
	Source string
	Alive  bool
}

var portSourceMap = map[int]string{
	2080: "nekobox",
	1080: "v2ray",
}

func detectOnPort(host string, port int) *ProxyInfo {
	info := &ProxyInfo{
		Host:   host,
		Port:   port,
		Source: "unknown",
		Alive:  false,
	}

	if src, ok := portSourceMap[port]; ok {
		info.Source = src
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return info
	}
	defer conn.Close()

	// Bound both write and read so a hung peer can't block indefinitely.
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return info
	}

	// SOCKS5 greeting: version 5, 1 method, "no authentication" (0x00).
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return info
	}

	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return info
	}

	// Valid reply: version 5 and a method other than 0xFF ("no acceptable
	// methods"). 0xFF means the server rejected our auth and isn't usable.
	if buf[0] == 0x05 && buf[1] != 0xFF {
		info.Alive = true
	}

	return info
}

// VerifyProxy actively checks that a specific host:port speaks SOCKS5.
// Returns a populated ProxyInfo on success, or an error if unreachable.
func VerifyProxy(host string, port int) (*ProxyInfo, error) {
	info := detectOnPort(host, port)
	if !info.Alive {
		return nil, fmt.Errorf("прокси %s:%d не отвечает по SOCKS5", host, port)
	}
	return info, nil
}

// DetectBestProxy probes all ports concurrently and returns the first alive
// proxy in the priority order given by ports. Probing in parallel bounds the
// total wait to a single timeout (~3s) instead of N×3s.
func DetectBestProxy(host string, ports []int) (*ProxyInfo, error) {
	results := make([]*ProxyInfo, len(ports))
	var wg sync.WaitGroup
	for i, port := range ports {
		wg.Add(1)
		go func(idx, p int) {
			defer wg.Done()
			results[idx] = detectOnPort(host, p)
		}(i, port)
	}
	wg.Wait()

	for _, info := range results {
		if info != nil && info.Alive {
			return info, nil
		}
	}

	return nil, errors.New("SOCKS5 прокси не найден; убедитесь, что v2ray или nekobox запущен")
}

package proxy

import (
	"errors"
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

	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		return info
	}

	buf := make([]byte, 2)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err = conn.Read(buf)
	if err != nil {
		return info
	}

	if buf[0] == 0x05 {
		info.Alive = true
	}

	return info
}

func DetectProxies(host string, ports []int) ([]ProxyInfo, error) {
	var mu sync.Mutex
	var results []ProxyInfo
	var wg sync.WaitGroup

	for _, port := range ports {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			info := detectOnPort(host, p)
			mu.Lock()
			results = append(results, *info)
			mu.Unlock()
		}(port)
	}
	wg.Wait()

	return results, nil
}

func DetectBestProxy(host string, ports []int) (*ProxyInfo, error) {
	for _, port := range ports {
		info := detectOnPort(host, port)
		if info.Alive {
			return info, nil
		}
	}

	return nil, errors.New("no SOCKS5 proxy found on configured ports; ensure v2ray/nekobox is running")
}

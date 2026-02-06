package main

import (
	"fmt"
	"net"
	"strconv"
)

func resolveWebAddr(host string, port int, autoIncrement bool) (string, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	if port <= 0 {
		port = 8080
	}

	maxTries := 20
	for i := 0; i < maxTries; i++ {
		candidate := port + i
		addr := net.JoinHostPort(host, strconv.Itoa(candidate))
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			_ = ln.Close()
			return addr, nil
		}
		if !autoIncrement {
			return "", fmt.Errorf("port %d unavailable", port)
		}
	}
	return "", fmt.Errorf("no available port found after %d attempts", maxTries)
}

package server

import (
	"fmt"
	"net"
	"sort"
)

type Config struct {
	Address   string
	Directory string
	Name      string
	PIN       string
	MaxFile   int64
	Quota     int64
}

func URLs(address string) []string {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil
	}
	host, _, _ := net.SplitHostPort(address)
	if host != "" && host != "0.0.0.0" && host != "::" {
		return []string{"http://" + net.JoinHostPort(host, port)}
	}
	urls := []string{"http://localhost:" + port}
	addresses, _ := net.InterfaceAddrs()
	for _, address := range addresses {
		network, ok := address.(*net.IPNet)
		if !ok || network.IP.IsLoopback() || network.IP.To4() == nil {
			continue
		}
		urls = append(urls, fmt.Sprintf("http://%s", net.JoinHostPort(network.IP.String(), port)))
	}
	sort.Strings(urls)
	return urls
}

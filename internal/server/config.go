package server

import (
	"fmt"
	"net"
	"sort"
	"strings"
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
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		virtual := false
		for _, prefix := range []string{"docker", "br-", "veth", "virbr", "tailscale", "tun", "wg"} {
			if strings.HasPrefix(iface.Name, prefix) {
				virtual = true
			}
		}
		if virtual {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			network, ok := address.(*net.IPNet)
			if !ok || network.IP.To4() == nil || network.IP.IsLoopback() || !network.IP.IsPrivate() {
				continue
			}
			urls = append(urls, fmt.Sprintf("http://%s", net.JoinHostPort(network.IP.String(), port)))
		}
	}
	sort.Strings(urls)
	return urls
}

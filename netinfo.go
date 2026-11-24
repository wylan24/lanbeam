package main

import (
	"net"
	"sort"
	"strings"
)

// Adapters that a phone on the same Wi-Fi can't reach.
var virtualAdapters = []string{
	"vethernet", "virtualbox", "vboxnet", "vmware", "vmnet", "docker", "veth",
	"br-", "wsl", "hyper-v", "utun", "awdl", "llw", "bridge", "zerotier", "tap",
}

// lanIPs returns the IPv4 addresses a phone is most likely to reach this
// computer on, best first.
func lanIPs() []string {
	primary := outboundIP()
	type cand struct {
		ip    string
		score int
	}
	var cands []cand
	seen := map[string]bool{}

	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(ifc.Name)
		if hasAny(name, virtualAdapters) {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP.To4()
			if ip == nil || !(ip.IsPrivate() || isCGNAT(ip)) || seen[ip.String()] {
				continue
			}
			seen[ip.String()] = true
			cands = append(cands, cand{ip.String(), score(ip, name, primary)})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })

	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.ip
	}
	return out
}

func score(ip net.IP, ifName, primary string) int {
	s := 0
	switch {
	case ip.String() == primary:
		s += 100
	case ip[0] == 172 && ip[1] == 20 && ip[2] == 10: // joined to an iPhone Personal Hotspot
		s += 60
	case ip[0] == 192 && ip[1] == 168 && ip[2] == 137: // Windows Mobile Hotspot host
		s += 50
	}
	if hasAny(ifName, []string{"wi-fi", "wifi", "wlan", "wireless", "en0"}) {
		s += 20
	}
	switch {
	case ip[0] == 192 && ip[1] == 168:
		s += 10
	case ip[0] == 10:
		s += 8
	case ip[0] == 172:
		s += 5
	}
	return s
}

// outboundIP is the address of the interface holding the default route.
// Dialing UDP sends no packets; it only asks the OS to pick a route.
func outboundIP() string {
	c, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}

func isCGNAT(ip net.IP) bool { return ip[0] == 100 && ip[1]&0xc0 == 64 }

func hasAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

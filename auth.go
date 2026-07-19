package main

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// parseTrustedProxies reads a comma-separated list of IP addresses or CIDR
// blocks. A blank spec returns nil: proxy enforcement stays off, which is
// only acceptable on a loopback listen address.
func parseTrustedProxies(spec string) ([]*net.IPNet, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var networks []*net.IPNet
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, network, err := net.ParseCIDR(entry)
			if err != nil {
				return nil, fmt.Errorf("trusted proxy %q is not a valid CIDR block", entry)
			}
			networks = append(networks, network)
			continue
		}
		ip := net.ParseIP(entry)
		if ip == nil {
			return nil, fmt.Errorf("trusted proxy %q is not a valid IP address", entry)
		}
		networks = append(networks, singleIPNet(ip))
	}
	if len(networks) == 0 {
		return nil, fmt.Errorf("trusted proxy list %q contains no addresses", spec)
	}
	return networks, nil
}

func singleIPNet(ip net.IP) *net.IPNet {
	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
}

// isLoopbackAddr reports whether a listen address is reachable only from
// this machine. An empty host binds every interface and is not loopback.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// proxyGate rejects every request that did not arrive from a trusted reverse
// proxy. The proxy is what authenticates users and asserts their identity via
// the authentigate-id header, so a request from anywhere else must not be
// served. With no proxies configured (the loopback default) it is a no-op.
func (a *App) proxyGate(c *gin.Context) {
	if len(a.trustedProxies) == 0 {
		return
	}
	if ip := remoteIP(c.Request.RemoteAddr); ip != nil {
		for _, network := range a.trustedProxies {
			if network.Contains(ip) {
				return
			}
		}
	}
	c.Abort()
	if a.isAPIRequest(c) {
		apiError(c, http.StatusForbidden, "requests must arrive through a trusted reverse proxy")
		return
	}
	a.renderError(c, http.StatusForbidden, "Requests must arrive through a trusted reverse proxy.")
}

func (a *App) isAPIRequest(c *gin.Context) bool {
	path := c.Request.URL.Path
	return path == a.prefix+"api" || strings.HasPrefix(path, a.prefix+"api/")
}

func remoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return net.ParseIP(host)
}

package handlers

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
)

// Shared by site listings/details and deployment responses. Configure once at
// startup; never infer public destinations from request or proxy headers.
type hostingAddress struct{ scheme, port string }

func (h *hostingAddress) SetHostingAddress(scheme, port string) { h.scheme, h.port = scheme, port }

func (h *hostingAddress) deploymentURL(fqdn, target string) (string, error) {
	if h.scheme != "http" && h.scheme != "https" {
		return "", fmt.Errorf("invalid hosting scheme")
	}
	port, err := strconv.Atoi(h.port)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid hosting port")
	}
	if !validSiteFQDN(fqdn) {
		return "", fmt.Errorf("invalid hosting name")
	}
	host := fqdn
	if target == "preview" {
		host = "preview." + fqdn
	} else if target != "live" {
		return "", fmt.Errorf("invalid hosting target")
	}
	if !(h.scheme == "http" && port == 80) && !(h.scheme == "https" && port == 443) {
		host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return (&url.URL{Scheme: h.scheme, Host: host, Path: "/"}).String(), nil
}

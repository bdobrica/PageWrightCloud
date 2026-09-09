package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Shared by site listings/details and deployment responses. Configure once at
// startup; never infer public destinations from request or proxy headers.
type hostingAddress struct{ scheme, port, tlsStatePath string }

var ErrTLSProvisioning = errors.New("HTTPS provisioning is pending; retry shortly")

func (h *hostingAddress) SetTLSStatePath(path string) { h.tlsStatePath = path }

// The host controller publishes this non-secret file through a read-only mount.
// A bounded lease fails closed after controller/proxy failure; private keys never
// enter the gateway. Development without a state path keeps its existing behavior.
func (h *hostingAddress) tlsReady(fqdn string) bool {
	if h.tlsStatePath == "" {
		return true
	}
	f, err := os.Open(h.tlsStatePath)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return false
	}
	var state struct {
		Expires int64           `json:"expires"`
		Hosts   map[string]bool `json:"hosts"`
	}
	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	if decoder.Decode(&state) != nil || decoder.Decode(new(any)) != io.EOF {
		return false
	}
	now := time.Now().Unix()
	return state.Expires > now && state.Expires <= now+900 && state.Hosts[fqdn]
}

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
	if !h.tlsReady(fqdn) {
		return "", ErrTLSProvisioning
	}
	host := fqdn
	if target == "preview" {
		host = strings.Replace(fqdn, ".", ".preview.", 1)
	} else if target != "live" {
		return "", fmt.Errorf("invalid hosting target")
	}
	if !(h.scheme == "http" && port == 80) && !(h.scheme == "https" && port == 443) {
		host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	return (&url.URL{Scheme: h.scheme, Host: host, Path: "/"}).String(), nil
}

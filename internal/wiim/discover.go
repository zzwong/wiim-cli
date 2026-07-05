package wiim

import (
	"net"
	"sort"
	"time"
)

// DiscoveredDevice describes a Linkplay/WiiM device found via SSDP discovery
// and confirmed by a direct call to its HTTP API.
type DiscoveredDevice struct {
	IP       string `json:"ip"`
	Name     string `json:"name"`
	Model    string `json:"model,omitempty"`
	Firmware string `json:"firmware,omitempty"`
}

const ssdpMulticastAddr = "239.255.255.250:1900"

// validateTimeoutSeconds bounds each per-candidate validation HTTP call.
// Discovery routinely turns up non-WiiM UPnP devices that never answer the
// WiiM HTTP API at all, so this is intentionally short and not
// user-configurable; validation runs concurrently across candidates, so a
// noisy LAN doesn't multiply this into a slow scan.
const validateTimeoutSeconds = 2.0

// ssdpSearchFunc performs the network I/O for Discover; swappable in tests so
// command/formatting behavior can be exercised without real UDP traffic.
var ssdpSearchFunc = ssdpSearch

// discoveryDevice is the narrow subset of Client used to validate an SSDP
// candidate and describe it. A package var (like newDevice) so tests can
// fake it instead of making real HTTP calls to arbitrary LAN IPs.
type discoveryDevice interface {
	StatusEx() (map[string]any, error)
	CastInfo() (map[string]any, error)
}

var newDiscoveryClient = func(ip string, timeoutSeconds float64) discoveryDevice {
	return NewClient(ip, timeoutSeconds)
}

// Discover finds Linkplay/WiiM devices on the local network. It multicasts an
// SSDP M-SEARCH request and, for every host that replies, confirms it's
// actually running the WiiM/Linkplay HTTP API (rather than some unrelated
// UPnP device — smart TVs, printers, and routers all answer SSDP too) before
// including it in the result. An empty result with a nil error is the normal
// outcome when nothing is found; it is not treated as a failure.
func Discover(ssdpTimeout time.Duration) ([]DiscoveredDevice, error) {
	ips, err := ssdpSearchFunc(ssdpTimeout)
	if err != nil {
		return nil, err
	}

	type candidate struct {
		device DiscoveredDevice
		ok     bool
	}
	results := make(chan candidate, len(ips))
	for _, ip := range ips {
		go func(ip string) {
			d, ok := validateDiscoveryCandidate(ip)
			results <- candidate{d, ok}
		}(ip)
	}

	found := []DiscoveredDevice{}
	for range ips {
		if r := <-results; r.ok {
			found = append(found, r.device)
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].IP < found[j].IP })
	return found, nil
}

func validateDiscoveryCandidate(ip string) (DiscoveredDevice, bool) {
	client := newDiscoveryClient(ip, validateTimeoutSeconds)
	statusEx, err := client.StatusEx()
	if err != nil {
		return DiscoveredDevice{}, false
	}
	cast, _ := client.CastInfo() // best-effort, same as the status command
	name := firstString(cast, "name", firstString(statusEx, "DeviceName", firstString(statusEx, "ssid", ip)))
	return DiscoveredDevice{
		IP:       ip,
		Name:     name,
		Model:    stringValue(statusEx["project"]),
		Firmware: stringValue(statusEx["firmware"]),
	}, true
}

// ssdpSearch multicasts a single SSDP M-SEARCH request and collects the
// source IP of every UDP response received before timeout elapses. It does
// not join the multicast group or parse response bodies — SSDP replies come
// back as unicast UDP to the requester's ephemeral port, and the response
// source address is all Discover needs; the WiiM API call in
// validateDiscoveryCandidate does the real confirmation. A timeout with zero
// responses is a normal outcome, not an error.
func ssdpSearch(timeout time.Duration) ([]string, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, runtimef("could not open a UDP socket for discovery: %v", err)
	}
	defer conn.Close()

	dst, err := net.ResolveUDPAddr("udp4", ssdpMulticastAddr)
	if err != nil {
		return nil, runtimef("could not resolve the SSDP multicast address: %v", err)
	}
	request := "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: " + ssdpMulticastAddr + "\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 2\r\n" +
		"ST: upnp:rootdevice\r\n\r\n"
	if _, err := conn.WriteToUDP([]byte(request), dst); err != nil {
		return nil, runtimef("could not send the SSDP discovery request: %v", err)
	}

	seen := map[string]bool{}
	var ips []string
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 2048)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if err := conn.SetReadDeadline(time.Now().Add(remaining)); err != nil {
			break
		}
		_, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			break // read deadline hit; no more responses are coming
		}
		ip := addr.IP.String()
		if !seen[ip] {
			seen[ip] = true
			ips = append(ips, ip)
		}
	}
	return ips, nil
}

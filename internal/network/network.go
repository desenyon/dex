package network

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type IPAddress struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
	Network   string `json:"network"`
}

type Port struct {
	Command string `json:"command"`
	PID     int    `json:"pid"`
	User    string `json:"user"`
	Address string `json:"address"`
	State   string `json:"state"`
}

type Interface struct {
	Name         string   `json:"name"`
	Index        int      `json:"index"`
	MTU          int      `json:"mtu"`
	HardwareAddr string   `json:"hardware_addr,omitempty"`
	Flags        string   `json:"flags"`
	Addresses    []string `json:"addresses,omitempty"`
}

type Route struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Flags       string `json:"flags"`
	Interface   string `json:"interface"`
}

type DNSResolver struct {
	Resolver   string `json:"resolver"`
	Nameserver string `json:"nameserver"`
}

type DNSRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type HTTPProbe struct {
	URL           string              `json:"url"`
	Status        string              `json:"status,omitempty"`
	StatusCode    int                 `json:"status_code,omitempty"`
	FinalURL      string              `json:"final_url,omitempty"`
	Method        string              `json:"method"`
	Headers       map[string][]string `json:"headers,omitempty"`
	RedirectChain []string            `json:"redirect_chain,omitempty"`
	Duration      string              `json:"duration"`
}

type CertificateInfo struct {
	Subject      string   `json:"subject"`
	Issuer       string   `json:"issuer"`
	DNSNames     []string `json:"dns_names,omitempty"`
	NotBefore    string   `json:"not_before"`
	NotAfter     string   `json:"not_after"`
	Expired      bool     `json:"expired"`
	ExpiresIn    string   `json:"expires_in"`
	TLSVersion   string   `json:"tls_version"`
	CipherSuite  string   `json:"cipher_suite"`
	ChainLength  int      `json:"chain_length"`
	PEM          string   `json:"pem,omitempty"`
	SerialNumber string   `json:"serial_number"`
}

type LatencyResult struct {
	Target      string   `json:"target"`
	DNS         string   `json:"dns"`
	TCP         string   `json:"tcp,omitempty"`
	HTTPStatus  string   `json:"http_status,omitempty"`
	HTTP        string   `json:"http,omitempty"`
	Total       string   `json:"total"`
	ResolvedIPs []string `json:"resolved_ips,omitempty"`
}

func IPs(includeAll bool) ([]IPAddress, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []IPAddress
	for _, iface := range interfaces {
		if !includeAll && iface.Flags&net.FlagUp == 0 {
			continue
		}
		if !includeAll && iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			ip, network := splitAddress(addr.String())
			parsedIP := net.ParseIP(ip)
			if !includeAll && (strings.HasPrefix(ip, "127.") || parsedIP != nil && (parsedIP.IsLoopback() || parsedIP.IsLinkLocalUnicast())) {
				continue
			}
			out = append(out, IPAddress{Interface: iface.Name, Address: ip, Network: network})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Interface == out[j].Interface {
			return out[i].Address < out[j].Address
		}
		return out[i].Interface < out[j].Interface
	})
	return out, nil
}

func Interfaces(includeAll bool) ([]Interface, error) {
	items, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]Interface, 0, len(items))
	for _, iface := range items {
		if !includeAll && iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		addresses := make([]string, 0, len(addrs))
		for _, addr := range addrs {
			addresses = append(addresses, addr.String())
		}
		out = append(out, Interface{
			Name:         iface.Name,
			Index:        iface.Index,
			MTU:          iface.MTU,
			HardwareAddr: iface.HardwareAddr.String(),
			Flags:        iface.Flags.String(),
			Addresses:    addresses,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func InterfaceByName(name string) (Interface, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return Interface{}, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return Interface{}, err
	}
	addresses := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		addresses = append(addresses, addr.String())
	}
	return Interface{
		Name:         iface.Name,
		Index:        iface.Index,
		MTU:          iface.MTU,
		HardwareAddr: iface.HardwareAddr.String(),
		Flags:        iface.Flags.String(),
		Addresses:    addresses,
	}, nil
}

func MACAddresses() ([]Interface, error) {
	items, err := Interfaces(false)
	if err != nil {
		return nil, err
	}
	out := make([]Interface, 0, len(items))
	for _, item := range items {
		if item.HardwareAddr != "" {
			out = append(out, item)
		}
	}
	return out, nil
}

func Hostname() (string, error) {
	return os.Hostname()
}

func PublicIP(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func ListeningPorts(ctx context.Context) ([]Port, error) {
	cmd := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return ParseLsofPorts(string(output)), nil
}

func ParseLsofPorts(input string) []Port {
	scanner := bufio.NewScanner(strings.NewReader(input))
	var ports []Port
	seen := map[string]bool{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "COMMAND") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		pid, _ := strconv.Atoi(fields[1])
		address := fields[8]
		state := ""
		if len(fields) > 9 {
			state = strings.Trim(fields[9], "()")
		}
		key := fmt.Sprintf("%s/%d/%s/%s", fields[0], pid, address, state)
		if seen[key] {
			continue
		}
		seen[key] = true
		ports = append(ports, Port{
			Command: fields[0],
			PID:     pid,
			User:    fields[2],
			Address: address,
			State:   state,
		})
	}
	return ports
}

func Routes(ctx context.Context) ([]Route, error) {
	cmd := exec.CommandContext(ctx, "netstat", "-rn")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return ParseRoutes(string(out)), nil
}

func ParseRoutes(input string) []Route {
	scanner := bufio.NewScanner(strings.NewReader(input))
	var routes []Route
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || fields[0] == "Destination" || fields[0] == "Routing" || fields[0] == "Internet:" || fields[0] == "Internet6:" {
			continue
		}
		routes = append(routes, Route{
			Destination: fields[0],
			Gateway:     fields[1],
			Flags:       fields[2],
			Interface:   fields[3],
		})
	}
	return routes
}

func Gateway(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "route", "-n", "get", "default")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gateway:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "gateway:")), nil
		}
	}
	return "", fmt.Errorf("gateway not found")
}

func DNSConfig(ctx context.Context) ([]DNSResolver, error) {
	cmd := exec.CommandContext(ctx, "scutil", "--dns")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return ParseDNSConfig(string(out)), nil
}

func ParseDNSConfig(input string) []DNSResolver {
	scanner := bufio.NewScanner(strings.NewReader(input))
	resolver := "default"
	var out []DNSResolver
	seen := map[string]bool{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "resolver #") {
			resolver = line
			continue
		}
		if strings.HasPrefix(line, "nameserver[") {
			_, value, ok := strings.Cut(line, ":")
			if ok {
				nameserver := strings.TrimSpace(value)
				key := resolver + "/" + nameserver
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, DNSResolver{Resolver: resolver, Nameserver: nameserver})
			}
		}
	}
	return out
}

func Proxy(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "scutil", "--proxy")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)) + "\n", nil
}

func VPN(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "scutil", "--nc", "list")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return "", err
	}
	return strings.TrimSpace(string(out)) + "\n", nil
}

func Lookup(ctx context.Context, name string, recordType string, resolver string) ([]DNSRecord, error) {
	resolverClient := net.DefaultResolver
	if resolver != "" {
		dialer := &net.Dialer{}
		resolverClient = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network string, address string) (net.Conn, error) {
				return dialer.DialContext(ctx, "udp", net.JoinHostPort(resolver, "53"))
			},
		}
	}
	recordType = strings.ToUpper(recordType)
	if recordType == "" {
		recordType = "A"
	}

	var records []DNSRecord
	switch recordType {
	case "A", "AAAA":
		ips, err := resolverClient.LookupIP(ctx, "ip", name)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if recordType == "A" && ip.To4() == nil {
				continue
			}
			if recordType == "AAAA" && ip.To4() != nil {
				continue
			}
			records = append(records, DNSRecord{Name: name, Type: recordType, Value: ip.String()})
		}
	case "MX":
		mxs, err := resolverClient.LookupMX(ctx, name)
		if err != nil {
			return nil, err
		}
		for _, mx := range mxs {
			records = append(records, DNSRecord{Name: name, Type: recordType, Value: fmt.Sprintf("%d %s", mx.Pref, mx.Host)})
		}
	case "TXT":
		values, err := resolverClient.LookupTXT(ctx, name)
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			records = append(records, DNSRecord{Name: name, Type: recordType, Value: value})
		}
	case "CNAME":
		value, err := resolverClient.LookupCNAME(ctx, name)
		if err != nil {
			return nil, err
		}
		records = append(records, DNSRecord{Name: name, Type: recordType, Value: value})
	case "NS":
		values, err := resolverClient.LookupNS(ctx, name)
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			records = append(records, DNSRecord{Name: name, Type: recordType, Value: value.Host})
		}
	default:
		return nil, fmt.Errorf("unsupported DNS type %q", recordType)
	}
	return records, nil
}

func HTTP(ctx context.Context, target string, method string) (HTTPProbe, error) {
	if method == "" {
		method = http.MethodGet
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "https://" + target
	}
	var redirects []string
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			redirects = append(redirects, req.URL.String())
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return HTTPProbe{}, err
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return HTTPProbe{}, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	return HTTPProbe{
		URL:           target,
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		FinalURL:      resp.Request.URL.String(),
		Method:        method,
		Headers:       resp.Header,
		RedirectChain: redirects,
		Duration:      time.Since(start).Round(time.Millisecond).String(),
	}, nil
}

func TLSInfo(ctx context.Context, host string, includePEM bool) (CertificateInfo, error) {
	if parsed, err := url.Parse(host); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "443")
	}
	dialer := &net.Dialer{}
	conn, err := tls.DialWithDialer(dialer, "tcp", host, &tls.Config{ServerName: hostnameOnly(host)})
	if err != nil {
		return CertificateInfo{}, err
	}
	defer conn.Close()
	if err := conn.HandshakeContext(ctx); err != nil {
		return CertificateInfo{}, err
	}
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return CertificateInfo{}, fmt.Errorf("no peer certificate")
	}
	cert := state.PeerCertificates[0]
	info := CertificateInfo{
		Subject:      cert.Subject.String(),
		Issuer:       cert.Issuer.String(),
		DNSNames:     cert.DNSNames,
		NotBefore:    cert.NotBefore.Format(time.RFC3339),
		NotAfter:     cert.NotAfter.Format(time.RFC3339),
		Expired:      time.Now().After(cert.NotAfter),
		ExpiresIn:    time.Until(cert.NotAfter).Round(time.Hour).String(),
		TLSVersion:   tlsVersion(state.Version),
		CipherSuite:  tls.CipherSuiteName(state.CipherSuite),
		ChainLength:  len(state.PeerCertificates),
		SerialNumber: cert.SerialNumber.String(),
	}
	if includePEM {
		info.PEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
	}
	return info, nil
}

func Latency(ctx context.Context, target string, tcpPort int, includeHTTP bool) (LatencyResult, error) {
	start := time.Now()
	host := target
	if parsed, err := url.Parse(target); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}

	dnsStart := time.Now()
	ips, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return LatencyResult{}, err
	}
	result := LatencyResult{
		Target:      target,
		DNS:         time.Since(dnsStart).Round(time.Millisecond).String(),
		ResolvedIPs: ips,
	}

	if tcpPort > 0 {
		tcpStart := time.Now()
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(tcpPort)))
		if err != nil {
			return LatencyResult{}, err
		}
		_ = conn.Close()
		result.TCP = time.Since(tcpStart).Round(time.Millisecond).String()
	}

	if includeHTTP {
		httpTarget := target
		if !strings.HasPrefix(httpTarget, "http://") && !strings.HasPrefix(httpTarget, "https://") {
			httpTarget = "https://" + httpTarget
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, httpTarget, nil)
		if err != nil {
			return LatencyResult{}, err
		}
		httpStart := time.Now()
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return LatencyResult{}, err
		}
		_ = resp.Body.Close()
		result.HTTP = time.Since(httpStart).Round(time.Millisecond).String()
		result.HTTPStatus = resp.Status
	}

	result.Total = time.Since(start).Round(time.Millisecond).String()
	return result, nil
}

func splitAddress(value string) (string, string) {
	ip, network, ok := strings.Cut(value, "/")
	if !ok {
		return value, ""
	}
	return ip, network
}

func DefaultTCPPort(target string) int {
	if parsed, err := url.Parse(target); err == nil && parsed.Scheme == "http" {
		return 80
	}
	return 443
}

func PortSummary(ports []Port) string {
	return fmt.Sprintf("%d listening ports", len(ports))
}

func hostnameOnly(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return host
}

func tlsVersion(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("0x%x", version)
	}
}

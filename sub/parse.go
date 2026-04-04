package sub

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// ParseLine parses a single subscription URI into a sing-box Outbound.
func ParseLine(line string) (option.Outbound, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return option.Outbound{}, fmt.Errorf("empty line")
	}

	u, err := url.Parse(line)
	if err != nil {
		return option.Outbound{}, fmt.Errorf("invalid URI: %w", err)
	}

	switch u.Scheme {
	case "trojan":
		return parseTrojan(u)
	case "hy2", "hysteria2":
		return parseHysteria2(u)
	case "https":
		return parseHTTP(u, true)
	case "http":
		return parseHTTP(u, false)
	case "socks", "socks5":
		return parseSOCKS(u)
	default:
		return option.Outbound{}, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
}

// Parse reads newline-separated URIs from r and returns all parsed Outbounds.
// The input may be plain text or base64-encoded; encoding is auto-detected.
// Blank lines are skipped.
func Parse(r io.Reader) ([]option.Outbound, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	data := decodeBase64(bytes.TrimSpace(raw))

	var outbounds []option.Outbound
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		ob, err := ParseLine(line)
		if err != nil {
			return nil, fmt.Errorf("parsing %q: %w", line, err)
		}
		outbounds = append(outbounds, ob)
	}
	return outbounds, nil
}

// decodeBase64 tries to base64-decode data. If the decoded result
// contains "://" (indicating protocol URIs), it returns the decoded bytes.
// Otherwise it returns the original data unchanged.
func decodeBase64(data []byte) []byte {
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err == nil && bytes.Contains(decoded, []byte("://")) {
		return decoded
	}
	return data
}

// serverOptions extracts server and port from a parsed URL.
// When no port is specified, the default is inferred from the scheme
// (443 for https/trojan/hy2/hysteria2, 80 for http, 1080 for socks/socks5).
func serverOptions(u *url.URL) (option.ServerOptions, error) {
	host := u.Hostname()
	if host == "" {
		return option.ServerOptions{}, fmt.Errorf("missing host")
	}

	portStr := u.Port()
	if portStr == "" {
		return option.ServerOptions{Server: host, ServerPort: defaultPort(u.Scheme)}, nil
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return option.ServerOptions{}, fmt.Errorf("invalid port %q: %w", portStr, err)
	}
	return option.ServerOptions{
		Server:     host,
		ServerPort: uint16(port),
	}, nil
}

func defaultPort(scheme string) uint16 {
	switch scheme {
	case "https", "trojan", "hy2", "hysteria2":
		return 443
	case "http":
		return 80
	case "socks", "socks5":
		return 1080
	default:
		return 0
	}
}

// outboundTLS builds OutboundTLSOptions from common query parameters.
func outboundTLS(q url.Values, defaultSNI string) *option.OutboundTLSOptions {
	security := q.Get("security")
	if security == "" || security == "none" {
		return nil
	}

	tls := &option.OutboundTLSOptions{
		Enabled: true,
	}

	if sni := q.Get("sni"); sni != "" {
		tls.ServerName = sni
	} else if defaultSNI != "" {
		tls.ServerName = defaultSNI
	}

	if q.Get("allowInsecure") == "1" {
		tls.Insecure = true
	}

	if fp := q.Get("fp"); fp != "" {
		tls.UTLS = &option.OutboundUTLSOptions{
			Enabled:     true,
			Fingerprint: fp,
		}
	}

	return tls
}

// v2rayTransport builds V2RayTransportOptions from query parameters.
func v2rayTransport(q url.Values) *option.V2RayTransportOptions {
	typ := q.Get("type")
	if typ == "" {
		return nil
	}

	switch typ {
	case "ws":
		t := &option.V2RayTransportOptions{
			Type: C.V2RayTransportTypeWebsocket,
		}
		t.WebsocketOptions.Path = q.Get("path")
		if host := q.Get("host"); host != "" {
			t.WebsocketOptions.Headers = badoption.HTTPHeader{
				"Host": {host},
			}
		}
		return t
	case "grpc":
		t := &option.V2RayTransportOptions{
			Type: C.V2RayTransportTypeGRPC,
		}
		t.GRPCOptions.ServiceName = q.Get("serviceName")
		return t
	case "http":
		t := &option.V2RayTransportOptions{
			Type: C.V2RayTransportTypeHTTP,
		}
		t.HTTPOptions.Path = q.Get("path")
		if host := q.Get("host"); host != "" {
			t.HTTPOptions.Host = []string{host}
		}
		return t
	default:
		return nil
	}
}

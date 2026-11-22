package sub

import (
	"encoding/base64"
	"strings"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

func TestParseTrojan(t *testing.T) {
	line := `trojan://testpass@example.com:443?security=tls&sni=example.com&fp=chrome&type=ws&host=example.com&path=%2Fws#Test-Trojan`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ob.Type != C.TypeTrojan {
		t.Fatalf("type = %q, want %q", ob.Type, C.TypeTrojan)
	}
	if ob.Tag != "Test-Trojan" {
		t.Fatalf("tag = %q, want %q", ob.Tag, "Test-Trojan")
	}
	opts := ob.Options.(*option.TrojanOutboundOptions)
	if opts.Server != "example.com" || opts.ServerPort != 443 {
		t.Fatalf("server = %s:%d", opts.Server, opts.ServerPort)
	}
	if opts.Password != "testpass" {
		t.Fatalf("password = %q", opts.Password)
	}
	if opts.TLS == nil || !opts.TLS.Enabled {
		t.Fatal("TLS not enabled")
	}
	if opts.TLS.ServerName != "example.com" {
		t.Fatalf("sni = %q", opts.TLS.ServerName)
	}
	if opts.TLS.UTLS == nil || opts.TLS.UTLS.Fingerprint != "chrome" {
		t.Fatal("utls fingerprint not set")
	}
	if opts.Transport == nil || opts.Transport.Type != C.V2RayTransportTypeWebsocket {
		t.Fatal("transport not websocket")
	}
	if opts.Transport.WebsocketOptions.Path != "/ws" {
		t.Fatalf("ws path = %q", opts.Transport.WebsocketOptions.Path)
	}
}

func TestParseTrojanIPv6(t *testing.T) {
	line := `trojan://testpass@[2001:db8::1]:443?security=tls&sni=example.com&fp=chrome&type=ws&host=example.com&path=%2Fws#Test-Trojan-v6`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	opts := ob.Options.(*option.TrojanOutboundOptions)
	if opts.Server != "2001:db8::1" {
		t.Fatalf("server = %q, want IPv6 address", opts.Server)
	}
}

func TestParseTrojanInsecure(t *testing.T) {
	line := `trojan://testpass@192.0.2.1:443?security=tls&sni=example.com&fp=chrome&allowInsecure=1&type=ws&host=example.com&path=%2Fws#Test-Insecure`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	opts := ob.Options.(*option.TrojanOutboundOptions)
	if opts.TLS == nil || !opts.TLS.Insecure {
		t.Fatal("TLS insecure not set")
	}
}

func TestParseHysteria2(t *testing.T) {
	line := `hy2://testpass@198.51.100.1:51419?sni=example.com&obfs=salamander&obfs-password=obfsecret#Test-Hy2`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ob.Type != C.TypeHysteria2 {
		t.Fatalf("type = %q", ob.Type)
	}
	if ob.Tag != "Test-Hy2" {
		t.Fatalf("tag = %q", ob.Tag)
	}
	opts := ob.Options.(*option.Hysteria2OutboundOptions)
	if opts.Server != "198.51.100.1" || opts.ServerPort != 51419 {
		t.Fatalf("server = %s:%d", opts.Server, opts.ServerPort)
	}
	if opts.Password != "testpass" {
		t.Fatalf("password = %q", opts.Password)
	}
	if opts.TLS == nil || !opts.TLS.Enabled || opts.TLS.ServerName != "example.com" {
		t.Fatal("TLS misconfigured")
	}
	if opts.Obfs == nil || opts.Obfs.Type != "salamander" || opts.Obfs.Password != "obfsecret" {
		t.Fatal("obfs misconfigured")
	}
}

func TestParseHTTPS(t *testing.T) {
	line := `https://alice:s3cret@203.0.113.1?security=tls&sni=proxy.example.com#Test-HTTPS`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ob.Type != C.TypeHTTP {
		t.Fatalf("type = %q", ob.Type)
	}
	if ob.Tag != "Test-HTTPS" {
		t.Fatalf("tag = %q", ob.Tag)
	}
	opts := ob.Options.(*option.HTTPOutboundOptions)
	if opts.ServerPort != 443 {
		t.Fatalf("server_port = %d, want 443 (default for https)", opts.ServerPort)
	}
	if opts.Username != "alice" || opts.Password != "s3cret" {
		t.Fatalf("auth = %s:%s", opts.Username, opts.Password)
	}
	if opts.TLS == nil || !opts.TLS.Enabled || opts.TLS.ServerName != "proxy.example.com" {
		t.Fatal("TLS misconfigured")
	}
}

func TestParseHTTP(t *testing.T) {
	line := `http://bob:hunter2@proxy.example.com:2080#Test-HTTP`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ob.Type != C.TypeHTTP {
		t.Fatalf("type = %q", ob.Type)
	}
	opts := ob.Options.(*option.HTTPOutboundOptions)
	if opts.Server != "proxy.example.com" || opts.ServerPort != 2080 {
		t.Fatalf("server = %s:%d", opts.Server, opts.ServerPort)
	}
	if opts.Username != "bob" || opts.Password != "hunter2" {
		t.Fatalf("auth = %s:%s", opts.Username, opts.Password)
	}
	if opts.TLS != nil {
		t.Fatal("TLS should not be set for http://")
	}
}

func TestParseHTTPDefaultPort(t *testing.T) {
	line := `http://192.168.1.1#Test-HTTP-DefaultPort`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	opts := ob.Options.(*option.HTTPOutboundOptions)
	if opts.ServerPort != 80 {
		t.Fatalf("server_port = %d, want 80 (default for http)", opts.ServerPort)
	}
}

func TestParseHTTPNoAuth(t *testing.T) {
	line := `http://192.168.1.1:1080#Test-HTTP-NoAuth`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	opts := ob.Options.(*option.HTTPOutboundOptions)
	if opts.Username != "" || opts.Password != "" {
		t.Fatalf("unexpected auth = %s:%s", opts.Username, opts.Password)
	}
}

func TestParseSOCKS(t *testing.T) {
	line := `socks://bob:hunter2@proxy.example.com:1080#Test-SOCKS`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ob.Type != C.TypeSOCKS {
		t.Fatalf("type = %q", ob.Type)
	}
	opts := ob.Options.(*option.SOCKSOutboundOptions)
	if opts.Username != "bob" || opts.Password != "hunter2" {
		t.Fatalf("auth = %s:%s", opts.Username, opts.Password)
	}
}

func TestParseSOCKSDefaultPort(t *testing.T) {
	line := `socks://10.0.0.1#Test-SOCKS-DefaultPort`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	opts := ob.Options.(*option.SOCKSOutboundOptions)
	if opts.ServerPort != 1080 {
		t.Fatalf("server_port = %d, want 1080 (default for socks)", opts.ServerPort)
	}
}

func TestParseSOCKSNoAuth(t *testing.T) {
	line := `socks://10.0.0.1:1080#Test-SOCKS-NoAuth`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	opts := ob.Options.(*option.SOCKSOutboundOptions)
	if opts.Username != "" || opts.Password != "" {
		t.Fatalf("unexpected auth = %s:%s", opts.Username, opts.Password)
	}
}

func TestParseMultiline(t *testing.T) {
	input := `trojan://abc@example.com:443?security=tls&type=ws&path=/ws#tag1
hy2://pass@example.com:8443?sni=example.com&obfs=salamander&obfs-password=secret#tag2

socks://10.0.0.1:1080#tag3
`
	outbounds, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(outbounds) != 3 {
		t.Fatalf("got %d outbounds, want 3", len(outbounds))
	}
	if outbounds[0].Type != C.TypeTrojan {
		t.Fatalf("[0] type = %q", outbounds[0].Type)
	}
	if outbounds[1].Type != C.TypeHysteria2 {
		t.Fatalf("[1] type = %q", outbounds[1].Type)
	}
	if outbounds[2].Type != C.TypeSOCKS {
		t.Fatalf("[2] type = %q", outbounds[2].Type)
	}
}

func TestParseBase64Encoded(t *testing.T) {
	plain := "trojan://abc@example.com:443?security=tls&type=ws&path=/ws#tag1\nsocks://10.0.0.1:1080#tag2\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(plain))
	outbounds, err := Parse(strings.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if len(outbounds) != 2 {
		t.Fatalf("got %d outbounds, want 2", len(outbounds))
	}
	if outbounds[0].Type != C.TypeTrojan {
		t.Fatalf("[0] type = %q", outbounds[0].Type)
	}
	if outbounds[1].Type != C.TypeSOCKS {
		t.Fatalf("[1] type = %q", outbounds[1].Type)
	}
}

func TestParseBodyReturnsLinesAndOutbounds(t *testing.T) {
	body := []byte("trojan://abc@example.com:443?security=tls&type=ws&path=/ws#tag1\nsocks://10.0.0.1:1080#tag2\n")

	lines, outbounds, err := ParseBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0] != "trojan://abc@example.com:443?security=tls&type=ws&path=/ws#tag1" {
		t.Fatalf("unexpected first line %q", lines[0])
	}
	if len(outbounds) != 2 {
		t.Fatalf("got %d outbounds, want 2", len(outbounds))
	}
}

func TestParseTrojanGRPCTransport(t *testing.T) {
	line := `trojan://testpass@example.com:443?security=tls&type=grpc&serviceName=mygrpc#Test-GRPC`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	opts := ob.Options.(*option.TrojanOutboundOptions)
	if opts.Transport == nil || opts.Transport.Type != C.V2RayTransportTypeGRPC {
		t.Fatal("transport not grpc")
	}
	if opts.Transport.GRPCOptions.ServiceName != "mygrpc" {
		t.Fatalf("grpc service = %q", opts.Transport.GRPCOptions.ServiceName)
	}
}

func TestParseTrojanHTTPTransport(t *testing.T) {
	line := `trojan://testpass@example.com:443?security=tls&type=http&host=example.com&path=%2Fh2#Test-H2`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	opts := ob.Options.(*option.TrojanOutboundOptions)
	if opts.Transport == nil || opts.Transport.Type != C.V2RayTransportTypeHTTP {
		t.Fatal("transport not http")
	}
	if opts.Transport.HTTPOptions.Path != "/h2" {
		t.Fatalf("http path = %q", opts.Transport.HTTPOptions.Path)
	}
	if len(opts.Transport.HTTPOptions.Host) != 1 || opts.Transport.HTTPOptions.Host[0] != "example.com" {
		t.Fatalf("http host = %v", opts.Transport.HTTPOptions.Host)
	}
}

func TestParseTrojanNoTransport(t *testing.T) {
	line := `trojan://testpass@example.com:443?security=tls#Test-NoTransport`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	opts := ob.Options.(*option.TrojanOutboundOptions)
	if opts.Transport != nil {
		t.Fatal("expected no transport")
	}
}

func TestParseEmptyLine(t *testing.T) {
	_, err := ParseLine("")
	if err == nil {
		t.Fatal("expected error for empty line")
	}
}

func TestParseLineInvalidPort(t *testing.T) {
	_, err := ParseLine("socks://10.0.0.1:99999#bad-port")
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

func TestParseTrojanMissingPassword(t *testing.T) {
	_, err := ParseLine("trojan://@example.com:443#no-pass")
	if err == nil {
		t.Fatal("expected error for missing trojan password")
	}
}

func TestParseHysteria2MissingPassword(t *testing.T) {
	_, err := ParseLine("hy2://@example.com:443?sni=example.com#no-pass")
	if err == nil {
		t.Fatal("expected error for missing hy2 password")
	}
}

func TestParseNoTag(t *testing.T) {
	outbounds, err := Parse(strings.NewReader("socks://10.0.0.1:1080\nhttp://10.0.0.2:8080\n"))
	if err != nil {
		t.Fatal(err)
	}
	if outbounds[0].Tag != "0" {
		t.Fatalf("[0] tag = %q, want \"0\"", outbounds[0].Tag)
	}
	if outbounds[1].Tag != "1" {
		t.Fatalf("[1] tag = %q, want \"1\"", outbounds[1].Tag)
	}
}

func TestConfig(t *testing.T) {
	outbounds, err := Parse(strings.NewReader("socks://10.0.0.1:1080#my-socks\nhttp://10.0.0.2:8080#my-http\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Config(outbounds, "my-socks")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Log == nil || cfg.Log.Level != "info" {
		t.Fatal("log level not set")
	}
	if len(cfg.Inbounds) != 1 {
		t.Fatalf("inbounds = %d, want 1", len(cfg.Inbounds))
	}
	if len(cfg.Outbounds) != 2 {
		t.Fatalf("outbounds = %d, want 2", len(cfg.Outbounds))
	}
	if cfg.Route == nil || cfg.Route.Final != "my-socks" {
		t.Fatal("route final not set")
	}
}

func TestConfigUnknownTag(t *testing.T) {
	outbounds, err := Parse(strings.NewReader("socks://10.0.0.1:1080#my-socks\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = Config(outbounds, "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown tag")
	}
}

func TestConfigTun(t *testing.T) {
	outbounds, err := Parse(strings.NewReader("socks://10.0.0.1:1080#direct\nhttp://10.0.0.2:8080#my-http\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ConfigWithMode(outbounds, "my-http", ModeTun)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inbounds) != 1 || cfg.Inbounds[0].Type != C.TypeTun {
		t.Fatalf("inbounds = %#v, want single tun inbound", cfg.Inbounds)
	}
	tunOpts := cfg.Inbounds[0].Options.(*option.TunInboundOptions)
	if len(tunOpts.Address) != 2 || tunOpts.Address[0].String() != TunIPv4CIDR || tunOpts.Address[1].String() != TunIPv6CIDR {
		t.Fatalf("tun address = %v, want [%s %s]", tunOpts.Address, TunIPv4CIDR, TunIPv6CIDR)
	}
	if !tunOpts.AutoRoute || !tunOpts.StrictRoute || tunOpts.Stack != "mixed" {
		t.Fatalf("tun options = %+v", tunOpts)
	}
	if cfg.DNS == nil || len(cfg.DNS.Servers) != 2 || cfg.DNS.Final != "remote" {
		t.Fatalf("dns = %+v", cfg.DNS)
	}
	for _, s := range cfg.DNS.Servers {
		if s.Type == C.DNSTypeFakeIP {
			t.Fatal("fakeip must not be configured")
		}
	}
	if cfg.Route == nil || !cfg.Route.AutoDetectInterface || cfg.Route.Final != "my-http" {
		t.Fatalf("route = %+v", cfg.Route)
	}
	if cfg.Route.DefaultDomainResolver == nil || cfg.Route.DefaultDomainResolver.Server != "local" {
		t.Fatalf("default_domain_resolver = %+v", cfg.Route.DefaultDomainResolver)
	}
	// Existing "direct" outbound tag must not collide with injected direct outbound.
	if len(cfg.Outbounds) != 3 || cfg.Outbounds[2].Type != C.TypeDirect || cfg.Outbounds[2].Tag == "direct" {
		t.Fatalf("injected direct outbound = %+v", cfg.Outbounds)
	}
}

func TestParseUnsupportedScheme(t *testing.T) {
	_, err := ParseLine("vmess://something")
	if err == nil {
		t.Fatal("expected error for unsupported scheme")
	}
}

func TestParseVLESSWS(t *testing.T) {
	line := `vless://00000000-0000-4000-8000-000000000001@example.com:443?security=tls&sni=sni.example.com&fp=chrome&encryption=none&type=ws&host=sni.example.com&path=%2Fchat#Test-VLESS-WS`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ob.Type != C.TypeVLESS {
		t.Fatalf("type = %q, want %q", ob.Type, C.TypeVLESS)
	}
	if ob.Tag != "Test-VLESS-WS" {
		t.Fatalf("tag = %q, want %q", ob.Tag, "Test-VLESS-WS")
	}
	opts := ob.Options.(*option.VLESSOutboundOptions)
	if opts.UUID != "00000000-0000-4000-8000-000000000001" {
		t.Fatalf("uuid = %q", opts.UUID)
	}
	if opts.Server != "example.com" || opts.ServerPort != 443 {
		t.Fatalf("server = %s:%d", opts.Server, opts.ServerPort)
	}
	if opts.TLS == nil || !opts.TLS.Enabled || opts.TLS.ServerName != "sni.example.com" {
		t.Fatal("TLS misconfigured")
	}
	if opts.TLS.UTLS == nil || !opts.TLS.UTLS.Enabled || opts.TLS.UTLS.Fingerprint != "chrome" {
		t.Fatal("uTLS misconfigured")
	}
	if opts.Transport == nil || opts.Transport.Type != C.V2RayTransportTypeWebsocket {
		t.Fatal("transport not websocket")
	}
	if opts.Transport.WebsocketOptions.Path != "/chat" {
		t.Fatalf("ws path = %q", opts.Transport.WebsocketOptions.Path)
	}
	if got := opts.Transport.WebsocketOptions.Headers["Host"]; len(got) != 1 || got[0] != "sni.example.com" {
		t.Fatalf("ws host header = %v", got)
	}
}

func TestParseVLESSTCPNoTLS(t *testing.T) {
	line := `vless://00000000-0000-4000-8000-000000000001@192.0.2.10:8080?security=none&encryption=none&type=tcp&tfo=1#Test-VLESS-TCP`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	opts := ob.Options.(*option.VLESSOutboundOptions)
	if opts.TLS != nil {
		t.Fatal("expected nil TLS for security=none")
	}
	if opts.Transport != nil {
		t.Fatal("expected nil transport for type=tcp")
	}
	if !opts.TCPFastOpen {
		t.Fatal("expected tcp_fast_open = true for tfo=1")
	}
}

func TestParseVLESSReality(t *testing.T) {
	line := `vless://00000000-0000-4000-8000-000000000001@example.com:443?security=reality&sni=www.example.com&fp=chrome&pbk=pubkey123&sid=abcd&flow=xtls-rprx-vision#Test-Reality`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	opts := ob.Options.(*option.VLESSOutboundOptions)
	if opts.Flow != "xtls-rprx-vision" {
		t.Fatalf("flow = %q", opts.Flow)
	}
	if opts.TLS == nil || !opts.TLS.Enabled || opts.TLS.Reality == nil || !opts.TLS.Reality.Enabled {
		t.Fatal("reality TLS misconfigured")
	}
	if opts.TLS.Reality.PublicKey != "pubkey123" || opts.TLS.Reality.ShortID != "abcd" {
		t.Fatalf("reality = %+v", opts.TLS.Reality)
	}
}

func TestParseVLESSMissingUUID(t *testing.T) {
	_, err := ParseLine("vless://@example.com:443#no-uuid")
	if err == nil {
		t.Fatal("expected error for missing vless uuid")
	}
}

func TestParseTrojanWSEarlyDataAndTFO(t *testing.T) {
	line := `trojan://testpass@example.com:443?security=tls&sni=example.com&fp=chrome&tfo=1&type=ws&host=example.com&path=%2Fws&ed=2560&eh=Sec-WebSocket-Protocol#Test-Trojan-ED`
	ob, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	opts := ob.Options.(*option.TrojanOutboundOptions)
	if !opts.TCPFastOpen {
		t.Fatal("expected tcp_fast_open = true")
	}
	if opts.Transport == nil || opts.Transport.WebsocketOptions.MaxEarlyData != 2560 {
		t.Fatalf("max_early_data = %v, want 2560", opts.Transport)
	}
	if opts.Transport.WebsocketOptions.EarlyDataHeaderName != "Sec-WebSocket-Protocol" {
		t.Fatalf("early_data_header_name = %q", opts.Transport.WebsocketOptions.EarlyDataHeaderName)
	}
}

func TestParseHTTPSFingerprintAndHy2Insecure(t *testing.T) {
	ob, err := ParseLine(`https://alice:s3cret@203.0.113.1:443?security=tls&sni=proxy.example.com&fp=chrome#Test-HTTPS-FP`)
	if err != nil {
		t.Fatal(err)
	}
	httpOpts := ob.Options.(*option.HTTPOutboundOptions)
	if httpOpts.TLS == nil || httpOpts.TLS.UTLS == nil || httpOpts.TLS.UTLS.Fingerprint != "chrome" {
		t.Fatal("expected uTLS fingerprint on https outbound")
	}

	ob2, err := ParseLine(`hy2://testpass@198.51.100.1:443?sni=example.com&insecure=1#Test-Hy2-Insecure`)
	if err != nil {
		t.Fatal(err)
	}
	hy2Opts := ob2.Options.(*option.Hysteria2OutboundOptions)
	if hy2Opts.TLS == nil || !hy2Opts.TLS.Insecure {
		t.Fatal("expected insecure TLS on hy2 with insecure=1")
	}
}

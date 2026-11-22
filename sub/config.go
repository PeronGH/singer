package sub

import (
	"fmt"
	"net/netip"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// Mode selects how sing-box captures local traffic.
type Mode int

const (
	// ModeProxy uses a mixed HTTP/SOCKS5 inbound with system proxy enabled.
	ModeProxy Mode = iota
	// ModeTun uses a TUN inbound with auto_route and real DNS (no FakeIP).
	ModeTun
)

const (
	// TunIPv4CIDR is an RFC 1918 /30 point-to-point subnet outside Docker's
	// default 172.17-172.31 pool, sing-box's 172.19.0.1/30 example, Clash's
	// 198.18.0.0/15 FakeIP range, and Tailscale's 100.64.0.0/10 CGNAT space.
	TunIPv4CIDR = "172.16.253.1/30"
	// TunIPv6CIDR is an RFC 4193 ULA /126 point-to-point subnet with a
	// dedicated 40-bit global ID.
	TunIPv6CIDR = "fd9c:4b2a:7e31::1/126"
)

// Config builds a full sing-box config in proxy mode that routes traffic through
// the outbound identified by tag.
func Config(outbounds []option.Outbound, outboundTag string) (option.Options, error) {
	return ConfigWithMode(outbounds, outboundTag, ModeProxy)
}

// ConfigWithMode builds a full sing-box config for the requested capture mode.
func ConfigWithMode(outbounds []option.Outbound, outboundTag string, mode Mode) (option.Options, error) {
	found := false
	for _, ob := range outbounds {
		if ob.Tag == outboundTag {
			found = true
			break
		}
	}
	if !found {
		return option.Options{}, fmt.Errorf("outbound tag %q not found", outboundTag)
	}

	switch mode {
	case ModeProxy:
		return option.Options{
			Log: &option.LogOptions{
				Level: "info",
			},
			Inbounds: []option.Inbound{{
				Type: C.TypeMixed,
				Tag:  "mixed-in",
				Options: &option.HTTPMixedInboundOptions{
					ListenOptions: option.ListenOptions{
						ListenPort: 2088,
					},
					SetSystemProxy: true,
				},
			}},
			Outbounds: outbounds,
			Route: &option.RouteOptions{
				Final: outboundTag,
			},
		}, nil
	case ModeTun:
		return tunConfig(outbounds, outboundTag), nil
	default:
		return option.Options{}, fmt.Errorf("unsupported config mode: %d", mode)
	}
}

func tunConfig(outbounds []option.Outbound, outboundTag string) option.Options {
	directTag := uniqueTag(outbounds, "direct")
	allOutbounds := make([]option.Outbound, 0, len(outbounds)+1)
	allOutbounds = append(allOutbounds, outbounds...)
	allOutbounds = append(allOutbounds, option.Outbound{
		Type:    C.TypeDirect,
		Tag:     directTag,
		Options: &option.DirectOutboundOptions{},
	})

	remoteDNS := &option.RemoteHTTPSDNSServerOptions{}
	remoteDNS.Server = "1.1.1.1"
	remoteDNS.Detour = outboundTag

	return option.Options{
		Log: &option.LogOptions{
			Level: "info",
		},
		DNS: &option.DNSOptions{
			RawDNSOptions: option.RawDNSOptions{
				Servers: []option.DNSServerOptions{
					{
						Type:    C.DNSTypeHTTPS,
						Tag:     "remote",
						Options: remoteDNS,
					},
					{
						Type:    C.DNSTypeLocal,
						Tag:     "local",
						Options: &option.LocalDNSServerOptions{},
					},
				},
				Final: "remote",
			},
		},
		Inbounds: []option.Inbound{{
			Type: C.TypeTun,
			Tag:  "tun-in",
			Options: &option.TunInboundOptions{
				Address: badoption.Listable[netip.Prefix]{
					netip.MustParsePrefix(TunIPv4CIDR),
					netip.MustParsePrefix(TunIPv6CIDR),
				},
				AutoRoute:   true,
				StrictRoute: true,
				Stack:       "mixed",
			},
		}},
		Outbounds: allOutbounds,
		Route: &option.RouteOptions{
			AutoDetectInterface: true,
			DefaultDomainResolver: &option.DomainResolveOptions{
				Server: "local",
			},
			Rules: []option.Rule{
				{
					Type: C.RuleTypeDefault,
					DefaultOptions: option.DefaultRule{
						RuleAction: option.RuleAction{
							Action: C.RuleActionTypeSniff,
						},
					},
				},
				{
					Type: C.RuleTypeDefault,
					DefaultOptions: option.DefaultRule{
						RawDefaultRule: option.RawDefaultRule{
							Protocol: badoption.Listable[string]{"dns"},
						},
						RuleAction: option.RuleAction{
							Action: C.RuleActionTypeHijackDNS,
						},
					},
				},
				{
					Type: C.RuleTypeDefault,
					DefaultOptions: option.DefaultRule{
						RawDefaultRule: option.RawDefaultRule{
							IPIsPrivate: true,
						},
						RuleAction: option.RuleAction{
							Action: C.RuleActionTypeRoute,
							RouteOptions: option.RouteActionOptions{
								Outbound: directTag,
							},
						},
					},
				},
			},
			Final: outboundTag,
		},
	}
}

func uniqueTag(outbounds []option.Outbound, base string) string {
	used := make(map[string]struct{}, len(outbounds))
	for _, ob := range outbounds {
		used[ob.Tag] = struct{}{}
	}
	if _, exists := used[base]; !exists {
		return base
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("singer-%s-%d", base, i)
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}

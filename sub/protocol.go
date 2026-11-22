package sub

import (
	"fmt"
	"net/url"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

func parseVLESS(u *url.URL) (option.Outbound, error) {
	server, err := serverOptions(u)
	if err != nil {
		return option.Outbound{}, err
	}

	uuid := u.User.Username()
	if uuid == "" {
		return option.Outbound{}, fmt.Errorf("vless: missing uuid")
	}

	q := u.Query()
	opts := &option.VLESSOutboundOptions{
		DialerOptions: dialerOptions(q),
		ServerOptions: server,
		UUID:          uuid,
		Flow:          q.Get("flow"),
		Transport:     v2rayTransport(q),
	}
	opts.TLS = outboundTLS(q, server.Server)
	if pe := q.Get("packetEncoding"); pe != "" {
		opts.PacketEncoding = &pe
	}

	return option.Outbound{
		Type:    C.TypeVLESS,
		Tag:     u.Fragment,
		Options: opts,
	}, nil
}

func parseTrojan(u *url.URL) (option.Outbound, error) {
	server, err := serverOptions(u)
	if err != nil {
		return option.Outbound{}, err
	}

	password := u.User.Username()
	if password == "" {
		return option.Outbound{}, fmt.Errorf("trojan: missing password")
	}

	q := u.Query()
	opts := &option.TrojanOutboundOptions{
		DialerOptions: dialerOptions(q),
		ServerOptions: server,
		Password:      password,
		Transport:     v2rayTransport(q),
	}
	opts.TLS = outboundTLS(q, server.Server)

	return option.Outbound{
		Type:    C.TypeTrojan,
		Tag:     u.Fragment,
		Options: opts,
	}, nil
}

func parseHysteria2(u *url.URL) (option.Outbound, error) {
	server, err := serverOptions(u)
	if err != nil {
		return option.Outbound{}, err
	}

	password := u.User.Username()
	if password == "" {
		return option.Outbound{}, fmt.Errorf("hysteria2: missing password")
	}

	q := u.Query()
	opts := &option.Hysteria2OutboundOptions{
		ServerOptions: server,
		Password:      password,
	}

	// TLS — hysteria2 always uses TLS
	tls := &option.OutboundTLSOptions{Enabled: true}
	if sni := q.Get("sni"); sni != "" {
		tls.ServerName = sni
	}
	if isInsecure(q) {
		tls.Insecure = true
	}
	opts.TLS = tls

	// Obfuscation
	if obfsType := q.Get("obfs"); obfsType != "" {
		opts.Obfs = &option.Hysteria2Obfs{
			Type:     obfsType,
			Password: q.Get("obfs-password"),
		}
	}

	return option.Outbound{
		Type:    C.TypeHysteria2,
		Tag:     u.Fragment,
		Options: opts,
	}, nil
}

func parseHTTP(u *url.URL, tls bool) (option.Outbound, error) {
	server, err := serverOptions(u)
	if err != nil {
		return option.Outbound{}, err
	}

	opts := &option.HTTPOutboundOptions{
		ServerOptions: server,
	}

	if u.User != nil {
		opts.Username = u.User.Username()
		opts.Password, _ = u.User.Password()
	}

	if tls {
		q := u.Query()
		tlsOpts := &option.OutboundTLSOptions{Enabled: true}
		if sni := q.Get("sni"); sni != "" {
			tlsOpts.ServerName = sni
		}
		if isInsecure(q) {
			tlsOpts.Insecure = true
		}
		if fp := q.Get("fp"); fp != "" {
			tlsOpts.UTLS = &option.OutboundUTLSOptions{
				Enabled:     true,
				Fingerprint: fp,
			}
		}
		opts.TLS = tlsOpts
	}

	return option.Outbound{
		Type:    C.TypeHTTP,
		Tag:     u.Fragment,
		Options: opts,
	}, nil
}

func parseSOCKS(u *url.URL) (option.Outbound, error) {
	server, err := serverOptions(u)
	if err != nil {
		return option.Outbound{}, err
	}

	opts := &option.SOCKSOutboundOptions{
		ServerOptions: server,
	}

	if u.User != nil {
		opts.Username = u.User.Username()
		opts.Password, _ = u.User.Password()
	}

	return option.Outbound{
		Type:    C.TypeSOCKS,
		Tag:     u.Fragment,
		Options: opts,
	}, nil
}

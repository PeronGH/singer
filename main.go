package main

import (
	"os"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"
)

func main() {
	opts := option.Options{
		Inbounds: []option.Inbound{{
			Type:    C.TypeSOCKS,
			Options: &option.SocksInboundOptions{},
		}},
		Outbounds: []option.Outbound{{
			Type:    C.TypeShadowsocks,
			Options: &option.ShadowsocksOutboundOptions{},
		}},
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(opts)
}

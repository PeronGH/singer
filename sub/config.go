package sub

import (
	"fmt"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

// Config builds a full sing-box config that routes traffic through the
// outbound identified by tag. Includes a mixed inbound on port 2088
// with system proxy enabled.
func Config(outbounds []option.Outbound, outboundTag string) (option.Options, error) {
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
}

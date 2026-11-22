package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/PeronGH/singer/sub"
	"github.com/sagernet/sing/common/json"
)

func main() {
	outboundTag := flag.String("outbound", "", "outbound tag to route traffic through (enables full config output)")
	tunMode := flag.Bool("tun", false, "use TUN inbound instead of mixed system proxy (requires -outbound)")
	flag.Parse()

	outbounds, err := sub.Parse(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var result any = outbounds
	if *outboundTag != "" {
		mode := sub.ModeProxy
		if *tunMode {
			mode = sub.ModeTun
		}
		result, err = sub.ConfigWithMode(outbounds, *outboundTag, mode)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

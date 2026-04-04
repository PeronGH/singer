package main

import (
	"fmt"
	"os"

	"github.com/PeronGH/singer/sub"
	"github.com/sagernet/sing/common/json"
)

func main() {
	outbounds, err := sub.Parse(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(outbounds); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

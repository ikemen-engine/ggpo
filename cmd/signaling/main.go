// Command signaling runs the GGPO WebRTC signaling server.
//
// Usage:
//
//	go run github.com/ikemen-engine/ggpo/cmd/signaling -addr :3000
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ikemen-engine/ggpo/signaling"
)

func main() {
	addr := flag.String("addr", ":3000", "address to listen on")
	flag.Parse()

	fmt.Printf("Signaling server listening on %s\n", *addr)
	if err := signaling.NewServer().ListenAndServe(*addr); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start server: %s\n", err)
		os.Exit(1)
	}
}

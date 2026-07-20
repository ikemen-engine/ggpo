//go:build !js || !wasm

package main

import (
	"flag"
	"log"
	"strings"
)

func main() {
	host := flag.Bool("host", false, "host the lobby (otherwise join it)")
	lobby := flag.String("lobby", "ggpo-test", "shared lobby id both players agree on")
	signaling := flag.String("signaling", "http://localhost:3000", "signaling server url")
	ice := flag.String("ice", "", "comma-separated STUN/TURN server URLs, e.g. stun:stun.l.google.com:19302")
	flag.Parse()

	if *ice == "" {
		*ice = "stun:stun.l.google.com:19302"
	}

	err := run(config{
		host:         *host,
		lobbyID:      *lobby,
		signalingURL: *signaling,
		iceServers:   strings.Split(*ice, ","),
	})

	if err != nil {
		log.Fatal(err)
	}
}

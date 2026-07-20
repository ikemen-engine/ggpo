//go:build js && wasm

package main

import (
	"log"
	"syscall/js"
)

// main reads its config from the page URL's query string, so the same wasm
// build serves both roles:
//
//	http://?host=1&lobby=test&signaling=https://sig.example.com -> hosts lobby "test" with signaling server https://sig.example.com
//	http://host/?lobby=test&signaling=https://sig.example.com         -> joins lobby "test" with signaling server https://sig.example.com
//
// An optional &signaling=<url> overrides the signaling server address.
func main() {
	params := js.Global().Get("URLSearchParams").New(
		js.Global().Get("location").Get("search"))

	get := func(key string) string {
		v := params.Call("get", key)
		if v.IsNull() {
			return ""
		}
		return v.String()
	}

	// getAll returns every value for a repeated key, so ICE servers can be
	// passed as &ice=stun:...&ice=turn:...
	getAll := func(key string) []string {
		arr := params.Call("getAll", key)
		out := make([]string, arr.Length())
		for i := range out {
			out[i] = arr.Index(i).String()
		}
		// default: use google's default STUN server
		if len(out) == 0 {
			return []string{"stun:stun.l.google.com:19302"}
		} else {
			return out
		}
	}

	host := get("host")
	err := run(config{
		host:         host == "1" || host == "true" || get("role") == "host",
		lobbyID:      get("lobby"),
		signalingURL: get("signaling"),
		iceServers:   getAll("ice"),
	})

	if err != nil {
		log.Fatal(err)
	}
}

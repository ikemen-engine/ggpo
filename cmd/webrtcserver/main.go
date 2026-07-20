//go:build !js

// Command webrtcserver runs the WebRTC example as a single deployable server: it
// relays signaling under /signaling and serves the pre-built WebAssembly game
// under /. It does no building itself, so it can be shipped as a plain binary
// plus a directory of static assets to a remote host.
//
// Because signaling and the game share one origin, the served page points the
// game at this server's own /signaling endpoint automatically
// (resolved relative to the page, so it survives a reverse-proxy path prefix like /webrtc/),
// so no &signaling= query param and no CORS is needed.
//
// To deploy this from a remote server:
// We recommend to just run ./scripts/package.sh from the root of this repository
//
// On your local machine:
//
//	 Build the static assets once (they go in -dir, default ./public):
//
//		GOOS=js GOARCH=wasm go build -o public/main.wasm ./example/webrtc
//		cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" public/
//
// Then build the server for the target host and run it there with the assets:
//
//	go build -o webrtcserver ./cmd/webrtcserver
//	./webrtcserver -addr :8080 -dir public
//	# open http://host:8080/?host=1&lobby=test in one tab
//	# open http://host:8080/?lobby=test        in another
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/ikemen-engine/ggpo/signaling"
)

const indexHTML = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>ggpo webrtc example</title></head>
<body>
<script src="wasm_exec.js"></script>
<script>
// We set the signaling server on the same machine since its just a different endpoint to the WASM game.
const params = new URLSearchParams(location.search);
if (!params.get("signaling")) {
	params.set("signaling", new URL("signaling", location.href).href);
	history.replaceState(null, "", location.pathname + "?" + params.toString());
}
const go = new Go();
go.env = {};
go.argv = ["js"];
WebAssembly.instantiateStreaming(fetch("main.wasm"), go.importObject)
	.then((result) => go.run(result.instance))
	.catch((err) => { document.body.textContent = err; console.error(err); });
</script>
</body>
</html>
`

func main() {
	addr := flag.String("addr", ":8080", "address to serve on")
	dir := flag.String("dir", "public", "directory holding the pre-built main.wasm and wasm_exec.js")
	flag.Parse()

	files := http.FileServer(http.Dir(*dir))

	mux := http.NewServeMux()

	// Signaling endpoints, mounted under /signaling so the client's base URL is
	// <origin>/signaling and the server's own paths (/lobby/*, /offer/*, ...)
	// live under it.
	mux.Handle("/signaling/", http.StripPrefix("/signaling", signaling.NewServer().Handler()))

	// Everything else: the index page at /, and the static assets (main.wasm,
	// wasm_exec.js) from -dir for any other path.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, indexHTML)
			return
		}
		// Some static hosts don't map .wasm; set it so instantiateStreaming works.
		if strings.HasSuffix(r.URL.Path, ".wasm") {
			w.Header().Set("Content-Type", "application/wasm")
		}
		files.ServeHTTP(w, r)
	})

	log.Printf("serving %s on %s (signaling under /signaling; open /?host=1&lobby=test and /?lobby=test)", *dir, *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

//go:build !js

// Command serve builds the WebRTC example to WebAssembly and serves it with a
// minimal page so it can be run in the browser.
//
// It exists because wasmserve injects the host's environment variables into the
// page (go.env). On systems with a large environment (e.g. Windows) that
// overflows Go's wasm argv/env limit and the page fails to start with:
//
//	total length of command line and environment variables exceeds limit
//
// This server sets go.env to {} instead, sidestepping the limit. It rebuilds
// the wasm on every load, so code changes show up on refresh.
//
// Usage:
//
//	go run github.com/ikemen-engine/ggpo/cmd/signaling -addr :3000
//	go run ./example/webrtc/serve
//	# open http://localhost:8080/?host=1&lobby=test in one tab
//	# open http://localhost:8080/?lobby=test        in another
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const wasmPackage = "github.com/ikemen-engine/ggpo/example/webrtc"

const indexHTML = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>ggpo webrtc example</title></head>
<body>
<script src="wasm_exec.js"></script>
<script>
const go = new Go();
// Empty env on purpose: wasmserve copies the host environment here, which on
// Windows overflows Go's wasm argv/env limit. Config comes from the URL query
// string (see config_js.go), not the environment.
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
	flag.Parse()

	execJS, err := wasmExecJS()
	if err != nil {
		log.Fatal(err)
	}

	var buildMu sync.Mutex

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, indexHTML)
	})

	http.HandleFunc("/wasm_exec.js", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, execJS)
	})

	http.HandleFunc("/main.wasm", func(w http.ResponseWriter, r *http.Request) {
		buildMu.Lock()
		wasm, err := buildWasm()
		buildMu.Unlock()
		if err != nil {
			http.Error(w, "build failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/wasm")
		http.ServeFile(w, r, wasm)
	})

	log.Printf("serving %s at http://localhost%s (open /?host=1&lobby=test and /?lobby=test)", wasmPackage, *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

// buildWasm compiles the example to a temp .wasm file and returns its path.
func buildWasm() (string, error) {
	out := filepath.Join(os.TempDir(), "ggpo-webrtc-example.wasm")
	cmd := exec.Command("go", "build", "-o", out, wasmPackage)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out, nil
}

// wasmExecJS locates the wasm_exec.js shipped with the active Go toolchain. It
// moved from misc/wasm to lib/wasm in Go 1.24, so both are checked.
func wasmExecJS() (string, error) {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return "", fmt.Errorf("finding GOROOT: %w", err)
	}
	goroot := strings.TrimSpace(string(out))
	candidates := []string{
		filepath.Join(goroot, "lib", "wasm", "wasm_exec.js"),
		filepath.Join(goroot, "misc", "wasm", "wasm_exec.js"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("wasm_exec.js not found under %s", goroot)
}

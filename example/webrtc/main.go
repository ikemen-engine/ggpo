// Command webrtc is a two-player build of the example game that connects over
// WebRTC data channels instead of UDP, so it runs in the browser as WebAssembly.
//
// It is the browser counterpart of the UDP example and reuses the same game
// logic from example/game. Both players agree on a lobby id ahead of time: the
// host creates the lobby under that id and the other player joins with it, so no
// generated id has to be exchanged first.
//
// Native (two terminals), with a signaling server on :3000:
//
//	go run github.com/ikemen-engine/ggpo/cmd/signaling -addr :3000
//	go run ./example/webrtc -host -lobby test
//	go run ./example/webrtc -lobby test
//
// Browser (WebAssembly): use the bundled dev server in ./serve, which sets
// go.env = {} to avoid Go's wasm argv/env limit (wasmserve injects the host
// environment there instead, which overflows the limit on Windows):
//
//	go run github.com/ikemen-engine/ggpo/cmd/signaling -addr :3000
//	go run ./example/webrtc/serve
//	# open http://localhost:8080/?host=1&lobby=test in one tab
//	# open http://localhost:8080/?lobby=test        in another
package main

import (
	"context"
	"errors"
	"io"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ikemen-engine/ggpo"
	"github.com/ikemen-engine/ggpo/example/game"
	"github.com/ikemen-engine/ggpo/transport/webrtc"
)

// config is the per-instance setup, read from URL query params under wasm and
// from command-line flags natively.
type config struct {
	host         bool
	lobbyID      string
	signalingURL string
	// iceServers lists STUN/TURN server URLs for connectivity across networks.
	// Empty means host candidates only, which works on a LAN or localhost.
	iceServers []string
}

const numPlayers = 2

func run(cfg config) error {
	if cfg.lobbyID == "" {
		return errors.New("Can't have an empty lobby name")
	}
	if cfg.signalingURL == "" {
		return errors.New("Can't have an empty signaling server")
	}

	session := game.NewGameSession()
	peer := ggpo.NewPeer(&session, numPlayers, game.InputSize())
	game.SetBackend(&peer)
	session.SetBackend(&peer)

	tr := webrtc.NewTransport()
	peer.InitializeTransport(tr)

	// Global, shared across both peers: the host is always player 1, the client always player 2.
	// Each machine marks its own as local and the other as remote.
	const hostNum, clientNum = 1, 2

	players := make([]ggpo.Player, numPlayers)
	if cfg.host {
		players[0] = ggpo.NewLocalPlayer(20, hostNum)
		players[1] = ggpo.NewRemotePlayer(20, clientNum, ggpo.PlayerHandle(clientNum))
	} else {
		players[0] = ggpo.NewLocalPlayer(20, clientNum)
		players[1] = ggpo.NewRemotePlayer(20, hostNum, ggpo.PlayerHandle(hostNum))
	}

	channel := connect(cfg)
	tr.AddPeer(ggpo.PlayerHandle(players[1].Remote.Handle), channel)

	var localHandle ggpo.PlayerHandle
	for i := range players {
		var handle ggpo.PlayerHandle
		if err := peer.AddPlayer(&players[i], &handle); err != nil {
			log.Fatalf("AddPlayer failed: %s", err)
		}
		if players[i].PlayerType == ggpo.PlayerTypeLocal {
			game.SetCurrentPlayer(int(handle))
			localHandle = handle
		}
	}
	peer.SetDisconnectTimeout(3000)
	peer.SetDisconnectNotifyStart(1000)
	peer.SetFrameDelay(localHandle, game.FrameDelay)
	peer.Start()

	game.StartClock()
	if err := ebiten.RunGame(session.Game()); err != nil {
		log.Fatal(err)
		return err
	}
	return nil
}

// connect performs the signaling handshake and returns the established data
// channel to the remote peer.
func connect(cfg config) io.ReadWriteCloser {
	ctx := context.Background()
	dialer := webrtc.NewDialer(cfg.signalingURL)
	dialer.ICEServers = webrtc.ICEServers(cfg.iceServers)

	if cfg.host {
		log.Printf("hosting lobby %q, waiting for a player to join...", cfg.lobbyID)
		lobby, err := dialer.HostLobbyWithID(ctx, cfg.lobbyID)
		if err != nil {
			log.Fatalf("hosting lobby: %s", err)
		}
		channel, playerID, err := lobby.Accept(ctx)
		if err != nil {
			log.Fatalf("accepting player: %s", err)
		}
		log.Printf("player %d connected", playerID)
		if err := lobby.Delete(ctx); err != nil {
			log.Printf("deleting lobby: %s", err)
		}
		return channel
	}

	log.Printf("joining lobby %q...", cfg.lobbyID)
	channel, playerID, err := dialer.Join(ctx, cfg.lobbyID)
	if err != nil {
		log.Fatalf("joining lobby: %s", err)
	}
	log.Printf("joined as player %d", playerID)
	return channel
}

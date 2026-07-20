package main

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ikemen-engine/ggpo"
	"github.com/ikemen-engine/ggpo/example/game"
	"github.com/ikemen-engine/ggpo/transport/udp"
)

type peerAddress struct {
	ip   string
	port int
}

// remotePeer is a player handle together with the UDP address to register it
// under.
type remotePeer struct {
	handle ggpo.PlayerHandle
	ip     string
	port   int
}

func getPeerAddress(address string) peerAddress {
	peerIPSlice := strings.Split(address, ":")
	if len(peerIPSlice) < 2 {
		panic("Please enter IP as ip:port")
	}
	peerPort, err := strconv.Atoi(peerIPSlice[1])
	if err != nil {
		panic("Please enter integer port")
	}
	return peerAddress{
		ip:   peerIPSlice[0],
		port: peerPort,
	}
}

func main() {
	argsWithoutProg := os.Args[1:]
	if len(argsWithoutProg) < 4 {
		panic("Must enter <port> <num players> ('local' |IP adress) ('local' |IP adress) currentPlayer or <port> <num players> spectate <host ip>:<host port>")
	}
	localPort, err := strconv.Atoi(argsWithoutProg[0])
	if err != nil {
		panic("Plase enter integer port")
	}

	numPlayers, err := strconv.Atoi(argsWithoutProg[1])
	if err != nil {
		panic("Please enter integer numPlayers")
	}

	var g *game.Game
	if argsWithoutProg[2] == "spectate" {
		hostAddress := getPeerAddress(argsWithoutProg[3])
		g = udpInitSpectator(localPort, numPlayers, hostAddress)
	} else {
		ipAddress := []string{argsWithoutProg[2], argsWithoutProg[3]}

		currentPlayer, err := strconv.Atoi(argsWithoutProg[4])
		if err != nil {
			panic("Please enter integer currentPlayer")
		}

		players := make([]ggpo.Player, ggpo.MaxPlayers+ggpo.MaxSpectators)
		var remotePeers []remotePeer
		var i int
		for i = 0; i < numPlayers; i++ {
			// the handle for each remote player is simply their player number
			handle := ggpo.PlayerHandle(i + 1)
			if ipAddress[i] == "local" {
				players[i] = ggpo.NewLocalPlayer(20, i+1)
			} else {
				remoteAddress := getPeerAddress(ipAddress[i])
				players[i] = ggpo.NewRemotePlayer(20, i+1, handle)
				remotePeers = append(remotePeers, remotePeer{handle: handle, ip: remoteAddress.ip, port: remoteAddress.port})
			}
		}

		offset := 5
		numSpectators := 0
		for offset < len(argsWithoutProg) {
			remoteAddress := getPeerAddress(argsWithoutProg[offset])
			// spectator handles start at 1000 to stay clear of player numbers
			handle := ggpo.PlayerHandle(1000 + numSpectators)
			players[i] = ggpo.NewSpectatorPlayer(20, handle)
			remotePeers = append(remotePeers, remotePeer{handle: handle, ip: remoteAddress.ip, port: remoteAddress.port})
			numSpectators++
			i++
			offset++
		}
		game.SetCurrentPlayer(currentPlayer)
		g = udpInit(localPort, numPlayers, players, numSpectators, remotePeers)
	}

	game.StartClock()
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}

func udpInitSpectator(localPort int, numPlayers int, host peerAddress) *game.Game {
	session := game.NewGameSession()

	hostHandle := ggpo.PlayerHandle(1)
	spectator := ggpo.NewSpectator(&session, numPlayers, game.InputSize(), hostHandle)
	game.SetBackend(&spectator)
	session.SetBackend(&spectator)

	tr := udp.NewUdp(localPort)
	tr.AddPeer(hostHandle, host.ip, host.port)
	spectator.InitializeTransport(tr)
	spectator.Start()

	return session.Game()
}

func udpInit(localPort int, numPlayers int, players []ggpo.Player, numSpectators int, remotePeers []remotePeer) *game.Game {
	session := game.NewGameSession()

	peer := ggpo.NewPeer(&session, numPlayers, game.InputSize())
	game.SetBackend(&peer)
	session.SetBackend(&peer)

	tr := udp.NewUdp(localPort)
	for _, rp := range remotePeers {
		tr.AddPeer(rp.handle, rp.ip, rp.port)
	}
	peer.InitializeTransport(tr)

	var localHandle ggpo.PlayerHandle
	for i := 0; i < numPlayers+numSpectators; i++ {
		var handle ggpo.PlayerHandle
		if err := peer.AddPlayer(&players[i], &handle); err != nil {
			log.Fatalf("There's an issue from AddPlayer")
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
	return session.Game()
}

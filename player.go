package ggpo

import "github.com/ikemen-engine/ggpo/transport"

// PlayerHandle identifies a player in the session.
// It is an alias of transport.PlayerHandle:
// the same handle addresses the player's traffic in whatever Transport the session uses.
type PlayerHandle = transport.PlayerHandle

type PlayerType int

const (
	PlayerTypeLocal PlayerType = iota
	PlayerTypeRemote
	PlayerTypeSpectator
)

const InvalidHandle int = -1

type Player struct {
	Size       int
	PlayerType PlayerType
	PlayerNum  int
	Remote     RemotePlayer
}

func NewLocalPlayer(size int, playerNum int) Player {
	return Player{
		Size:       size,
		PlayerNum:  playerNum,
		PlayerType: PlayerTypeLocal}
}

func NewRemotePlayer(size int, playerNum int, handle PlayerHandle) Player {
	return Player{
		Size:       size,
		PlayerNum:  playerNum,
		PlayerType: PlayerTypeRemote,
		Remote: RemotePlayer{
			Handle: handle},
	}
}
func NewSpectatorPlayer(size int, handle PlayerHandle) Player {
	return Player{
		Size:       size,
		PlayerType: PlayerTypeSpectator,
		Remote: RemotePlayer{
			Handle: handle},
	}
}

// RemotePlayer identifies a remote player by the handle their transport
// traffic is addressed with:
// the lobby-assigned handle for WebRTC,
// or the handle their address was registered under for UDP (udp.Udp.AddPeer).
type RemotePlayer struct {
	Handle PlayerHandle
}

type LocalEndpoint struct {
	playerNum int
}

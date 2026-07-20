// Package transport defines the wire format and transport contract shared by every GGPO network backend.
// A concrete backend (see transport/udp and transport/webrtc) serializes the same Message packets and satisfies Transport;
// the protocol layer talks only to these interfaces, so it is
// agnostic to whether packets travel over UDP sockets or WebRTC data channels.
package transport

// PlayerHandle identifies a player in the session. Transports address remote
// peers by the handle of the player they belong to: a lobby-based backend
// (e.g. WebRTC) uses the handle the lobby assigned to the player, while the
// UDP backend maps each handle to an ip:port registered with it up front.
// The root ggpo package aliases this type as ggpo.PlayerHandle.
type PlayerHandle int

// Transport delivers GGPO packets between the local session and its remote
// peers.
type Transport interface {
	// SendTo serializes msg and delivers it to the given player.
	SendTo(msg Message, player PlayerHandle)
	// Read blocks, decoding inbound packets and pushing them onto messageChan
	// tagged with the sending player's handle. Run it in its own goroutine.
	Read(messageChan chan MessageChannelItem)
	Close()
}

// MessageHandler receives a decoded packet along with the handle of the player
// it came from.
type MessageHandler interface {
	HandleMessage(player PlayerHandle, msg Message, length int)
}

// MessageChannelItem is one decoded inbound packet and its origin, as delivered
// over the channel passed to Transport.Read.
type MessageChannelItem struct {
	Player  PlayerHandle
	Message Message
	Length  int
}

type Stats struct {
	BytesSent   int
	PacketsSent int
	KbpsSent    float64
}

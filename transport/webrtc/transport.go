package webrtc

import (
	"io"
	"sync"

	"github.com/ikemen-engine/ggpo/transport"
)

// maxMessageSize bounds a single decoded packet. WebRTC data channels are
// message-oriented, so one Read yields one whole packet; this only needs to be
// large enough for the biggest GGPO packet.
const maxMessageSize = 4096

// Transport adapts a set of WebRTC data channels to transport.Transport, so
// the GGPO protocol layer can drive a WebRTC session exactly as it drives UDP.
//
// Peers are identified by the player handle the signaling server assigned in
// the lobby: register each established channel under that handle with AddPeer,
// and use the same handle when adding the player to the session
// (Backend.AddPlayer). A typical host flow:
//
//	tr := webrtc.NewTransport()
//	backend.InitializeTransport(tr) // protocol layer will call tr.Read
//	ch, player, _ := lobby.Accept(ctx)
//	tr.AddPeer(player, ch)
//	backend.AddPlayer(&Player{Remote: RemotePlayer{Handle: player}, ...}, &handle)
type Transport struct {
	mu        sync.Mutex
	peers     map[transport.PlayerHandle]*peer
	msgChan   chan transport.MessageChannelItem
	done      chan struct{}
	closeOnce sync.Once
}

type peer struct {
	channel io.ReadWriteCloser
	handle  transport.PlayerHandle
	reading bool
}

func NewTransport() *Transport {
	return &Transport{
		peers: make(map[transport.PlayerHandle]*peer),
		done:  make(chan struct{}),
	}
}

// AddPeer registers an established data channel under the peer's lobby-assigned
// player handle. It is safe to call before or after Read: a reader goroutine
// starts as soon as both the channel and the destination for decoded messages
// (set by Read) are known.
func (t *Transport) AddPeer(player transport.PlayerHandle, channel io.ReadWriteCloser) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := &peer{channel: channel, handle: player}
	t.peers[player] = p
	if t.msgChan != nil {
		p.reading = true
		go t.readPeer(p)
	}
}

// SendTo serializes msg and writes it to the data channel registered for the
// peer. Sends to an unregistered peer are dropped, mirroring a UDP send to an
// unreachable address.
func (t *Transport) SendTo(msg transport.Message, player transport.PlayerHandle) {
	if msg == nil {
		return
	}
	t.mu.Lock()
	p, ok := t.peers[player]
	t.mu.Unlock()
	if !ok {
		return
	}
	p.channel.Write(msg.ToBytes())
}

// Read records where decoded messages should be delivered, starts a reader for
// every peer registered so far, then blocks until Close. Run it in its own
// goroutine, the same way udp.Udp.Read is used.
func (t *Transport) Read(messageChan chan transport.MessageChannelItem) {
	t.mu.Lock()
	t.msgChan = messageChan
	for _, p := range t.peers {
		if !p.reading {
			p.reading = true
			go t.readPeer(p)
		}
	}
	t.mu.Unlock()
	<-t.done
}

// readPeer reads whole packets from one data channel until it closes, decoding
// each and forwarding it tagged with the peer's player handle.
func (t *Transport) readPeer(p *peer) {
	buf := make([]byte, maxMessageSize)
	for {
		n, err := p.channel.Read(buf)
		if err != nil {
			return // channel closed
		}
		if n <= 0 {
			continue
		}
		msg, err := transport.DecodeMessageBinary(buf[:n])
		if err != nil {
			continue // drop malformed packets, as UDP would
		}
		select {
		case t.msgChan <- transport.MessageChannelItem{
			Player:  p.handle,
			Message: msg,
			Length:  n,
		}:
		case <-t.done:
			return
		}
	}
}

// Close tears down every registered data channel and unblocks Read.
func (t *Transport) Close() {
	t.closeOnce.Do(func() {
		close(t.done)
		t.mu.Lock()
		for _, p := range t.peers {
			p.channel.Close()
		}
		t.mu.Unlock()
	})
}

package mocks

import (
	"fmt"

	"github.com/ikemen-engine/ggpo/transport"
)

// FakeTransport records outbound messages per player handle and delivers nothing.
type FakeTransport struct {
	SendMap         map[transport.PlayerHandle][]transport.Message
	LastSentMessage transport.Message
}

func NewFakeTransport() FakeTransport {
	return FakeTransport{
		SendMap: make(map[transport.PlayerHandle][]transport.Message),
	}
}

func (f *FakeTransport) SendTo(msg transport.Message, player transport.PlayerHandle) {
	f.SendMap[player] = append(f.SendMap[player], msg)
	f.LastSentMessage = msg
}

func (f *FakeTransport) Read(messageChan chan transport.MessageChannelItem) {

}

func (f *FakeTransport) Close() {

}

// FakeP2PTransport hands every outbound message straight to the remote peer's
// message handler, tagged with localHandle: the player handle the remote side has
// registered for us.
type FakeP2PTransport struct {
	remoteHandler   transport.MessageHandler
	localHandle     transport.PlayerHandle
	printOutput     bool
	LastSentMessage transport.Message
	MessageHistory  []transport.Message
}

func (f *FakeP2PTransport) SendTo(msg transport.Message, player transport.PlayerHandle) {
	if f.printOutput {
		fmt.Printf("f.localHandle %d msg %s size %d\n", f.localHandle, msg, msg.PacketSize())
	}
	f.LastSentMessage = msg
	f.MessageHistory = append(f.MessageHistory, msg)
	f.remoteHandler.HandleMessage(f.localHandle, msg, msg.PacketSize())
}

func (f *FakeP2PTransport) Read(messageChan chan transport.MessageChannelItem) {
}

func (f *FakeP2PTransport) Close() {

}

func NewFakeP2PTransport(remoteHandler transport.MessageHandler, localHandle transport.PlayerHandle) FakeP2PTransport {
	f := FakeP2PTransport{}
	f.remoteHandler = remoteHandler
	f.localHandle = localHandle
	f.MessageHistory = make([]transport.Message, 10)
	return f
}

// FakeMultiplePeerTransport broadcasts every outbound message to all remote
// handlers, tagged with localHandle.
type FakeMultiplePeerTransport struct {
	remoteHandler []transport.MessageHandler
	localHandle   transport.PlayerHandle
	printOutput   bool
}

func (f *FakeMultiplePeerTransport) SendTo(msg transport.Message, player transport.PlayerHandle) {
	if f.printOutput {
		fmt.Printf("f.localHandle %d msg %s size %d\n", f.localHandle, msg, msg.PacketSize())
	}
	for _, r := range f.remoteHandler {
		r.HandleMessage(f.localHandle, msg, msg.PacketSize())
	}
}

func (f *FakeMultiplePeerTransport) Read(messageChan chan transport.MessageChannelItem) {
}

func (f *FakeMultiplePeerTransport) Close() {

}

func NewFakeMultiplePeerTransport(remoteHandler []transport.MessageHandler, localHandle transport.PlayerHandle) FakeMultiplePeerTransport {
	f := FakeMultiplePeerTransport{}
	f.remoteHandler = remoteHandler
	f.localHandle = localHandle
	return f
}

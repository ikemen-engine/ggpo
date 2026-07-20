package mocks

import (
	"github.com/ikemen-engine/ggpo/internal/protocol"
	"github.com/ikemen-engine/ggpo/transport"
)

type FakeMessageHandler struct {
	Endpoint *protocol.Protocol
}

func (f *FakeMessageHandler) HandleMessage(player transport.PlayerHandle, msg transport.Message, length int) {
	if f.Endpoint.HandlesMsg(player) {
		f.Endpoint.OnMsg(msg, length)
	}
}

func NewFakeMessageHandler(endpoint *protocol.Protocol) FakeMessageHandler {
	f := FakeMessageHandler{}
	f.Endpoint = endpoint
	return f
}

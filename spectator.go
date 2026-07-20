package ggpo

import (
	"github.com/ikemen-engine/ggpo/internal/input"
	"github.com/ikemen-engine/ggpo/internal/polling"
	"github.com/ikemen-engine/ggpo/internal/protocol"
	"github.com/ikemen-engine/ggpo/internal/util"
	"github.com/ikemen-engine/ggpo/transport"
)

const SpectatorFrameBufferSize int = 32
const DefaultMaxFramesBehind int = 10
const DefaultCatchupSpeed int = 1

type Spectator struct {
	session         Session
	poll            polling.Poller
	transport       transport.Transport
	host            protocol.Protocol
	synchonizing    bool
	inputSize       int
	numPlayers      int
	nextInputToSend int
	inputs          []input.GameInput
	hostHandle      PlayerHandle
	framesBehind    int
	currentFrame    int
	messageChannel  chan transport.MessageChannelItem
}

// NewSpectator creates a spectator session that watches the host identified by
// hostHandle, the player handle the host is registered under in the transport
// passed to InitializeTransport.
func NewSpectator(cb Session, numPlayers int, inputSize int, hostHandle PlayerHandle) Spectator {
	s := Spectator{}
	s.numPlayers = numPlayers
	s.inputSize = inputSize
	s.nextInputToSend = 0

	s.session = cb
	s.synchonizing = true

	inputs := make([]input.GameInput, SpectatorFrameBufferSize)
	for _, i := range inputs {
		i.Frame = -1
	}
	s.inputs = inputs
	s.hostHandle = hostHandle
	var poll polling.Poll = polling.NewPoll()
	s.poll = &poll
	s.messageChannel = make(chan transport.MessageChannelItem, 200)
	return s
}

func (s *Spectator) Idle(timeout int, timeFunc ...polling.FuncTimeType) error {
	s.HandleMessages()
	if len(timeFunc) == 0 {
		s.poll.Pump()
	} else {
		s.poll.Pump(timeFunc[0])
	}
	s.PollProtocolEvents()

	if s.framesBehind > 0 {
		for s.nextInputToSend < s.currentFrame {
			s.session.AdvanceFrame(0)
			util.Log.Printf("In Spectator: skipping frame %d\n", s.nextInputToSend)
			s.nextInputToSend++
		}
		s.framesBehind = 0
	}

	return nil
}

func (s *Spectator) SyncInput(disconnectFlags *int) ([][]byte, error) {
	// Wait until we've started to return inputs
	if s.synchonizing {
		return nil, Error{Code: ErrorCodeNotSynchronized, Name: "ErrorCodeNotSynchronized"}
	}

	input := s.inputs[s.nextInputToSend%SpectatorFrameBufferSize]
	s.currentFrame = input.Frame
	if input.Frame < s.nextInputToSend {
		// Haved recieved input from the host yet. Wait
		return nil, Error{Code: ErrorCodePredictionThreshod, Name: "ErrorCodePredictionThreshod"}

	}
	if input.Frame > s.nextInputToSend {
		s.framesBehind = input.Frame - s.nextInputToSend
		// The host is way way way far ahead of the spetator. How'd this
		// happen? Any, the input we need is gone forever.
		return nil, Error{Code: ErrorCodeGeneralFailure, Name: "ErrorCodeGeneralFailure"}
	}
	//s.framesBehind = 0

	//Assert(size >= s.inputSize*s.numPlayers)
	values := make([][]byte, s.numPlayers)
	offset := 0
	counter := 0
	for offset < len(input.Bits) {
		values[counter] = input.Bits[offset : s.inputSize+offset]
		offset += s.inputSize
		counter++
	}

	if disconnectFlags != nil {
		*disconnectFlags = 0 // xxx: we should get them from the host! -pond3r
	}
	s.nextInputToSend++
	return values, nil
}

func (s *Spectator) AdvanceFrame(checksum uint32) error {
	util.Log.Printf("End of frame (%d)...\n", s.nextInputToSend-1)
	s.Idle(0)
	s.PollProtocolEvents()

	return nil
}

func (s *Spectator) PollProtocolEvents() {
	for {
		evt, ok := s.host.GetEvent()
		if ok != nil {
			break
		} else {
			s.OnProtocolEvent(evt)
		}
	}
}

func (s *Spectator) OnProtocolEvent(evt *protocol.ProtocolEvent) {
	var info Event
	switch evt.Type() {
	case protocol.ConnectedEvent:
		info.Code = EventCodeConnectedToPeer
		info.Player = 0
		s.session.OnEvent(&info)

	case protocol.SynchronizingEvent:
		info.Code = EventCodeSynchronizingWithPeer
		info.Player = 0
		info.Count = evt.Count
		info.Total = evt.Total
		s.session.OnEvent(&info)

	case protocol.SynchronziedEvent:
		if s.synchonizing {
			info.Code = EventCodeSynchronizedWithPeer
			info.Player = 0
			s.session.OnEvent(&info)

			info.Code = EventCodeRunning
			s.session.OnEvent(&info)
			s.synchonizing = false
		}

	case protocol.NetworkInterruptedEvent:
		info.Code = EventCodeConnectionInterrupted
		info.Player = 0
		info.DisconnectTimeout = evt.DisconnectTimeout
		s.session.OnEvent(&info)

	case protocol.NetworkResumedEvent:
		info.Code = EventCodeConnectionResumed
		info.Player = 0
		s.session.OnEvent(&info)

	case protocol.DisconnectedEvent:
		info.Code = EventCodeDisconnectedFromPeer
		info.Player = 0
		s.session.OnEvent(&info)

	case protocol.InputEvent:
		input := evt.Input

		s.host.SetLocalFrameNumber(input.Frame)
		s.host.SendInputAck()
		s.inputs[input.Frame%SpectatorFrameBufferSize] = input
	}
}

func (s *Spectator) HandleMessage(player PlayerHandle, msg transport.Message, len int) {
	if s.host.HandlesMsg(player) {
		s.host.OnMsg(msg, len)
	}
}

func (p *Spectator) AddLocalInput(player PlayerHandle, values []byte, size int) error {
	return nil
}

func (s *Spectator) AddPlayer(player *Player, handle *PlayerHandle) error {
	return Error{Code: ErrorCodeInvalidRequest, Name: "ErrorCodeInvalidRequest"}
}

// We must 'impliment' these for this to be a true Session
func (s *Spectator) DisconnectPlayer(handle PlayerHandle) error {
	return Error{Code: ErrorCodeInvalidRequest, Name: "ErrorCodeInvalidRequest"}
}
func (s *Spectator) GetNetworkStats(handle PlayerHandle) (protocol.NetworkStats, error) {
	return protocol.NetworkStats{}, Error{Code: ErrorCodeInvalidRequest, Name: "ErrorCodeInvalidRequest"}
}
func (s *Spectator) SetFrameDelay(player PlayerHandle, delay int) error {
	return Error{Code: ErrorCodeInvalidRequest, Name: "ErrorCodeInvalidRequest"}
}
func (s *Spectator) SetDisconnectTimeout(timeout int) error {
	return Error{Code: ErrorCodeInvalidRequest, Name: "ErrorCodeInvalidRequest"}
}
func (s *Spectator) SetDisconnectNotifyStart(timeout int) error {
	return Error{Code: ErrorCodeInvalidRequest, Name: "ErrorCodeInvalidRequest"}
}
func (s *Spectator) Close() error {
	return Error{Code: ErrorCodeInvalidRequest, Name: "ErrorCodeInvalidRequest"}
}
func (s *Spectator) InitializeTransport(t transport.Transport) error {
	s.transport = t
	return nil
}

func (s *Spectator) HandleMessages() {
	for i := 0; i < len(s.messageChannel); i++ {
		mi := <-s.messageChannel
		s.HandleMessage(mi.Player, mi.Message, mi.Length)
	}
}

func (s *Spectator) Start() {
	go s.transport.Read(s.messageChannel)

	s.host = protocol.NewProtocol(s.transport, 0, s.hostHandle, nil)
	s.poll.RegisterLoop(&s.host, nil)
	s.host.Synchronize()

}

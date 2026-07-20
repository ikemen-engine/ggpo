package protocol_test

import (
	"testing"
	"time"

	"github.com/ikemen-engine/ggpo/internal/input"
	"github.com/ikemen-engine/ggpo/internal/mocks"
	"github.com/ikemen-engine/ggpo/internal/polling"
	"github.com/ikemen-engine/ggpo/internal/protocol"
	"github.com/ikemen-engine/ggpo/transport"
)

// Player handles the two test endpoints are registered under.
const (
	testPeerHandle  = 7001
	testLocalHandle = 7000
)

func defaultConnectStatus() []transport.ConnectStatus {
	return []transport.ConnectStatus{
		{Disconnected: false, LastFrame: 20},
		{Disconnected: false, LastFrame: 22},
	}
}

func fourConnectStatus() []transport.ConnectStatus {
	return []transport.ConnectStatus{
		{Disconnected: false, LastFrame: 20},
		{Disconnected: false, LastFrame: 22},
		{Disconnected: false, LastFrame: 20},
		{Disconnected: false, LastFrame: 22},
	}
}

func MakeEndpoint() (*mocks.FakeTransport, protocol.Protocol) {
	return MakeEndpointWithStatus(defaultConnectStatus())
}

func MakeEndpointWithStatus(connectStatus []transport.ConnectStatus) (*mocks.FakeTransport, protocol.Protocol) {
	connection := mocks.NewFakeTransport()
	endpoint := protocol.NewProtocol(&connection, 0, testPeerHandle, &connectStatus)
	return &connection, endpoint
}

func MakeTwoEndpoints(connectStatus []transport.ConnectStatus) (*mocks.FakeP2PTransport, *protocol.Protocol, *mocks.FakeP2PTransport, *protocol.Protocol) {
	f := &mocks.FakeMessageHandler{}
	f2 := &mocks.FakeMessageHandler{}

	connection := mocks.NewFakeP2PTransport(f, testPeerHandle)
	endpoint := protocol.NewProtocol(&connection, 0, testLocalHandle, &connectStatus)

	connection2 := mocks.NewFakeP2PTransport(f2, testLocalHandle)
	endpoint2 := protocol.NewProtocol(&connection2, 0, testPeerHandle, &connectStatus)

	f2.Endpoint = &endpoint
	f.Endpoint = &endpoint2

	//ggpo.EnableLogger()
	endpoint.Synchronize()
	endpoint2.Synchronize()
	return &connection, &endpoint, &connection2, &endpoint2
}

// synchronizeEndpoint drives an endpoint through the full sync-request/reply
// handshake by replaying the required number of sync replies back at it.
func synchronizeEndpoint(connection *mocks.FakeTransport, endpoint *protocol.Protocol) {
	endpoint.Synchronize()
	syncRequest := connection.LastSentMessage.(*transport.SyncRequestPacket)

	syncReply := transport.NewMessage(transport.SyncReplyMsg).(*transport.SyncReplyPacket)
	syncReply.RandomReply = syncRequest.RandomRequest
	for i := 0; i < protocol.NumSyncPackets; i++ {
		endpoint.OnSyncReply(syncReply, syncReply.PacketSize())
		syncRequest = connection.LastSentMessage.(*transport.SyncRequestPacket)
		syncReply.RandomReply = syncRequest.RandomRequest
	}
}

// synchronizeAndDrainEvents completes the handshake and pops the synchronizing /
// synchronized / connected events off the queue, leaving the endpoint ready to
// send game input.
func synchronizeAndDrainEvents(connection *mocks.FakeTransport, endpoint *protocol.Protocol) {
	synchronizeEndpoint(connection, endpoint)
	for i := 0; i < protocol.NumSyncPackets+1; i++ {
		endpoint.GetEvent()
	}
}

// polls the endpoint enough times to emit a heartbeat input packet
// and asserts that it did.
func triggerHeartbeatInput(t *testing.T, connection *mocks.FakeTransport, endpoint *protocol.Protocol) {
	t.Helper()
	heartbeatTriggerInterval := 2
	for i := 0; i < heartbeatTriggerInterval; i++ {
		endpoint.OnLoopPoll(polling.DefaultTime)
	}
	if connection.LastSentMessage.Type() != transport.InputMsg {
		t.Errorf("This expected the OnLoopPoll to send a heartbeat game input")
	}
}

func TestMakeProtocol(t *testing.T) {
	_, endpoint := MakeEndpoint()
	if !endpoint.IsInitialized() {
		t.Errorf("The fake connection wasn't properly saved.")
	}
}

/*
	Sends.
*/
/*
	Characterization dunno why it works this way
*/
func TestProtocolSendInput(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	input := input.GameInput{Bits: []byte{1, 2, 3, 4}}
	endpoint.SendInput(&input)
	msgs, ok := connection.SendMap[endpoint.PeerHandle]
	if ok != true {
		t.Errorf("The message was never sent. ")
	}
	inputPacket := msgs[0].(*transport.InputPacket)
	got := inputPacket.Bits
	if got != nil {
		t.Errorf("expected '%#v' but got '%#v'", nil, got)
	}
}

func TestProtocolSendMultipleInput(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	input := input.GameInput{Size: 4, Bits: []byte{1, 2, 3, 4}}
	numInputs := 8
	for i := 0; i < numInputs; i++ {
		endpoint.SendInput(&input)
	}
	messages, ok := connection.SendMap[endpoint.PeerHandle]
	if ok != true {
		t.Errorf("The messages were never sent. ")
	}
	want := numInputs
	got := len(messages)
	if len(messages) != numInputs {
		t.Errorf("expected '%d' but got '%d'", want, got)
	}
}

func TestProtocolSynchronize(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	endpoint.Synchronize()
	msgs, ok := connection.SendMap[endpoint.PeerHandle]
	if ok != true {
		t.Errorf("The message was not sent. ")
	}

	syncPacket := msgs[0].(*transport.SyncRequestPacket)
	if syncPacket.Header().HeaderType != uint8(transport.SyncRequestMsg) {
		t.Errorf("The message that was sent/recieved wsa not a SyncRequestMessage. ")
	}
}

func TestProtocolSendInputAck(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	endpoint.SendInputAck()
	msgs, ok := connection.SendMap[endpoint.PeerHandle]
	if ok != true {
		t.Errorf("The message was not sent. ")
	}

	inputAckMessage := msgs[0].(*transport.InputAckPacket)
	if inputAckMessage.Header().HeaderType != uint8(transport.InputAckMsg) {
		t.Errorf("The message that was sent/recieved wsa not a SyncRequestMessage. ")
	}
}

func TestProtocolOnQualityReport(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	msg := transport.NewMessage(transport.QualityReportMsg)
	qualityReportPacket := msg.(*transport.QualityReportPacket)
	qualityReportPacket.FrameAdvantage = 6
	qualityReportPacket.Ping = 50
	endpoint.OnQualityReport(qualityReportPacket, qualityReportPacket.PacketSize())
	msgs, ok := connection.SendMap[endpoint.PeerHandle]
	if ok != true {
		t.Errorf("The message was not sent. ")
	}

	qualityReplyPacket := msgs[0].(*transport.QualityReplyPacket)
	if qualityReplyPacket.Header().HeaderType != uint8(transport.QualityReplyMsg) {
		t.Errorf("The message that was sent/recieved wsa not a SyncRequestMessage. ")
	}
}

func TestProtocolOnSyncRequest(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	msg := transport.NewMessage(transport.SyncRequestMsg)
	syncRequestPacket := msg.(*transport.SyncRequestPacket)

	endpoint.OnSyncRequest(syncRequestPacket, syncRequestPacket.PacketSize())

	msgs, ok := connection.SendMap[endpoint.PeerHandle]
	if ok != true {
		t.Errorf("The message was not sent. ")
	}

	syncReplyPacket := msgs[0].(*transport.SyncReplyPacket)
	if syncReplyPacket.Header().HeaderType != uint8(transport.SyncReplyMsg) {
		t.Errorf("The message that was sent/recieved wsa not a SyncRequestMessage. ")
	}
}

func TestProtocolGetPeerConnectStatus(t *testing.T) {
	_, endpoint := MakeEndpoint()
	var frame int32
	want := true
	got := endpoint.GetPeerConnectStatus(0, &frame)

	if want != got {
		t.Errorf("expected '%t' but got '%t'", want, got)
	}

	wantFrame := int32(input.NullFrame)
	gotFrame := frame
	if wantFrame != gotFrame {
		t.Errorf("expected '%d' but got '%d'", wantFrame, gotFrame)
	}

}

func TestProtocolHandlesMessage(t *testing.T) {
	_, endpoint := MakeEndpoint()

	want := true
	got := endpoint.HandlesMsg(testPeerHandle)

	if want != got {
		t.Errorf("expected '%t' but got '%t'", want, got)
	}
}

func TestProtocolHandlesMessageFalse(t *testing.T) {
	_, endpoint := MakeEndpoint()

	want := false
	got := endpoint.HandlesMsg(0)

	if want != got {
		t.Errorf("expected '%t' but got '%t'", want, got)
	}
}

func TestProtocolSetLocalFrameNumber(t *testing.T) {
	_, endpoint := MakeEndpoint()

	endpoint.SetLocalFrameNumber(8)
	stats := endpoint.GetNetworkStats()
	want := float32(0.000000)
	got := stats.Timesync.LocalFramesBehind
	if want != got {
		t.Errorf("expected '%f' but got '%f'", want, got)
	}
}

func TestProtocolOnQualityReply(t *testing.T) {
	_, endpoint := MakeEndpoint()
	msg := transport.NewMessage(transport.QualityReplyMsg)
	qualityReplyPacket := msg.(*transport.QualityReplyPacket)
	qualityReplyPacket.Pong = 0
	var checkInterval int64 = 60
	endpoint.OnQualityReply(qualityReplyPacket, qualityReplyPacket.PacketSize())

	stats := endpoint.GetNetworkStats()
	want := -9.0 //
	got := stats.Timesync.LocalFramesBehind
	now := time.Now().UnixMilli()
	if now-checkInterval > stats.Network.Ping {
		t.Errorf("expected '%f' but got '%f'", want, got)
	}
}

func TestProtocolQueEventPanic(t *testing.T) {
	_, endpoint := MakeEndpoint()
	event := protocol.ProtocolEvent{}
	capcity := 64
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic when QueEvent attempted to add an event higher than the capacity.")
		}
	}()
	for i := 0; i < capcity+1; i++ {
		endpoint.QueueEvent(&event)
	}
}

func TestProtocolGetEventError(t *testing.T) {
	_, endpoint := MakeEndpoint()
	_, err := endpoint.GetEvent()
	if err == nil {
		t.Errorf("The program did not return an error when trying to get an event from an empty event queue.")
	}
}

func TestProtocolGetEvent(t *testing.T) {
	_, endpoint := MakeEndpoint()
	want := protocol.ProtocolEvent{}
	endpoint.QueueEvent(&want)
	got, _ := endpoint.GetEvent()
	if want.String() != got.String() {
		t.Errorf("expected '%s' but got '%s'", want, got)
	}
}

func TestProtocolSyncchronize(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	synchronizeEndpoint(connection, &endpoint)

	evt, err := endpoint.GetEvent()
	if err != nil {
		t.Errorf("Got an error when should be recievving events")
	}

	if evt.Type() != protocol.ConnectedEvent {
		t.Errorf("First popped event should be connected.")
	}
	for i := 0; i < protocol.NumSyncPackets; i++ {
		evt, err := endpoint.GetEvent()
		if err != nil {
			t.Errorf("Got an error when should be recievving events")
		}
		if i >= 0 && i < protocol.NumSyncPackets-1 {
			if evt.Type() != protocol.SynchronizingEvent {
				t.Errorf("These should be Synchronizing Events.")
			}
		} else if i == protocol.NumSyncPackets-1 {
			if evt.Type() != protocol.SynchronziedEvent {
				t.Errorf("This should be a Synchronized Event")
			}
		}
	}

}

func TestProtocolOnLoopPoll(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	synchronizeAndDrainEvents(connection, &endpoint)

	endpoint.OnLoopPoll(polling.DefaultTime)
	if connection.LastSentMessage.Type() != transport.QualityReportMsg {
		t.Errorf("This expected the OnLoopPoll to send a quality report message")
	}

}

func TestProtocolHeartbeatGameInput(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	synchronizeAndDrainEvents(connection, &endpoint)
	triggerHeartbeatInput(t, connection, &endpoint)
}

func TestProtocolOnInputDefaultPanic(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	synchronizeAndDrainEvents(connection, &endpoint)
	triggerHeartbeatInput(t, connection, &endpoint)

	msg := transport.NewMessage(transport.InputMsg)
	inputPacket := msg.(*transport.InputPacket)
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic when OnInput recieved a completely empty input packet.")
		}
	}()
	endpoint.OnInput(inputPacket, inputPacket.PacketSize())
}

func TestProtocolOnInputPanicWithNonEqualConnectStatus(t *testing.T) {
	connectStatus := defaultConnectStatus()
	connection, endpoint := MakeEndpointWithStatus(connectStatus)
	synchronizeAndDrainEvents(connection, &endpoint)
	triggerHeartbeatInput(t, connection, &endpoint)

	msg := transport.NewMessage(transport.InputMsg)
	inputPacket := msg.(*transport.InputPacket)
	inputPacket.PeerConnectStatus = connectStatus
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic when OnInput recieved a packet with number of connect statuses not equal to its own.")
		}
	}()
	endpoint.OnInput(inputPacket, inputPacket.PacketSize())
}

func TestProtocolOnInputAfterSynchronizeCharacterization(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	synchronizeAndDrainEvents(connection, &endpoint)
	triggerHeartbeatInput(t, connection, &endpoint)

	msg := transport.NewMessage(transport.InputMsg)
	inputPacket := msg.(*transport.InputPacket)
	inputPacket.PeerConnectStatus = make([]transport.ConnectStatus, 4)
	inputPacket.Bits = []byte{1, 2, 3, 4}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic when OnInput recieved a packet without its imput size set")
		}
	}()
	endpoint.OnInput(inputPacket, inputPacket.PacketSize())
}

func TestProtocolOnInputAfterSynchronize(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	synchronizeAndDrainEvents(connection, &endpoint)
	triggerHeartbeatInput(t, connection, &endpoint)

	msg := transport.NewMessage(transport.InputMsg)
	inputPacket := msg.(*transport.InputPacket)
	inputPacket.PeerConnectStatus = make([]transport.ConnectStatus, 4)
	inputPacket.Bits = []byte{1, 2, 3, 4}
	inputPacket.InputSize = 4
	endpoint.OnInput(inputPacket, inputPacket.PacketSize())
	evt, err := endpoint.GetEvent()
	if err != nil {
		t.Errorf("Expected there to be game input event, not error.")
	}
	if evt.Type() != protocol.InputEvent {
		t.Errorf("Expected the event to be InputEvent, not %s", evt)
	}
}

func TestProtocolFakeP2PandMessageHandler(t *testing.T) {
	_, endpoint, _, endpoint2 := MakeTwoEndpoints(defaultConnectStatus())

	advance := func() int64 {
		return time.Now().Add(time.Millisecond * 11000).UnixMilli()
	}
	for !endpoint2.IsSynchronized() {
		endpoint2.OnLoopPoll(advance)
	}

	if !endpoint.IsSynchronized() {
		t.Errorf("First endpoint never synchronized.")
	}

	if !endpoint2.IsSynchronized() {
		t.Errorf("Second endpoint never synchronized.")
	}
}

func TestProtocolDiscconect(t *testing.T) {
	_, endpoint := MakeEndpoint()
	endpoint.Disconnect()
	if endpoint.IsRunning() {
		t.Errorf("The endpoint should be disconnected after running the disconnect method.")
	}
}

func TestProtocolDiscconectOnLoopPoll(t *testing.T) {
	_, endpoint := MakeEndpoint()
	endpoint.Disconnect()
	advance := func() int64 {
		return time.Now().Add(time.Millisecond * 8000).UnixMilli()
	}

	endpoint.OnLoopPoll(advance)
	if endpoint.IsInitialized() {
		t.Errorf("Disconnected endpoints should not still be 'initalized' after they've been polled. ")
	}
}

func TestProtocolOnInputDisconnectedRequest(t *testing.T) {
	_, endpoint := MakeEndpointWithStatus(fourConnectStatus())
	msg := transport.NewMessage(transport.InputMsg)
	inputPacket := msg.(*transport.InputPacket)
	inputPacket.DisconectRequested = true
	endpoint.OnInput(inputPacket, inputPacket.PacketSize())
	evt, _ := endpoint.GetEvent()
	if evt.Type() != protocol.DisconnectedEvent {
		t.Errorf("Recieving an input with DisconnectRequested = true should create a DisconnectedEvent")
	}
}

func TestProtocolIsInitalized(t *testing.T) {
	connectStatus := defaultConnectStatus()
	endpoint := protocol.NewProtocol(nil, 0, testPeerHandle, &connectStatus)
	if endpoint.IsInitialized() {
		t.Errorf("The endpoint should not be initialized if connection is nil.")
	}
}

func TestProtocolOnInvalid(t *testing.T) {
	_, endpoint := MakeEndpoint()
	invalidMessageType := 88
	msg := transport.NewMessage(transport.MessageType(invalidMessageType))
	endpoint.OnMsg(msg, msg.PacketSize())
	handled, err := endpoint.OnInvalid(msg, msg.PacketSize())
	if handled == true {
		t.Errorf("Invalid messasge shouldn't be handled")
	}
	if err == nil {
		t.Errorf("Invalid message should create an error")
	}
}

func TestProtocolSendPendingOutputDefault(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	endpoint.SendPendingOutput()
	msg := connection.LastSentMessage
	inputPacket := msg.(*transport.InputPacket)
	if inputPacket.StartFrame != 0 {
		t.Errorf("Inputs sent when there's no pending output should have startframe 0 ")
	}
	if inputPacket.InputSize != 0 {
		t.Errorf("Inputs sent when there's no pending output should have Input size 0 ")
	}
}

func TestProtocolSequenceNumberReject(t *testing.T) {
	connection, endpoint := MakeEndpoint()
	msg := transport.NewMessage(transport.QualityReportMsg)
	msg.SetHeader(0, protocol.MaxSeqDistance+1)
	endpoint.OnMsg(msg, msg.PacketSize())
	if connection.LastSentMessage != nil {
		t.Errorf("No messages should have been sent in response to the quality report message because of the sequence number. ")
	}
}

func TestProtocolKeepAlive(t *testing.T) {
	connection, endpoint, _, endpoint2 := MakeTwoEndpoints(defaultConnectStatus())

	advance := func() int64 {
		return time.Now().Add(time.Millisecond * 11000).UnixMilli()
	}
	for !endpoint2.IsSynchronized() {
		endpoint2.OnLoopPoll(advance)
	}

	endpoint.OnLoopPoll(advance)
	endpoint.OnLoopPoll(advance)
	if connection.LastSentMessage.Header().HeaderType != uint8(transport.KeepAliveMsg) {
		t.Errorf("Endpoint should've sent keep alive packet.")
	}
}

func TestProtocolHeartBeatCharacterization(t *testing.T) {
	_, _, _, endpoint2 := MakeTwoEndpoints(defaultConnectStatus())

	advance := func() int64 {
		return time.Now().Add(time.Millisecond * 1000).UnixMilli()
	}
	for !endpoint2.IsSynchronized() {
		endpoint2.OnLoopPoll(advance)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic when OnInput recieved a connection status with length < 4")
		}
	}()

	for i := 0; i < 10; i++ {
		endpoint2.OnLoopPoll(advance)
	}
}

func TestProtocolHeartBeat(t *testing.T) {
	connection, endpoint, connection2, endpoint2 := MakeTwoEndpoints(fourConnectStatus())

	advance := func() int64 {
		return time.Now().Add(time.Millisecond * 1000).UnixMilli()
	}
	for !endpoint2.IsSynchronized() {
		endpoint2.OnLoopPoll(advance)
	}

	for i := 0; i < 20; i++ {
		endpoint2.OnLoopPoll(advance)
		endpoint.OnLoopPoll(advance)
	}
	e1k := connection.MessageHistory[len(connection.MessageHistory)-1]
	e1i := connection.MessageHistory[len(connection.MessageHistory)-2]
	e2k := connection2.MessageHistory[len(connection2.MessageHistory)-1]
	e2i := connection2.MessageHistory[len(connection2.MessageHistory)-2]
	if e1k.Header().HeaderType != uint8(transport.KeepAliveMsg) {
		t.Errorf("Endpoint 1 should've sent a keep alive msg")
	}

	if e1i.Header().HeaderType != uint8(transport.InputMsg) {
		t.Errorf("Endpoint 1 should've sent a heartbeat input prior to the keep alive message")
	}

	if e2k.Header().HeaderType != uint8(transport.KeepAliveMsg) {
		t.Errorf("Endpoint 2 should've sent a keep alive msg")
	}

	if e2i.Header().HeaderType != uint8(transport.InputMsg) {
		t.Errorf("Endpoint 2 should've sent a heartbeat input prior to the keep alive message")
	}

}

package webrtc

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/ikemen-engine/ggpo/transport"
)

// Transport must satisfy the connection contract the protocol layer expects.
var _ transport.Transport = (*Transport)(nil)

// fakeChannel is a message-oriented io.ReadWriteCloser standing in for a
// detached WebRTC data channel: each Write becomes exactly one Read.
type fakeChannel struct {
	in        chan []byte // messages to hand back from Read
	out       chan []byte // messages captured from Write
	closed    chan struct{}
	closeOnce sync.Once
}

func newFakeChannel() *fakeChannel {
	return &fakeChannel{
		in:     make(chan []byte, 16),
		out:    make(chan []byte, 16),
		closed: make(chan struct{}),
	}
}

func (f *fakeChannel) Read(p []byte) (int, error) {
	select {
	case b := <-f.in:
		return copy(p, b), nil
	case <-f.closed:
		return 0, io.EOF
	}
}

func (f *fakeChannel) Write(p []byte) (int, error) {
	b := append([]byte(nil), p...)
	select {
	case f.out <- b:
		return len(p), nil
	case <-f.closed:
		return 0, io.ErrClosedPipe
	}
}

func (f *fakeChannel) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

func syncRequest(random uint32) transport.Message {
	msg := transport.NewMessage(transport.SyncRequestMsg).(*transport.SyncRequestPacket)
	msg.RandomRequest = random
	return msg
}

func TestSendToRoutesToRegisteredPeer(t *testing.T) {
	tr := NewTransport()
	a, b := newFakeChannel(), newFakeChannel()
	tr.AddPeer(1, a)
	tr.AddPeer(2, b)

	tr.SendTo(syncRequest(42), 1)

	select {
	case raw := <-a.out:
		msg, err := transport.DecodeMessageBinary(raw)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		got, ok := msg.(*transport.SyncRequestPacket)
		if !ok || got.RandomRequest != 42 {
			t.Errorf("peer a got %#v, want SyncRequest{RandomRequest:42}", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("peer a received nothing")
	}

	select {
	case <-b.out:
		t.Error("peer b should not have received a message")
	default:
	}
}

func TestSendToUnknownPeerIsDropped(t *testing.T) {
	tr := NewTransport()
	a := newFakeChannel()
	tr.AddPeer(1, a)

	tr.SendTo(syncRequest(1), 9) // no such peer

	select {
	case <-a.out:
		t.Error("message delivered to the wrong peer")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestReadDecodesInboundTaggedWithPeer(t *testing.T) {
	tr := NewTransport()
	a := newFakeChannel()
	tr.AddPeer(1, a)

	msgChan := make(chan transport.MessageChannelItem, 1)
	go tr.Read(msgChan)
	defer tr.Close()

	a.in <- syncRequest(7).ToBytes()

	select {
	case item := <-msgChan:
		if item.Player != 1 {
			t.Errorf("player = %d, want 1", item.Player)
		}
		got, ok := item.Message.(*transport.SyncRequestPacket)
		if !ok || got.RandomRequest != 7 {
			t.Errorf("message = %#v, want SyncRequest{RandomRequest:7}", item.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("no inbound message decoded")
	}
}

// TestReadStartsForPeersAddedAfterRead covers the ordering where Read runs
// before any peer is registered (the real flow: the protocol layer starts
// reading, then players join).
func TestReadStartsForPeersAddedAfterRead(t *testing.T) {
	tr := NewTransport()
	msgChan := make(chan transport.MessageChannelItem, 1)
	go tr.Read(msgChan)
	defer tr.Close()

	a := newFakeChannel()
	tr.AddPeer(3, a)
	a.in <- syncRequest(9).ToBytes()

	select {
	case item := <-msgChan:
		if item.Player != 3 {
			t.Errorf("player = %d, want 3", item.Player)
		}
	case <-time.After(time.Second):
		t.Fatal("late-registered peer was never read")
	}
}

func TestCloseUnblocksReadAndClosesChannels(t *testing.T) {
	tr := NewTransport()
	a := newFakeChannel()
	tr.AddPeer(1, a)

	done := make(chan struct{})
	go func() {
		tr.Read(make(chan transport.MessageChannelItem))
		close(done)
	}()

	tr.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Read did not return after Close")
	}
	select {
	case <-a.closed:
	default:
		t.Error("peer channel was not closed")
	}
}

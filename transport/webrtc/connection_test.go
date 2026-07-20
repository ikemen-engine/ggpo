package webrtc

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ikemen-engine/ggpo/signaling"
	"github.com/ikemen-engine/ggpo/transport"
)

// newSignalingServer starts an in-process signaling server and returns a dialer
// pointed at it plus a cleanup func.
func newSignalingServer(t *testing.T) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(signaling.NewServer().Handler())
	return srv.URL, srv.Close
}

func TestHostLobbyWithIDUsesGivenID(t *testing.T) {
	url, stop := newSignalingServer(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	lobby, err := NewDialer(url).HostLobbyWithID(ctx, "my-lobby")
	if err != nil {
		t.Fatalf("HostLobbyWithID: %v", err)
	}
	if lobby.ID != "my-lobby" {
		t.Errorf("lobby ID = %q, want %q", lobby.ID, "my-lobby")
	}
}

func TestHostLobbyWithIDRejectsDuplicate(t *testing.T) {
	url, stop := newSignalingServer(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	d := NewDialer(url)
	if _, err := d.HostLobbyWithID(ctx, "dup"); err != nil {
		t.Fatalf("first HostLobbyWithID: %v", err)
	}
	if _, err := d.HostLobbyWithID(ctx, "dup"); err == nil {
		t.Error("hosting a second lobby with the same id should fail")
	}
}

// TestHostAndJoinExchangeData drives the whole handshake over a real signaling
// server and a real pair of pion peer connections, then checks bytes flow both
// ways across the established data channel.
func TestHostAndJoinExchangeData(t *testing.T) {
	url, stop := newSignalingServer(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const lobbyID = "play"
	lobby, err := NewDialer(url).HostLobbyWithID(ctx, lobbyID)
	if err != nil {
		t.Fatalf("HostLobbyWithID: %v", err)
	}

	type acceptResult struct {
		ch       io.ReadWriteCloser
		playerID transport.PlayerHandle
		err      error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		ch, id, err := lobby.Accept(ctx)
		accepted <- acceptResult{ch, id, err}
	}()

	joinCh, joinID, err := NewDialer(url).Join(ctx, lobbyID)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	defer joinCh.Close()

	res := <-accepted
	if res.err != nil {
		t.Fatalf("Accept: %v", res.err)
	}
	defer res.ch.Close()

	if res.playerID != joinID {
		t.Errorf("host saw player %d, joiner thinks it is %d", res.playerID, joinID)
	}

	// joiner -> host
	assertDelivers(t, joinCh, res.ch, []byte("ping"))
	// host -> joiner
	assertDelivers(t, res.ch, joinCh, []byte("pong"))
}

// assertDelivers writes want to from and fails unless it is read back from to.
// The data channel is unreliable, so it resends until the message arrives or
// the deadline passes; on loopback the first send effectively always lands.
func assertDelivers(t *testing.T, from, to io.ReadWriteCloser, want []byte) {
	t.Helper()

	read := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		n, err := to.Read(buf)
		if err != nil {
			return
		}
		read <- buf[:n]
	}()

	deadline := time.After(10 * time.Second)
	resend := time.NewTicker(200 * time.Millisecond)
	defer resend.Stop()
	for {
		if _, err := from.Write(want); err != nil {
			t.Fatalf("write: %v", err)
		}
		select {
		case got := <-read:
			if string(got) != string(want) {
				t.Errorf("read %q, want %q", got, want)
			}
			return
		case <-resend.C:
		case <-deadline:
			t.Fatalf("message %q never arrived", want)
		}
	}
}

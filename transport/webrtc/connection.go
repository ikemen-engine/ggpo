package webrtc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ikemen-engine/ggpo/signaling"
	"github.com/ikemen-engine/ggpo/transport"
	"github.com/pion/webrtc/v4"
)

const channelLabel = "ggpo"

// Dialer connects to a signaling server to host or join lobbies.
type Dialer struct {
	// SignalingURL is the base URL of the signaling server,
	// e.g. "http://127.0.0.1:3000".
	SignalingURL string
	// ICEServers lists the STUN/TURN servers used for connectivity.
	// Empty means host candidates only, which works on a LAN or localhost.
	// For connections across the internet supply a STUN or TURN server,
	// e.g. stun:stun.l.google.com:19302.
	ICEServers []webrtc.ICEServer
	// PollInterval is how often the signaling server is polled while waiting
	// for offers, answers, and joining players. Defaults to 250ms.
	PollInterval time.Duration
	HTTPClient   *http.Client

	api *webrtc.API
}

func NewDialer(signalingURL string) *Dialer {
	// The detached data channel API is required so channels can be used as io.ReadWriteClosers
	settings := webrtc.SettingEngine{}
	settings.DetachDataChannels()
	return &Dialer{
		SignalingURL: signalingURL,
		PollInterval: 250 * time.Millisecond,
		HTTPClient:   http.DefaultClient,
		api:          webrtc.NewAPI(webrtc.WithSettingEngine(settings)),
	}
}

// ICEServers builds a slice suitable for Dialer.ICEServers from a list of ICE server URLs
// For now, we only allow STUN, and there is only one URL per server. Blank URLs are skipped.
func ICEServers(urls []string) []webrtc.ICEServer {
	servers := make([]webrtc.ICEServer, 0, len(urls))
	for _, u := range urls {
		if u = strings.TrimSpace(u); u != "" {
			servers = append(servers, webrtc.ICEServer{URLs: []string{u}})
		}
	}
	return servers
}

// Lobby is a hosted lobby on the signaling server.
type Lobby struct {
	// ID identifies the lobby; joining players pass it to Dialer.Join.
	ID string

	dialer  *Dialer
	handled map[int]bool
}

// HostLobby creates a new lobby on the signaling server.
func (d *Dialer) HostLobby(ctx context.Context) (*Lobby, error) {
	body, err := d.post(ctx, d.SignalingURL+"/lobby/host", []byte{})
	if err != nil {
		return nil, fmt.Errorf("webrtc: hosting lobby: %w", err)
	}
	return &Lobby{ID: string(body), dialer: d, handled: make(map[int]bool)}, nil
}

// HostLobbyWithID creates a lobby under a caller-chosen id. Peers that agreed
// on an id ahead of time can each connect without first exchanging a generated
// one; the joiners pass the same id to Join. It fails if the id is taken.
func (d *Dialer) HostLobbyWithID(ctx context.Context, id string) (*Lobby, error) {
	body, err := d.post(ctx, d.SignalingURL+"/lobby/hostWithId?id="+url.QueryEscape(id), []byte{})
	if err != nil {
		return nil, fmt.Errorf("webrtc: hosting lobby %q: %w", id, err)
	}
	return &Lobby{ID: string(body), dialer: d, handled: make(map[int]bool)}, nil
}

// Accept waits for the next player to join the lobby, performs the
// offer/answer exchange with them, and returns the established data channel
// along with the player handle the signaling server assigned to them. Call it
// once per expected remote player.
func (l *Lobby) Accept(ctx context.Context) (io.ReadWriteCloser, transport.PlayerHandle, error) {
	d := l.dialer

	playerID, err := l.nextUnregisteredPlayer(ctx)
	if err != nil {
		return nil, 0, err
	}

	pc, err := d.api.NewPeerConnection(webrtc.Configuration{ICEServers: d.ICEServers})
	if err != nil {
		return nil, 0, err
	}

	channelChan := make(chan io.ReadWriteCloser, 1)
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			raw, err := dc.Detach()
			if err != nil {
				return
			}
			channelChan <- raw
		})
	})

	channel, err := d.completeConnection(ctx, pc, l.ID, playerID, channelChan, true)
	if err != nil {
		pc.Close()
		return nil, 0, err
	}
	return channel, transport.PlayerHandle(playerID), nil
}

// Delete removes the lobby from the signaling server. Established connections
// are unaffected; call it once every expected player has been accepted.
func (l *Lobby) Delete(ctx context.Context) error {
	_, err := l.dialer.get(ctx, l.dialer.SignalingURL+"/lobby/delete?id="+l.ID)
	return err
}

// Join joins the lobby with the given ID, performs the offer/answer exchange
// with the host, and returns the established data channel along with the
// player handle the signaling server assigned to us.
func (d *Dialer) Join(ctx context.Context, lobbyID string) (io.ReadWriteCloser, transport.PlayerHandle, error) {
	body, err := d.get(ctx, d.SignalingURL+"/lobby/join?id="+lobbyID)
	if err != nil {
		return nil, 0, fmt.Errorf("webrtcconn: joining lobby %s: %w", lobbyID, err)
	}
	var playerID signaling.PlayerId
	if err := json.Unmarshal(body, &playerID); err != nil {
		return nil, 0, err
	}

	pc, err := d.api.NewPeerConnection(webrtc.Configuration{ICEServers: d.ICEServers})
	if err != nil {
		return nil, 0, err
	}

	// Unordered, no retransmissions: GGPO expects UDP-like delivery and
	// handles loss itself.
	ordered := false
	maxRetransmits := uint16(0)
	dc, err := pc.CreateDataChannel(channelLabel, &webrtc.DataChannelInit{
		Ordered:        &ordered,
		MaxRetransmits: &maxRetransmits,
	})
	if err != nil {
		pc.Close()
		return nil, 0, err
	}

	channelChan := make(chan io.ReadWriteCloser, 1)
	dc.OnOpen(func() {
		raw, err := dc.Detach()
		if err != nil {
			return
		}
		channelChan <- raw
	})

	channel, err := d.completeConnection(ctx, pc, lobbyID, int(playerID), channelChan, false)
	if err != nil {
		pc.Close()
		return nil, 0, err
	}
	return channel, transport.PlayerHandle(playerID), nil
}

// completeConnection runs the SDP exchange for one host/client pair and waits
// for the data channel to open. The host answers the client's offer; the
// client offers and waits for the host's answer.
func (d *Dialer) completeConnection(
	ctx context.Context,
	pc *webrtc.PeerConnection,
	lobbyID string,
	playerID int,
	channelChan chan io.ReadWriteCloser,
	isHost bool,
) (io.ReadWriteCloser, error) {
	player := "?lobby_id=" + lobbyID + "&player_id=" + strconv.Itoa(playerID)

	if isHost {
		offer, err := d.pollSDP(ctx, d.SignalingURL+"/offer/get"+player)
		if err != nil {
			return nil, err
		}
		if err := pc.SetRemoteDescription(offer); err != nil {
			return nil, err
		}
		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			return nil, err
		}
		if err := d.finishLocalDescription(ctx, pc, answer); err != nil {
			return nil, err
		}
		if err := d.postSDP(ctx, d.SignalingURL+"/answer/post"+player, pc.LocalDescription()); err != nil {
			return nil, err
		}
	} else {
		offer, err := pc.CreateOffer(nil)
		if err != nil {
			return nil, err
		}
		if err := d.finishLocalDescription(ctx, pc, offer); err != nil {
			return nil, err
		}
		if err := d.postSDP(ctx, d.SignalingURL+"/offer/post"+player, pc.LocalDescription()); err != nil {
			return nil, err
		}
		answer, err := d.pollSDP(ctx, d.SignalingURL+"/answer/get"+player)
		if err != nil {
			return nil, err
		}
		if err := pc.SetRemoteDescription(answer); err != nil {
			return nil, err
		}
	}

	select {
	case channel := <-channelChan:
		return &peerChannel{ReadWriteCloser: channel, pc: pc}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// finishLocalDescription applies desc and blocks until ICE gathering
// completes, so the description posted to the signaling server contains every
// candidate and no trickle ICE exchange is needed.
func (d *Dialer) finishLocalDescription(ctx context.Context, pc *webrtc.PeerConnection, desc webrtc.SessionDescription) error {
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(desc); err != nil {
		return err
	}
	select {
	case <-gatherComplete:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// nextUnregisteredPlayer polls the lobby until a player this host has not yet
// handled joins.
func (l *Lobby) nextUnregisteredPlayer(ctx context.Context) (int, error) {
	d := l.dialer
	for {
		body, err := d.get(ctx, d.SignalingURL+"/lobby/unregisteredPlayers?id="+l.ID)
		if err == nil {
			var playerIDs []int
			if err := json.Unmarshal(body, &playerIDs); err != nil {
				return 0, err
			}
			for _, id := range playerIDs {
				if !l.handled[id] {
					l.handled[id] = true
					return id, nil
				}
			}
		}
		select {
		case <-time.After(d.PollInterval):
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}

// pollSDP polls url until it returns a session description.
func (d *Dialer) pollSDP(ctx context.Context, url string) (webrtc.SessionDescription, error) {
	var desc webrtc.SessionDescription
	for {
		body, err := d.get(ctx, url)
		if err == nil {
			if err := json.Unmarshal(body, &desc); err != nil {
				return desc, err
			}
			return desc, nil
		}
		select {
		case <-time.After(d.PollInterval):
		case <-ctx.Done():
			return desc, ctx.Err()
		}
	}
}

func (d *Dialer) postSDP(ctx context.Context, url string, desc *webrtc.SessionDescription) error {
	payload, err := json.Marshal(desc)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST %s: unexpected status %s", url, resp.Status)
	}
	return nil
}

func (d *Dialer) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (d *Dialer) post(ctx context.Context, endpoint string, payload []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("POST %s: unexpected status %s", endpoint, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// peerChannel ties the lifetime of the peer connection to the data channel so
// closing the channel tears the whole connection down.
type peerChannel struct {
	io.ReadWriteCloser
	pc *webrtc.PeerConnection
}

func (p *peerChannel) Close() error {
	err := p.ReadWriteCloser.Close()
	if cErr := p.pc.Close(); err == nil {
		err = cErr
	}
	return err
}

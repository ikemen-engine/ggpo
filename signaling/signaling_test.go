package signaling

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/pion/webrtc/v4"
)

// newTestServer spins up the signaling handler on a throwaway HTTP server and
// returns the Server (for white-box assertions) alongside its httptest server.
func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	s := NewServer()
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func doGet(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func doPost(t *testing.T, ts *httptest.Server, path string, body []byte) *http.Response {
	t.Helper()
	resp, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// hostLobby creates a lobby and returns its ID, asserting the response shape.
func hostLobby(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	resp := doPost(t, ts, "/lobby/host", []byte{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("host: status %d, want 200", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("host: read body: %v", err)
	}
	id := string(b)
	if len(id) != lobbyIDLength {
		t.Fatalf("host: lobby id %q has length %d, want %d", id, len(id), lobbyIDLength)
	}
	return id
}

// hostLobby creates a lobby and returns its ID, asserting the response shape.
func hostLobbyWithId(t *testing.T, ts *httptest.Server, id string) string {
	t.Helper()
	request_string := "/lobby/hostWithId?id=" + id
	resp := doPost(t, ts, request_string, []byte{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("host: status %d, want 200", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("host: read body: %v", err)
	}
	returnedId := string(b)
	if id != returnedId {
		t.Fatalf("host: lobby id %q is not returned id %q", id, returnedId)
	}
	return id
}

// joinLobby joins the lobby and returns the assigned player ID.
func joinLobby(t *testing.T, ts *httptest.Server, lobbyID string) int {
	t.Helper()
	resp := doGet(t, ts, "/lobby/join?id="+lobbyID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("join %s: status %d, want 200", lobbyID, resp.StatusCode)
	}
	var playerID int
	if err := json.NewDecoder(resp.Body).Decode(&playerID); err != nil {
		t.Fatalf("join %s: decode player id: %v", lobbyID, err)
	}
	return playerID
}

func sdpPath(resource, op, lobbyID string, playerID int) string {
	return fmt.Sprintf("/%s/%s?lobby_id=%s&player_id=%d", resource, op, lobbyID, playerID)
}

func sdp(kind webrtc.SDPType, body string) webrtc.SessionDescription {
	return webrtc.SessionDescription{Type: kind, SDP: body}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// postSDP posts an offer/answer for a player and asserts it was accepted.
func postSDP(t *testing.T, ts *httptest.Server, resource, lobbyID string, playerID int, desc webrtc.SessionDescription) {
	t.Helper()
	resp := doPost(t, ts, sdpPath(resource, "post", lobbyID, playerID), mustJSON(t, desc))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post %s: status %d, want 200", resource, resp.StatusCode)
	}
}

// getSDP fetches an offer/answer for a player, requiring a 200 and decoding it.
func getSDP(t *testing.T, ts *httptest.Server, resource, lobbyID string, playerID int) webrtc.SessionDescription {
	t.Helper()
	resp := doGet(t, ts, sdpPath(resource, "get", lobbyID, playerID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get %s: status %d, want 200", resource, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("get %s: Content-Type %q, want application/json", resource, ct)
	}
	var desc webrtc.SessionDescription
	if err := json.NewDecoder(resp.Body).Decode(&desc); err != nil {
		t.Fatalf("get %s: decode: %v", resource, err)
	}
	return desc
}

func unregisteredPlayers(t *testing.T, ts *httptest.Server, lobbyID string) []int {
	t.Helper()
	resp := doGet(t, ts, "/lobby/unregisteredPlayers?id="+lobbyID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unregisteredPlayers: status %d, want 200", resp.StatusCode)
	}
	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		t.Fatalf("unregisteredPlayers: decode: %v", err)
	}
	return ids
}

func TestHostReturnsUniqueLobbyIDs(t *testing.T) {
	s, ts := newTestServer(t)

	seen := map[string]bool{}
	const n = 50
	for i := 0; i < n; i++ {
		id := hostLobby(t, ts)
		if seen[id] {
			t.Fatalf("duplicate lobby id %q", id)
		}
		seen[id] = true
	}
	if len(s.lobbies) != n {
		t.Errorf("server holds %d lobbies, want %d", len(s.lobbies), n)
	}
}

func TestHostReturnsCustomLobbyID(t *testing.T) {
	s, ts := newTestServer(t)

	seen := map[string]bool{}
	const n = 50
	for i := 0; i < n; i++ {
		id := hostLobbyWithId(t, ts, strconv.Itoa(i))
		if seen[id] {
			t.Fatalf("duplicate lobby id %q", id)
		}
		seen[id] = true
	}
	if len(s.lobbies) != n {
		t.Errorf("server holds %d lobbies, want %d", len(s.lobbies), n)
	}
}

/*
TODO: add test where we can't use a lobby id that already exists in hostWithId
func TestHostReuseLobbyID(t *testing.T) {
}
*/
func TestJoinAssignsSequentialPlayerIDs(t *testing.T) {
	_, ts := newTestServer(t)
	lobbyID := hostLobby(t, ts) // host is player 0

	for want := 1; want <= 3; want++ {
		if got := joinLobby(t, ts, lobbyID); got != want {
			t.Errorf("join #%d assigned player %d, want %d", want, got, want)
		}
	}
}

func TestJoinNonexistentLobby(t *testing.T) {
	_, ts := newTestServer(t)
	resp := doGet(t, ts, "/lobby/join?id=nope123")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("join missing lobby: status %d, want 404", resp.StatusCode)
	}
}

// TestOfferAnswerExchange walks a full handshake: a joiner posts an offer, the
// host reads it and posts an answer, and the joiner reads the answer back.
func TestOfferAnswerExchange(t *testing.T) {
	_, ts := newTestServer(t)
	lobbyID := hostLobby(t, ts)
	player := joinLobby(t, ts, lobbyID)

	offer := sdp(webrtc.SDPTypeOffer, "v=0\r\no=offer")
	postSDP(t, ts, "offer", lobbyID, player, offer)

	gotOffer := getSDP(t, ts, "offer", lobbyID, player)
	if gotOffer.Type != webrtc.SDPTypeOffer || gotOffer.SDP != offer.SDP {
		t.Errorf("offer round-trip: got %+v, want %+v", gotOffer, offer)
	}

	answer := sdp(webrtc.SDPTypeAnswer, "v=0\r\no=answer")
	postSDP(t, ts, "answer", lobbyID, player, answer)

	gotAnswer := getSDP(t, ts, "answer", lobbyID, player)
	if gotAnswer.Type != webrtc.SDPTypeAnswer || gotAnswer.SDP != answer.SDP {
		t.Errorf("answer round-trip: got %+v, want %+v", gotAnswer, answer)
	}
}

func TestGetOfferBeforePostReturns404(t *testing.T) {
	_, ts := newTestServer(t)
	lobbyID := hostLobby(t, ts)
	player := joinLobby(t, ts, lobbyID)

	resp := doGet(t, ts, sdpPath("offer", "get", lobbyID, player))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get unset offer: status %d, want 404", resp.StatusCode)
	}
}

// TestUnregisteredPlayers verifies the host-facing list of players awaiting an
// answer: it never includes the host, and a player drops off once answered.
func TestUnregisteredPlayers(t *testing.T) {
	_, ts := newTestServer(t)
	lobbyID := hostLobby(t, ts)
	p1 := joinLobby(t, ts, lobbyID)
	p2 := joinLobby(t, ts, lobbyID)
	p3 := joinLobby(t, ts, lobbyID)

	if got := unregisteredPlayers(t, ts, lobbyID); !equalInts(got, []int{p1, p2, p3}) {
		t.Errorf("initial unregistered = %v, want %v", got, []int{p1, p2, p3})
	}

	postSDP(t, ts, "answer", lobbyID, p2, sdp(webrtc.SDPTypeAnswer, "answered"))

	if got := unregisteredPlayers(t, ts, lobbyID); !equalInts(got, []int{p1, p3}) {
		t.Errorf("after answering p2, unregistered = %v, want %v", got, []int{p1, p3})
	}
}

// TestMultipleLobbiesAreIsolated ensures lobbies do not share player numbering
// or stored session descriptions, and that deleting one leaves the other intact.
func TestMultipleLobbiesAreIsolated(t *testing.T) {
	s, ts := newTestServer(t)
	lobbyA := hostLobby(t, ts)
	lobbyB := hostLobby(t, ts)
	if lobbyA == lobbyB {
		t.Fatalf("two hosts got the same lobby id %q", lobbyA)
	}
	if len(s.lobbies) != 2 {
		t.Fatalf("server holds %d lobbies, want 2", len(s.lobbies))
	}

	// Player numbering restarts per lobby.
	pA := joinLobby(t, ts, lobbyA)
	pB := joinLobby(t, ts, lobbyB)
	if pA != 1 || pB != 1 {
		t.Errorf("player ids not per-lobby: A=%d B=%d, want 1 and 1", pA, pB)
	}

	// An offer in A must not be visible in B.
	postSDP(t, ts, "offer", lobbyA, pA, sdp(webrtc.SDPTypeOffer, "offerA"))
	resp := doGet(t, ts, sdpPath("offer", "get", lobbyB, pB))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("offer leaked across lobbies: B get status %d, want 404", resp.StatusCode)
	}

	// Deleting A must not disturb B.
	deleteLobby(t, ts, lobbyA)
	if len(s.lobbies) != 1 {
		t.Errorf("after delete, server holds %d lobbies, want 1", len(s.lobbies))
	}
	// B is unaffected and still accepts joins.
	if got := joinLobby(t, ts, lobbyB); got != 2 {
		t.Errorf("join B after deleting A assigned %d, want 2", got)
	}
}

func deleteLobby(t *testing.T, ts *httptest.Server, lobbyID string) {
	t.Helper()
	resp := doGet(t, ts, "/lobby/delete?id="+lobbyID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete %s: status %d, want 200", lobbyID, resp.StatusCode)
	}
}

func leaveLobby(t *testing.T, ts *httptest.Server, lobbyID string, playerID int) {
	t.Helper()
	resp := doGet(t, ts, fmt.Sprintf("/lobby/leave?lobby_id=%s&player_id=%d", lobbyID, playerID))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("leave %s/%d: status %d, want 200", lobbyID, playerID, resp.StatusCode)
	}
}

// TestLeaveRemovesPlayer verifies a client can leave without disturbing the others:
// it drops off the unregistered list and its stored SDP becomes unreachable,
// while remaining players are untouched.
func TestLeaveRemovesPlayer(t *testing.T) {
	s, ts := newTestServer(t)
	lobbyID := hostLobby(t, ts)
	p1 := joinLobby(t, ts, lobbyID)
	p2 := joinLobby(t, ts, lobbyID)
	postSDP(t, ts, "offer", lobbyID, p1, sdp(webrtc.SDPTypeOffer, "offer1"))

	leaveLobby(t, ts, lobbyID, p1)

	if got := len(s.lobbies[lobbyID].clients); got != 1 {
		t.Errorf("after leave, lobby holds %d clients, want 1", got)
	}
	if got := unregisteredPlayers(t, ts, lobbyID); !equalInts(got, []int{p2}) {
		t.Errorf("after p1 left, unregistered = %v, want %v", got, []int{p2})
	}
	// p1's offer is now unreachable.
	resp := doGet(t, ts, sdpPath("offer", "get", lobbyID, p1))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("get offer for departed player: status %d, want 404", resp.StatusCode)
	}
	// p2 is still fully usable.
	postSDP(t, ts, "answer", lobbyID, p2, sdp(webrtc.SDPTypeAnswer, "answer2"))
	if got := getSDP(t, ts, "answer", lobbyID, p2); got.SDP != "answer2" {
		t.Errorf("p2 answer = %q, want %q", got.SDP, "answer2")
	}
}

// TestLeaveDoesNotReuseIds ensures a departed player's id is not handed to a
// later joiner, so a stale peer cannot collide with a new one.
func TestLeaveDoesNotReuseIds(t *testing.T) {
	_, ts := newTestServer(t)
	lobbyID := hostLobby(t, ts)
	p1 := joinLobby(t, ts, lobbyID)
	p2 := joinLobby(t, ts, lobbyID)

	leaveLobby(t, ts, lobbyID, p1)

	p3 := joinLobby(t, ts, lobbyID)
	if p3 == p1 || p3 == p2 {
		t.Errorf("rejoin id %d collides with existing/departed id (p1=%d, p2=%d)", p3, p1, p2)
	}
	if p3 != 3 {
		t.Errorf("rejoin got id %d, want 3 (ids are monotonic)", p3)
	}
}

func TestLeaveInvalid(t *testing.T) {
	_, ts := newTestServer(t)
	lobbyID := hostLobby(t, ts)
	p1 := joinLobby(t, ts, lobbyID)

	cases := []struct {
		name string
		path string
	}{
		{"missing lobby", fmt.Sprintf("/lobby/leave?lobby_id=missing&player_id=%d", p1)},
		{"unknown player", fmt.Sprintf("/lobby/leave?lobby_id=%s&player_id=99", lobbyID)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doGet(t, ts, tc.path)
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("status %d, want 404", resp.StatusCode)
			}
		})
	}

	// Leaving twice: the second attempt no longer resolves the player.
	leaveLobby(t, ts, lobbyID, p1)
	resp := doGet(t, ts, fmt.Sprintf("/lobby/leave?lobby_id=%s&player_id=%d", lobbyID, p1))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("second leave: status %d, want 404", resp.StatusCode)
	}
}

// TestDeleteLobby covers a host tearing down its lobby: afterwards the lobby is gone and joins/offers 404.
func TestDeleteLobby(t *testing.T) {
	s, ts := newTestServer(t)
	lobbyID := hostLobby(t, ts)
	joinLobby(t, ts, lobbyID)

	deleteLobby(t, ts, lobbyID)
	if _, ok := s.lobbies[lobbyID]; ok {
		t.Errorf("lobby %q still present after delete", lobbyID)
	}

	resp := doGet(t, ts, "/lobby/join?id="+lobbyID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("join deleted lobby: status %d, want 404", resp.StatusCode)
	}
}

func TestDeleteNonexistentLobbyIsNoop(t *testing.T) {
	_, ts := newTestServer(t)
	resp := doGet(t, ts, "/lobby/delete?id=doesnotexist")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("delete missing lobby: status %d, want 200 (idempotent)", resp.StatusCode)
	}
}

func TestSDPInvalidLobbyAndPlayer(t *testing.T) {
	_, ts := newTestServer(t)
	lobbyID := hostLobby(t, ts)
	joinLobby(t, ts, lobbyID) // valid players are now 0 and 1

	cases := []struct {
		name string
		path string
	}{
		{"missing lobby", sdpPath("offer", "get", "missing", 1)},
		{"player out of range", sdpPath("offer", "get", lobbyID, 99)},
		{"negative player", sdpPath("offer", "get", lobbyID, -1)},
		{"non-numeric player", fmt.Sprintf("/offer/get?lobby_id=%s&player_id=abc", lobbyID)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doGet(t, ts, tc.path)
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("status %d, want 404", resp.StatusCode)
			}
		})
	}
}

// TestPostRejectsBadSDP confirms the server validates incoming session
// descriptions rather than storing arbitrary bytes.
func TestPostRejectsBadSDP(t *testing.T) {
	_, ts := newTestServer(t)
	lobbyID := hostLobby(t, ts)
	player := joinLobby(t, ts, lobbyID)

	cases := []struct {
		name string
		body []byte
	}{
		{"unknown sdp type", []byte(`{"type":"bogus","sdp":"x"}`)},
		{"malformed json", []byte(`{not json`)},
		{"empty body", []byte(``)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doPost(t, ts, sdpPath("offer", "post", lobbyID, player), tc.body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestCORSPreflight(t *testing.T) {
	_, ts := newTestServer(t)
	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/lobby/host", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("preflight status %d, want 204", resp.StatusCode)
	}
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("Access-Control-Allow-Origin %q, want *", origin)
	}
}

// TestConcurrentHostAndJoin exercises the server mutex: many clients host and
// join simultaneously. Run with -race to surface data races.
func TestConcurrentHostAndJoin(t *testing.T) {
	s, ts := newTestServer(t)

	const lobbies = 20
	const joinersPerLobby = 5

	var wg sync.WaitGroup
	for i := 0; i < lobbies; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lobbyID := hostLobby(t, ts)
			var inner sync.WaitGroup
			for j := 0; j < joinersPerLobby; j++ {
				inner.Add(1)
				go func() {
					defer inner.Done()
					joinLobby(t, ts, lobbyID)
				}()
			}
			inner.Wait()
		}()
	}
	wg.Wait()

	if len(s.lobbies) != lobbies {
		t.Errorf("server holds %d lobbies, want %d", len(s.lobbies), lobbies)
	}
	// The host is not stored in clients, so the map holds only the joiners.
	for id, l := range s.lobbies {
		if got := len(l.clients); got != joinersPerLobby {
			t.Errorf("lobby %s has %d clients, want %d", id, got, joinersPerLobby)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

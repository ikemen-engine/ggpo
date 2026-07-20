// Package signaling implements a small lobby-based HTTP signaling server used
// to establish WebRTC data channel connections between GGPO peers.
//
// A host creates a lobby and receives a lobby ID which it shares on Discord or wherever else.
// Clients join the lobby, post an SDP offer, and poll for the host's SDP answer.
// The server never inspects the session descriptions; it only stores and relays them
//
// Endpoints:
//
//	POST /lobby/host                                      -> lobby ID (text)
//	POST /lobby/hostWithId?id={lobby}					  -> lobby ID (text) (it's the same lobby ID we put in)
//	GET  /lobby/join?id={lobby}                           -> playerID (JSON number)
//	GET  /lobby/delete?id={lobby}
//	GET  /lobby/leave?lobby_id={lobby}&player_id={player}
//	GET  /lobby/unregisteredPlayers?id={lobby}            -> [playerID, ...]
//	GET  /offer/get?lobby_id={lobby}&player_id={player}   -> SDP offer
//	POST /offer/post?lobby_id={lobby}&player_id={player}
//	GET  /answer/get?lobby_id={lobby}&player_id={player}  -> SDP answer
//	POST /answer/post?lobby_id={lobby}&player_id={player}
package signaling

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"sync"

	"github.com/pion/webrtc/v4"
)

const lobbyIDLength = 6

type clientConnection struct {
	Offer  webrtc.SessionDescription
	Answer webrtc.SessionDescription
}

// PlayerId identifies a client within a lobby. The host is not a player and is
// never assigned a PlayerId.
type PlayerId int

type lobby struct {
	// host is the single host of the lobby. There is exactly one host per
	// lobby, and it is never part of clients.
	host clientConnection
	// clients holds every non-host player, keyed by the id handed out at join.
	clients map[PlayerId]clientConnection
	// nextID is the id the next joining client will receive. It only ever
	// increases, so ids stay stable even as clients come and go.
	nextID PlayerId
}

// Server relays session descriptions between peers in lobbies.
type Server struct {
	mutex   sync.Mutex
	lobbies map[string]*lobby
}

func NewServer() *Server {
	return &Server{
		lobbies: make(map[string]*lobby),
	}
}

// Handler returns the http.Handler serving the signaling endpoints. All
// responses allow cross-origin requests so browser builds can use the server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/lobby/host", s.lobbyHost)
	mux.HandleFunc("/lobby/hostWithId", s.lobbyHostWithId)
	mux.HandleFunc("/lobby/join", s.lobbyJoin)
	mux.HandleFunc("/lobby/delete", s.lobbyDelete)
	mux.HandleFunc("/lobby/leave", s.lobbyLeave)
	mux.HandleFunc("/lobby/unregisteredPlayers", s.lobbyUnregisteredPlayers)
	mux.HandleFunc("/offer/get", s.sdpGet(getOffer))
	mux.HandleFunc("/offer/post", s.sdpPost(setOffer))
	mux.HandleFunc("/answer/get", s.sdpGet(getAnswer))
	mux.HandleFunc("/answer/post", s.sdpPost(setAnswer))
	return allowCORS(mux)
}

// ListenAndServe serves the signaling endpoints on addr, e.g. ":3000".
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.Handler())
}

func allowCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) generateLobbyID() string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	for {
		buffer := make([]rune, lobbyIDLength)
		for i := range buffer {
			buffer[i] = letters[rand.Intn(len(letters))]
		}
		id := string(buffer)
		if _, taken := s.lobbies[id]; !taken {
			return id
		}
	}
}

func (s *Server) lobbyHost(w http.ResponseWriter, _ *http.Request) {
	s.mutex.Lock()
	lobbyID := s.generateLobbyID()
	s.lobbies[lobbyID] = &lobby{
		clients: make(map[PlayerId]clientConnection),
		nextID:  1,
	}
	s.mutex.Unlock()

	w.Write([]byte(lobbyID))
}

// lobbyHostWithId hosts a lobby under a caller-chosen id instead of a generated
// one, so peers that agreed on an id out of band (e.g. a preset shared in a
// browser test) can find each other without exchanging a generated id first. It
// fails if the id is already in use.
func (s *Server) lobbyHostWithId(w http.ResponseWriter, r *http.Request) {
	lobbyID := r.URL.Query().Get("id")
	if lobbyID == "" {
		http.Error(w, "400 - missing lobby id", http.StatusBadRequest)
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, taken := s.lobbies[lobbyID]; taken {
		http.Error(w, "409 - lobby id already in use", http.StatusConflict)
		return
	}
	s.lobbies[lobbyID] = &lobby{
		clients: make(map[PlayerId]clientConnection),
		nextID:  1,
	}

	w.Write([]byte(lobbyID))
}

func (s *Server) lobbyJoin(w http.ResponseWriter, r *http.Request) {
	lobbyID := r.URL.Query().Get("id")

	s.mutex.Lock()
	defer s.mutex.Unlock()
	l, ok := s.lobbies[lobbyID]
	if !ok {
		http.Error(w, "404 - Lobby not found", http.StatusNotFound)
		return
	}

	playerID := l.nextID
	l.nextID++
	l.clients[playerID] = clientConnection{}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(playerID)
}

func (s *Server) lobbyDelete(w http.ResponseWriter, r *http.Request) {
	lobbyID := r.URL.Query().Get("id")

	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.lobbies, lobbyID)
	w.WriteHeader(http.StatusOK)
}

// lobbyLeave removes a single client from a lobby. The host is not a player and
// cannot leave this way; it tears the lobby down via lobbyDelete instead. A
// left player's id is never reused, so remaining players keep their ids.
func (s *Server) lobbyLeave(w http.ResponseWriter, r *http.Request) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	l, playerID, ok := s.validatePlayer(w, r)
	if !ok {
		return
	}
	delete(l.clients, playerID)
	w.WriteHeader(http.StatusOK)
}

// lobbyUnregisteredPlayers returns the players the host has not answered yet.
func (s *Server) lobbyUnregisteredPlayers(w http.ResponseWriter, r *http.Request) {
	lobbyID := r.URL.Query().Get("id")

	s.mutex.Lock()
	defer s.mutex.Unlock()
	l, ok := s.lobbies[lobbyID]
	if !ok {
		http.Error(w, "404 - Lobby not found", http.StatusNotFound)
		return
	}

	playerIDs := []PlayerId{}
	for id, client := range l.clients {
		if client.Answer.SDP == "" {
			playerIDs = append(playerIDs, id)
		}
	}
	// Map iteration is unordered; sort so the host sees a stable list.
	sort.Slice(playerIDs, func(a, b int) bool { return playerIDs[a] < playerIDs[b] })

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(playerIDs)
}

// validatePlayer resolves the lobby and player referenced by the request's
// query parameters. The server mutex must be held by the caller.
func (s *Server) validatePlayer(w http.ResponseWriter, r *http.Request) (*lobby, PlayerId, bool) {
	lobbyID := r.URL.Query().Get("lobby_id")
	l, ok := s.lobbies[lobbyID]
	if !ok {
		http.Error(w, "404 - Lobby not found", http.StatusNotFound)
		return nil, 0, false
	}

	raw, err := strconv.Atoi(r.URL.Query().Get("player_id"))
	if err != nil {
		http.Error(w, "404 - Player not found", http.StatusNotFound)
		return nil, 0, false
	}
	playerID := PlayerId(raw)
	if _, exists := l.clients[playerID]; !exists {
		http.Error(w, "404 - Player not found", http.StatusNotFound)
		return nil, 0, false
	}

	return l, playerID, true
}

func getOffer(c *clientConnection) *webrtc.SessionDescription  { return &c.Offer }
func getAnswer(c *clientConnection) *webrtc.SessionDescription { return &c.Answer }

func setOffer(c *clientConnection, sdp webrtc.SessionDescription)  { c.Offer = sdp }
func setAnswer(c *clientConnection, sdp webrtc.SessionDescription) { c.Answer = sdp }

func (s *Server) sdpGet(field func(*clientConnection) *webrtc.SessionDescription) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mutex.Lock()
		defer s.mutex.Unlock()
		l, playerID, ok := s.validatePlayer(w, r)
		if !ok {
			return
		}
		client := l.clients[playerID]
		sdp := field(&client)
		if sdp.SDP == "" {
			http.Error(w, "404 - Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sdp)
	}
}

func (s *Server) sdpPost(field func(*clientConnection, webrtc.SessionDescription)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var sdp webrtc.SessionDescription
		if err := json.NewDecoder(r.Body).Decode(&sdp); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.mutex.Lock()
		defer s.mutex.Unlock()
		l, playerID, ok := s.validatePlayer(w, r)
		if !ok {
			return
		}
		client := l.clients[playerID]
		field(&client, sdp)
		l.clients[playerID] = client
		w.WriteHeader(http.StatusOK)
	}
}

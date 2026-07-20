package udp

import (
	"net"
	"strconv"
	"sync"

	"github.com/ikemen-engine/ggpo/internal/util"
	"github.com/ikemen-engine/ggpo/transport"
)

const (
	MaxUDPEndpoints  = 16
	MaxUDPPacketSize = 4096
)

// PeerAddress is the ip:port a peer's packets are sent to and arrive from.
type PeerAddress struct {
	Ip   string
	Port int
}

func (p PeerAddress) String() string {
	return p.Ip + ":" + strconv.Itoa(p.Port)
}

// Udp adapts a UDP socket to transport.Transport. The rest of GGPO addresses
// peers only by player handle; the mapping from handle to ip:port lives here,
// so every remote peer must be registered with AddPeer before packets can flow.
type Udp struct {
	Stats transport.Stats // may not need this, may just be a service used by others

	listener  net.PacketConn
	localPort int
	sendChan  chan sendRequest

	mu            sync.Mutex
	peerAddresses map[transport.PlayerHandle]PeerAddress // player handle -> address packets are sent to
	peerHandles   map[string]transport.PlayerHandle      // "ip:port" -> player handle for inbound packets
}

func getPeerAddress(address net.Addr) PeerAddress {
	switch addr := address.(type) {
	case *net.UDPAddr:
		return PeerAddress{
			Ip:   addr.IP.String(),
			Port: addr.Port,
		}
	}
	return PeerAddress{}
}

func (u *Udp) Close() {
	if u.listener != nil {
		u.listener.Close()
	}
}

type sendRequest struct {
	msg  transport.Message
	addr PeerAddress
}

func NewUdp(localPort int) *Udp {
	u := Udp{
		peerAddresses: make(map[transport.PlayerHandle]PeerAddress),
		peerHandles:   make(map[string]transport.PlayerHandle),
	}

	u.sendChan = make(chan sendRequest, 256) // Create a buffered channel

	go func() { // Start a goroutine to handle sending of messages
		for req := range u.sendChan {
			RemoteEP := net.UDPAddr{IP: net.ParseIP(req.addr.Ip), Port: req.addr.Port}
			buf := req.msg.ToBytes()
			_, err := u.listener.WriteTo(buf, &RemoteEP)
			if err != nil {
				util.Log.Printf("WriteTo error: %s", err)
			}
		}
	}()

	portStr := strconv.Itoa(localPort)

	u.localPort = localPort
	util.Log.Printf("binding udp socket to port %d.\n", localPort)
	u.listener, _ = net.ListenPacket("udp", "0.0.0.0:"+portStr)
	return &u
}

// AddPeer registers the address for a player. Packets sent to that player go to
// ip:port, and inbound packets from ip:port are tagged with the handle.
func (u *Udp) AddPeer(player transport.PlayerHandle, ip string, port int) {
	addr := PeerAddress{Ip: ip, Port: port}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.peerAddresses[player] = addr
	u.peerHandles[addr.String()] = player
}

func (u *Udp) SendTo(msg transport.Message, player transport.PlayerHandle) {
	if msg == nil {
		return
	}
	u.mu.Lock()
	addr, ok := u.peerAddresses[player]
	u.mu.Unlock()
	if !ok {
		util.Log.Printf("dropping send to unregistered player %d\n", player)
		return
	}

	u.sendChan <- sendRequest{msg: msg, addr: addr} // Add the request to the channel
}

func (u *Udp) Read(messageChan chan transport.MessageChannelItem) {
	defer u.listener.Close()
	recvBuf := make([]byte, MaxUDPPacketSize*2)
	for {
		len, addr, err := u.listener.ReadFrom(recvBuf)
		if err != nil {
			util.Log.Printf("conn.Read error returned: %s\n", err)
			break
		} else if len <= 0 {
			util.Log.Printf("no data recieved\n")
		} else if len > 0 {
			util.Log.Printf("recvfrom returned (len:%d  from:%s).\n", len, addr.String())
			peer := getPeerAddress(addr)

			u.mu.Lock()
			player, ok := u.peerHandles[peer.String()]
			u.mu.Unlock()
			if !ok {
				util.Log.Printf("dropping packet from unregistered address %s\n", peer)
				continue
			}

			msg, err := transport.DecodeMessageBinary(recvBuf)
			if err != nil {
				util.Log.Printf("Error decoding message: %s", err)
				continue
			}
			messageChan <- transport.MessageChannelItem{Player: player, Message: msg, Length: len}
		}

	}
}

func (u *Udp) IsInitialized() bool {
	return u.listener != nil
}

package dht

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"time"
	"the-hive/internal/logger"
)

var (
	defaultMulticastAddr = "239.0.0.1:7441"
	beaconInterval       = 30 * time.Second
)

// DiscoveryBeacon is the message sent over multicast to announce a node.
type DiscoveryBeacon struct {
	ID      NodeID `json:"id"`
	DHTPort int    `json:"dht_port"`
}

// Discovery service handles automatic peer discovery on the local network.
type Discovery struct {
	router      *Router
	dhtPort     int
	mcastAddr   string
	autoEnabled bool
	interval    time.Duration
}

// NewDiscovery creates a new Discovery service.
func NewDiscovery(router *Router, dhtPort int, mcastAddr string, enabled bool) *Discovery {
	if mcastAddr == "" {
		mcastAddr = defaultMulticastAddr
	}
	return &Discovery{
		router:      router,
		dhtPort:     dhtPort,
		mcastAddr:   mcastAddr,
		autoEnabled: enabled,
		interval:    beaconInterval,
	}
}

func (d *Discovery) SetInterval(i time.Duration) {
	d.interval = i
}

// Start initiates the discovery process.
func (d *Discovery) Start(ctx context.Context) {
	if !d.autoEnabled {
		return
	}

	logger.Info("Discovery: Iniciando autodescubrimiento en %s", d.mcastAddr)

	// 1. Start Listener
	go d.listenLoop(ctx)

	// 2. Start Beacon
	go d.beaconLoop(ctx)
}

func (d *Discovery) listenLoop(ctx context.Context) {
	addr, err := net.ResolveUDPAddr("udp", d.mcastAddr)
	if err != nil {
		logger.Error("Discovery: Error al resolver dirección multicast: %v", err)
		return
	}

	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		logger.Error("Discovery: Error al escuchar multicast: %v", err)
		return
	}
	defer conn.Close()

	// Handle shutdown
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	buffer := make([]byte, 1024)
	for {
		n, src, err := conn.ReadFromUDP(buffer)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				logger.Error("Discovery: Error de lectura: %v", err)
				continue
			}
		}

		var beacon DiscoveryBeacon
		if err := json.Unmarshal(buffer[:n], &beacon); err != nil {
			continue
		}

		// Don't discover ourselves
		if beacon.ID == d.router.sender.LocalID() {
			continue
		}

		// Construct the peer's DHT address using the source IP and the announced port
		host, _, _ := net.SplitHostPort(src.String())
		peerAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(beacon.DHTPort)))
		if err != nil {
			continue
		}

		// Using HandleMessage logic (via Ping) to ensure bidirectional connectivity
		// and trigger any necessary handoffs.
		msg := Message{
			Type:     Ping,
			Version:  ProtocolVersion,
			SenderID: beacon.ID,
		}
		d.router.HandleMessage(peerAddr, msg)
	}
}

func (d *Discovery) beaconLoop(ctx context.Context) {
	addr, err := net.ResolveUDPAddr("udp", d.mcastAddr)
	if err != nil {
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		logger.Error("Discovery: Error al crear socket de envío: %v", err)
		return
	}
	defer conn.Close()

	beacon := DiscoveryBeacon{
		ID:      d.router.sender.LocalID(),
		DHTPort: d.dhtPort,
	}
	data, _ := json.Marshal(beacon)

	// Send initial beacon
	_, _ = conn.Write(data)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := conn.Write(data)
			if err != nil {
				logger.Error("Discovery: Error al enviar beacon: %v", err)
			}
		}
	}
}

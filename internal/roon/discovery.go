package roon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// This matches node-roon-api/lib.js
const roonRegistryServiceID = "00720724-5143-4a9b-abac-0e50cba674bb"

type discoverOptions struct {
	Timeout time.Duration
}

func (c *Client) Discover(ctx context.Context) ([]Core, error) {
	return c.DiscoverWithOptions(ctx, discoverOptions{Timeout: 1500 * time.Millisecond})
}

func (c *Client) DiscoverWithOptions(ctx context.Context, opt discoverOptions) ([]Core, error) {
	if opt.Timeout <= 0 {
		opt.Timeout = 1500 * time.Millisecond
	}

	queryID, err := randomUUIDLike()
	if err != nil {
		return nil, err
	}

	pkt, err := encodeSoodQuery(map[string]string{
		"_tid":             queryID,
		"query_service_id": roonRegistryServiceID,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, opt.Timeout)
	defer cancel()
	deadline := time.Now().Add(opt.Timeout)

	// Some replies come back to the *sender's ephemeral port* (node-roon-api listens on both
	// recv_sock (9003) and send_sock (ephemeral)). So we listen on:
	// - :9003 (broadcast/unicast)
	// - one ephemeral socket per interface address (for unicast replies)
	recvBroadcast, err := listenUDPReuse("udp4", fmt.Sprintf(":%d", soodPort))
	if err != nil {
		return nil, err
	}
	_ = recvBroadcast.SetReadDeadline(deadline)

	// Send query on each IPv4 interface address (multicast + broadcast), similar to sood.js.
	ifaces, _ := net.Interfaces()

	// Listen for multicast on each interface (this is the important part!).
	mcastConns := make([]*net.UDPConn, 0, 4)
	group := &net.UDPAddr{IP: net.ParseIP(soodMulticastIP), Port: soodPort}
	for i := range ifaces {
		iface := ifaces[i]
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		cn, err := net.ListenMulticastUDP("udp4", &iface, group)
		if err != nil {
			continue
		}
		_ = cn.SetReadDeadline(deadline)
		mcastConns = append(mcastConns, cn)
	}
	defer func() {
		for _, cn := range mcastConns {
			_ = cn.Close()
		}
	}()

	senders := make([]*net.UDPConn, 0, 4)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP == nil {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil || ip4.IsLoopback() {
				continue
			}

			// Open a per-interface sender socket (ephemeral port) and listen on it too.
			conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip4, Port: 0})
			if err != nil {
				c.log.Debug("sood send sock open failed", "ip", ip4.String(), "err", err)
				continue
			}
			_ = conn.SetReadDeadline(deadline)
			_ = conn.SetWriteDeadline(deadline)
			_ = enableBroadcast(conn)
			_ = setIPv4MulticastTTL(conn, 1)
			senders = append(senders, conn)

			// Multicast send.
			_, _ = conn.WriteToUDP(pkt, &net.UDPAddr{IP: net.ParseIP(soodMulticastIP), Port: soodPort})

			// Broadcast send (best-effort).
			if len(ipnet.Mask) == 4 {
				bcast := make(net.IP, 4)
				for i := 0; i < 4; i++ {
					bcast[i] = ip4[i] | ^ipnet.Mask[i]
				}
				_, _ = conn.WriteToUDP(pkt, &net.UDPAddr{IP: bcast, Port: soodPort})
			}
		}
	}

	// Also send+listen from an unbound socket (like sood.js _unicast).
	unbound, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err == nil {
		_ = unbound.SetReadDeadline(deadline)
		_ = unbound.SetWriteDeadline(deadline)
		_ = enableBroadcast(unbound)
		_ = setIPv4MulticastTTL(unbound, 1)
		senders = append(senders, unbound)
		_, _ = unbound.WriteToUDP(pkt, &net.UDPAddr{IP: net.ParseIP(soodMulticastIP), Port: soodPort})
	}

	type coreKey struct {
		id string
	}
	seen := map[coreKey]Core{}

	var packetCount atomic.Int64

	// (reader is implemented further down; concurrent readers share the same deadline)

	var mu sync.Mutex
	readConn := func(conn *net.UDPConn) {
		defer func() { _ = conn.Close() }()
		buf := make([]byte, 64*1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, addr, err := conn.ReadFromUDPAddrPort(buf)
			if err != nil {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					return
				}
				return
			}

			if packetCount.Add(1) <= 10 {
				c.log.Debug("sood packet received", "from", addr.String(), "n", n)
			}

			msg, err := parseSoodPacket(buf[:n], addr.Addr().String(), int(addr.Port()))
			if err != nil {
				if packetCount.Load() <= 10 {
					c.log.Debug("sood packet parse failed", "err", err)
				}
				continue
			}

			if packetCount.Load() <= 10 {
				c.log.Debug("sood packet parsed",
					"type", string([]byte{msg.Type}),
					"service_id", msg.Props["service_id"],
					"unique_id", msg.Props["unique_id"],
					"http_port", msg.Props["http_port"],
				)
			}

			if msg.Props["service_id"] != roonRegistryServiceID {
				continue
			}
			uid := msg.Props["unique_id"]
			if uid == "" {
				continue
			}

			port, _ := strconv.Atoi(msg.Props["http_port"])
			if port <= 0 || port > 65535 {
				port = 0
			}

			name := msg.Props["display_name"]
			if name == "" {
				name = msg.Props["name"]
			}
			if name == "" {
				name = uid
			}

			core := Core{
				ID:   CoreID(uid),
				Name: name,
				Host: msg.FromIP,
				Port: port,
			}

			mu.Lock()
			seen[coreKey{id: uid}] = core
			mu.Unlock()
		}
	}

	// Read from broadcast + all multicast listeners concurrently until ctx timeout.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		readConn(recvBroadcast)
	}()
	for _, cn := range mcastConns {
		wg.Add(1)
		go func(cn *net.UDPConn) {
			defer wg.Done()
			readConn(cn)
		}(cn)
	}
	for _, cn := range senders {
		wg.Add(1)
		go func(cn *net.UDPConn) {
			defer wg.Done()
			readConn(cn)
		}(cn)
	}

	<-ctx.Done()
	_ = recvBroadcast.Close()
	for _, cn := range mcastConns {
		_ = cn.Close()
	}
	for _, cn := range senders {
		_ = cn.Close()
	}
	wg.Wait()

	out := make([]Core, 0, len(seen))
	for _, core := range seen {
		out = append(out, core)
	}

	// Helpful debug when discovery yields nothing.
	if len(out) == 0 {
		c.log.Debug("no cores discovered (timeout reached)", "timeout", opt.Timeout)
	}
	return out, nil
}

// Legacy helpers removed: we now keep sender sockets open so we can receive replies
// on the sender's ephemeral port (matches node-roon-api behavior).

func randomUUIDLike() (string, error) {
	// node-uuid v4 style; we only need uniqueness for _tid.
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]), nil
}

func (c *Client) withDefaultLogger() {
	if c.log == nil {
		c.log = slog.Default()
	}
}

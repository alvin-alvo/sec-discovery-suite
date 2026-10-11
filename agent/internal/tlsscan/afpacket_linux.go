package tlsscan

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	"sds/agent/internal/attribution"
	"sds/agent/internal/certinfo"
)

// TLSObservation represents the docs/06 tls_observations record wire format.
type TLSObservation struct {
	ConnKey         string                `json:"conn_key"`
	FirstTS         *string               `json:"first_ts"`
	LastTS          *string               `json:"last_ts"`
	Direction       *string               `json:"direction"`
	Transport       string                `json:"transport"`
	LocalIP         *string               `json:"local_ip"`
	LocalPort       int                   `json:"local_port"`
	RemoteIP        *string               `json:"remote_ip"`
	RemotePort      int                   `json:"remote_port"`
	SNI             *string               `json:"sni"`
	TLSVersion      *string               `json:"tls_version"`
	CipherSuite     *string               `json:"cipher_suite"`
	KexGroup        *string               `json:"kex_group"`
	PQCClass        *string               `json:"pqc_class"`
	Outcome         *string               `json:"outcome"`
	PeerCertFP      *string               `json:"peer_cert_fp"`
	PeerCertSource  *string               `json:"peer_cert_source"`
	Process         *attribution.ProcessInfo `json:"process"`
	RecordCount     int                   `json:"record_count"`
	HandshakeCount  int                   `json:"handshake_count"`
	AppdataCount    int                   `json:"appdata_count"`
	AlertCount      int                   `json:"alert_count"`
	HandshakeSeq    *string               `json:"handshake_seq"`
	PeerCertificate *certinfo.Certificate `json:"peer_certificate"`
}

// Sniffer interface per docs/02 and prompt.
type Sniffer interface {
	Run(ctx context.Context, emit func(*TLSObservation)) error
	Mode() string // "af_packet"
}

type AFPacketSniffer struct {
	localAddrs map[string]bool
	loIndex    int
}

func htons(v uint16) int {
	return int((v<<8)&0xff00 | (v>>8)&0x00ff)
}

func NewAFPacketSniffer() *AFPacketSniffer {
	localAddrs := make(map[string]bool)
	loIdx := 0

	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagLoopback != 0 {
				loIdx = iface.Index
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, a := range addrs {
				var ip net.IP
				switch v := a.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil {
					localAddrs[ip.String()] = true
				}
			}
		}
	}
	if loIdx == 0 {
		if iface, err := net.InterfaceByName("lo"); err == nil {
			loIdx = iface.Index
		}
	}

	return &AFPacketSniffer{
		localAddrs: localAddrs,
		loIndex:    loIdx,
	}
}

func (s *AFPacketSniffer) Mode() string {
	return "af_packet"
}

type flowKey struct {
	ip1, ip2     string
	port1, port2 uint16
	proto        uint8
}

func makeFlowKey(sip, dip string, sport, dport uint16, proto uint8) flowKey {
	if sip < dip || (sip == dip && sport <= dport) {
		return flowKey{ip1: sip, ip2: dip, port1: sport, port2: dport, proto: proto}
	}
	return flowKey{ip1: dip, ip2: sip, port1: dport, port2: sport, proto: proto}
}

type flowEntry struct {
	key                flowKey
	firstTS            time.Time
	lastTS             time.Time
	clientIP           string
	clientPort         int
	serverIP           string
	serverPort         int
	state              *TLSConnectionState
	clientReasm        *FlowReassembler
	serverReasm        *FlowReassembler
	lastEmittedRecords int
	lastEmittedOutcome string
}

func (s *AFPacketSniffer) Run(ctx context.Context, emit func(*TLSObservation)) error {
	// Create AF_PACKET raw socket with correct network byte order protocol
	htonsAll := htons(unix.ETH_P_ALL)
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, htonsAll)
	if err != nil {
		if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
			return fmt.Errorf("raw socket denied: needs CAP_NET_RAW: %w", err)
		}
		return fmt.Errorf("failed to open AF_PACKET socket: %w", err)
	}
	defer unix.Close(fd)

	// Set non-blocking to allow context cancellation
	if err := unix.SetNonblock(fd, true); err != nil {
		return fmt.Errorf("failed to set nonblocking: %w", err)
	}

	flows := make(map[flowKey]*flowEntry)
	var flowsMu sync.Mutex

	buf := make([]byte, 65536)

	// Ticker for periodic live flow emission and eviction
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Scan ended: flush active flows to emit
			flowsMu.Lock()
			for _, flow := range flows {
				if flow.state.RecordCount > 0 && (flow.state.RecordCount != flow.lastEmittedRecords || flow.state.Outcome != flow.lastEmittedOutcome) {
					obs := s.buildObservation(flow)
					if obs != nil {
						flow.lastEmittedRecords = flow.state.RecordCount
						flow.lastEmittedOutcome = flow.state.Outcome
						emit(obs)
					}
				}
			}
			flowsMu.Unlock()
			return nil

		case <-ticker.C:
			flowsMu.Lock()
			now := time.Now()
			for k, flow := range flows {
				// Evict expired flows (> 60s inactivity)
				if now.Sub(flow.lastTS) > 60*time.Second {
					if flow.state.RecordCount > 0 && (flow.state.RecordCount != flow.lastEmittedRecords || flow.state.Outcome != flow.lastEmittedOutcome) {
						obs := s.buildObservation(flow)
						if obs != nil {
							emit(obs)
						}
					}
					delete(flows, k)
					continue
				}

				// Emit active flow if new TLS records were seen or outcome updated
				if flow.state.RecordCount > 0 && (flow.state.RecordCount != flow.lastEmittedRecords || flow.state.Outcome != flow.lastEmittedOutcome) {
					obs := s.buildObservation(flow)
					if obs != nil {
						flow.lastEmittedRecords = flow.state.RecordCount
						flow.lastEmittedOutcome = flow.state.Outcome
						emit(obs)
					}
				}
			}
			flowsMu.Unlock()

		default:
			n, from, err := unix.Recvfrom(fd, buf, 0)
			if err != nil {
				if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
					time.Sleep(1 * time.Millisecond)
					continue
				}
				if errors.Is(err, unix.EINTR) {
					continue
				}
				// Other error
				continue
			}

			if n < 14 {
				continue
			}

			// Filter loopback: skip PACKET_OUTGOING on loopback "lo"
			if ll, ok := from.(*unix.SockaddrLinklayer); ok {
				if s.loIndex > 0 && int(ll.Ifindex) == s.loIndex && ll.Pkttype == unix.PACKET_OUTGOING {
					// Loopback frame arriving outgoing: skip so loopback frames aren't doubled
					continue
				}
			}

			frame := buf[:n]
			s.handleFrame(frame, time.Now(), flows, &flowsMu)
		}
	}
}

func (s *AFPacketSniffer) handleFrame(frame []byte, now time.Time, flows map[flowKey]*flowEntry, flowsMu *sync.Mutex) {
	if len(frame) < 14 {
		return
	}

	// 1. Ethernet header
	etherType := binary.BigEndian.Uint16(frame[12:14])
	offset := 14

	// Handle 802.1Q VLAN
	if etherType == 0x8100 && len(frame) >= offset+4 {
		etherType = binary.BigEndian.Uint16(frame[offset+2 : offset+4])
		offset += 4
	}

	var srcIP, dstIP net.IP
	var proto uint8
	var ipPayload []byte

	// 2. IP Header
	if etherType == 0x0800 { // IPv4
		if len(frame) < offset+20 {
			return
		}
		ihl := int(frame[offset]&0x0F) * 4
		if len(frame) < offset+ihl {
			return
		}
		proto = frame[offset+9]
		srcIP = net.IPv4(frame[offset+12], frame[offset+13], frame[offset+14], frame[offset+15])
		dstIP = net.IPv4(frame[offset+16], frame[offset+17], frame[offset+18], frame[offset+19])
		ipPayload = frame[offset+ihl:]
	} else if etherType == 0x86DD { // IPv6
		if len(frame) < offset+40 {
			return
		}
		proto = frame[offset+6]
		srcIP = net.IP(frame[offset+8 : offset+24])
		dstIP = net.IP(frame[offset+24 : offset+40])
		ipPayload = frame[offset+40:]
	} else {
		return
	}

	// Filter TCP only (proto 6)
	if proto != 6 || len(ipPayload) < 20 {
		return
	}

	// 3. TCP Header
	srcPort := binary.BigEndian.Uint16(ipPayload[0:2])
	dstPort := binary.BigEndian.Uint16(ipPayload[2:4])
	tcpDataOffset := int(ipPayload[12]>>4) * 4
	if len(ipPayload) < tcpDataOffset {
		return
	}

	flags := ipPayload[13]
	tcpPayload := ipPayload[tcpDataOffset:]

	sip := srcIP.String()
	dip := dstIP.String()
	fKey := makeFlowKey(sip, dip, srcPort, dstPort, proto)

	flowsMu.Lock()
	defer flowsMu.Unlock()

	flow, exists := flows[fKey]
	if !exists {
		if len(flows) >= 4096 {
			// Cap reached
			return
		}

		flow = &flowEntry{
			key:         fKey,
			firstTS:     now,
			lastTS:      now,
			clientIP:    sip,
			clientPort:  int(srcPort),
			serverIP:    dip,
			serverPort:  int(dstPort),
			state:       NewTLSConnectionState(),
			clientReasm: NewFlowReassembler(),
			serverReasm: NewFlowReassembler(),
		}
		flows[fKey] = flow
	}

	flow.lastTS = now

	// Detect if TCP segment carries TLS data
	if len(tcpPayload) > 0 {
		// ClientHello packet: sip:srcPort is definitely the client
		if len(tcpPayload) >= 6 && tcpPayload[0] == 22 && tcpPayload[5] == 1 {
			if flow.clientIP != sip || flow.clientPort != int(srcPort) {
				flow.clientIP, flow.serverIP = sip, dip
				flow.clientPort, flow.serverPort = int(srcPort), int(dstPort)
				flow.clientReasm.Reset()
				flow.serverReasm.Reset()
			}
		}

		// Feed according to direction
		if sip == flow.clientIP && int(srcPort) == flow.clientPort {
			flow.clientReasm.Append(tcpPayload)
			flow.state.FeedBytes(flow.clientReasm)
		} else {
			flow.serverReasm.Append(tcpPayload)
			flow.state.FeedBytes(flow.serverReasm)
		}
	}

	// If FIN or RST flag seen, connection is ending
	if flags&(0x01|0x04) != 0 {
		if flow.state.Outcome == "incomplete" {
			if flow.state.sawServerHello && (flow.state.sawFinished || flow.state.sawAppdata) {
				flow.state.Outcome = "established"
			} else {
				flow.state.Outcome = "incomplete"
			}
		}
	}
}

func (s *AFPacketSniffer) buildObservation(flow *flowEntry) *TLSObservation {
	st := flow.state
	if st.RecordCount == 0 && !st.sawClientHello {
		return nil // Not a TLS connection
	}

	// Direction: ClientHello sender is the client
	// Outbound if the client IP is a local interface address (loopback => outbound)
	isClientLocal := s.localAddrs[flow.clientIP] || strings.HasPrefix(flow.clientIP, "127.") || flow.clientIP == "::1"
	direction := "inbound"
	if isClientLocal {
		direction = "outbound"
	}

	var localIP, remoteIP string
	var localPort, remotePort int

	if direction == "outbound" {
		localIP = flow.clientIP
		localPort = flow.clientPort
		remoteIP = flow.serverIP
		remotePort = flow.serverPort
	} else {
		localIP = flow.serverIP
		localPort = flow.serverPort
		remoteIP = flow.clientIP
		remotePort = flow.clientPort
	}

	// Hash 5-tuple + firstTS
	firstTSStr := flow.firstTS.UTC().Format(time.RFC3339Nano)
	lastTSStr := flow.lastTS.UTC().Format(time.RFC3339Nano)
	fiveTupleKey := fmt.Sprintf("%s:%d-%s:%d-%d-%s", flow.clientIP, flow.clientPort, flow.serverIP, flow.serverPort, flow.key.proto, firstTSStr)
	sum := sha1.Sum([]byte(fiveTupleKey))
	connKey := hex.EncodeToString(sum[:])

	// Handshake sequence
	seqStr := st.HandshakeSequenceString()
	var handshakeSeq *string
	if seqStr != "" {
		handshakeSeq = &seqStr
	}

	// Attribution
	procInfo := attribution.ResolveConnection(localIP, localPort, remoteIP, remotePort)

	// Peer cert source
	certSource := "unavailable"
	if st.PeerCertFP != nil {
		certSource = "observed"
	}

	outcome := st.Outcome
	if outcome == "" {
		outcome = "incomplete"
	}

	return &TLSObservation{
		ConnKey:         connKey,
		FirstTS:         &firstTSStr,
		LastTS:          &lastTSStr,
		Direction:       &direction,
		Transport:       "tcp",
		LocalIP:         &localIP,
		LocalPort:       localPort,
		RemoteIP:        &remoteIP,
		RemotePort:      remotePort,
		SNI:             st.SNI,
		TLSVersion:      st.TLSVersion,
		CipherSuite:     st.CipherSuite,
		KexGroup:        st.KexGroup,
		PQCClass:        st.PQCClass,
		Outcome:         &outcome,
		PeerCertFP:      st.PeerCertFP,
		PeerCertSource:  &certSource,
		Process:         procInfo,
		RecordCount:     st.RecordCount,
		HandshakeCount:  st.HandshakeCount,
		AppdataCount:    st.AppdataCount,
		AlertCount:      st.AlertCount,
		HandshakeSeq:    handshakeSeq,
		PeerCertificate: st.PeerCertificate,
	}
}

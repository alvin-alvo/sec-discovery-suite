package tlsscan

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"sds/agent/internal/certinfo"
)

// HelloRetryRequest magic random constant per RFC 8446
var hrrRandom = []byte{
	0xcf, 0x21, 0xad, 0x74, 0xe5, 0x9a, 0x61, 0x11,
	0xbe, 0x1d, 0x8c, 0x02, 0x1e, 0x65, 0xb8, 0x91,
	0xc2, 0xa2, 0x11, 0x16, 0x7a, 0xbb, 0x8c, 0x5e,
	0x07, 0x9e, 0x09, 0xe2, 0xc8, 0xa8, 0x33, 0x9c,
}

// TLSConnectionState tracks TLS metadata for a connection flow.
type TLSConnectionState struct {
	TLSVersion      *string
	CipherSuite     *string
	KexGroup        *string
	PQCClass        *string // classical | hybrid | pqc | unknown
	SNI             *string
	Outcome         string // established | failed | alert | incomplete
	PeerCertFP      *string
	PeerCertSource  *string // observed | probed | unavailable
	PeerCertificate *certinfo.Certificate

	RecordCount    int
	HandshakeCount int
	AppdataCount   int
	AlertCount     int
	HandshakeSeq   []string

	sawClientHello bool
	sawServerHello bool
	sawFinished    bool
	sawAppdata     bool
	sawAlert       bool
}

func NewTLSConnectionState() *TLSConnectionState {
	return &TLSConnectionState{
		Outcome:      "incomplete",
		HandshakeSeq: make([]string, 0, 8),
	}
}

// FeedBytes processes incoming segment bytes on a flow direction.
func (s *TLSConnectionState) FeedBytes(reasm *FlowReassembler) {
	for {
		buf := reasm.Bytes()
		if len(buf) < 5 {
			return // Need at least 5 bytes for TLS record header
		}

		contentType := buf[0]
		// Validate TLS record content type: 20=ChangeCipherSpec, 21=Alert, 22=Handshake, 23=AppData
		if contentType < 20 || contentType > 23 {
			// Not a valid TLS record header; discard byte to resync
			reasm.Discard(1)
			continue
		}

		recordLen := int(binary.BigEndian.Uint16(buf[3:5]))
		if recordLen > 16384+2048 { // TLS record max is ~16KB + padding
			reasm.Discard(1)
			continue
		}

		if len(buf) < 5+recordLen {
			return // Incomplete record, wait for more bytes
		}

		// Full record observed!
		s.RecordCount++
		payload := buf[5 : 5+recordLen]

		switch contentType {
		case 20: // ChangeCipherSpec
			s.appendSeq("CCS")
		case 21: // Alert
			s.AlertCount++
			s.sawAlert = true
			s.Outcome = "alert"
			s.appendSeq("ALERT")
		case 22: // Handshake
			s.HandshakeCount++
			s.parseHandshakePayload(payload)
		case 23: // ApplicationData
			// CRITICAL: NEVER store application data bytes!
			s.AppdataCount++
			s.sawAppdata = true
			if s.sawServerHello && s.Outcome != "alert" {
				s.Outcome = "established"
			}
		}

		reasm.Discard(5 + recordLen)
	}
}

func (s *TLSConnectionState) appendSeq(step string) {
	if len(s.HandshakeSeq) == 0 || s.HandshakeSeq[len(s.HandshakeSeq)-1] != step {
		s.HandshakeSeq = append(s.HandshakeSeq, step)
	}
}

func (s *TLSConnectionState) parseHandshakePayload(payload []byte) {
	for len(payload) >= 4 {
		hsType := payload[0]
		hsLen := int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
		if len(payload) < 4+hsLen {
			return // Incomplete handshake message inside record
		}

		hsBody := payload[4 : 4+hsLen]

		switch hsType {
		case 1: // ClientHello
			s.sawClientHello = true
			s.appendSeq("CH")
			s.parseClientHello(hsBody)
		case 2: // ServerHello
			s.sawServerHello = true
			s.parseServerHello(hsBody)
		case 11: // Certificate (TLS <= 1.2)
			s.appendSeq("CERT")
			s.parseCertificate(hsBody)
		case 20: // Finished
			s.sawFinished = true
			s.appendSeq("FIN")
			if s.Outcome != "alert" {
				s.Outcome = "established"
			}
		default:
			// Other handshake types: SKE, SHD, CKE, etc.
		}

		payload = payload[4+hsLen:]
	}
}

func (s *TLSConnectionState) parseClientHello(body []byte) {
	if len(body) < 34 {
		return
	}
	// Skip client_version (2), random (32)
	offset := 34

	// Session ID
	if offset >= len(body) {
		return
	}
	sessIDLen := int(body[offset])
	offset += 1 + sessIDLen

	// Cipher Suites
	if offset+2 > len(body) {
		return
	}
	ciphersLen := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2 + ciphersLen

	// Compression Methods
	if offset >= len(body) {
		return
	}
	compLen := int(body[offset])
	offset += 1 + compLen

	// Extensions
	if offset+2 > len(body) {
		return
	}
	extsLen := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2

	extsEnd := offset + extsLen
	if extsEnd > len(body) {
		extsEnd = len(body)
	}

	for offset+4 <= extsEnd {
		extType := binary.BigEndian.Uint16(body[offset : offset+2])
		extLen := int(binary.BigEndian.Uint16(body[offset+2 : offset+4]))
		offset += 4

		if offset+extLen > extsEnd {
			break
		}

		extData := body[offset : offset+extLen]
		offset += extLen

		// Ext 0: Server Name Indication (SNI)
		if extType == 0 && len(extData) >= 5 {
			// server_name_list length (2 bytes)
			sniOffset := 2
			for sniOffset+3 <= len(extData) {
				nameType := extData[sniOffset]
				nameLen := int(binary.BigEndian.Uint16(extData[sniOffset+1 : sniOffset+3]))
				sniOffset += 3
				if sniOffset+nameLen <= len(extData) && nameType == 0 { // 0 = host_name
					sni := string(extData[sniOffset : sniOffset+nameLen])
					s.SNI = &sni
					break
				}
				sniOffset += nameLen
			}
		}
	}
}

func (s *TLSConnectionState) parseServerHello(body []byte) {
	if len(body) < 38 {
		return
	}
	// Check HelloRetryRequest
	randomBytes := body[2:34]
	if bytes.Equal(randomBytes, hrrRandom) {
		s.appendSeq("HRR")
	} else {
		s.appendSeq("SH")
	}

	offset := 34
	sessIDLen := int(body[offset])
	offset += 1 + sessIDLen

	if offset+2 > len(body) {
		return
	}
	cipherID := binary.BigEndian.Uint16(body[offset : offset+2])
	offset += 2

	cipherStr := lookupCipherSuite(cipherID)
	s.CipherSuite = &cipherStr

	// Default version to TLS1.2 legacy
	tls12 := "TLS1.2"
	s.TLSVersion = &tls12

	// Skip compression (1)
	if offset < len(body) {
		offset++
	}

	// Extensions
	if offset+2 > len(body) {
		return
	}
	extsLen := int(binary.BigEndian.Uint16(body[offset : offset+2]))
	offset += 2

	extsEnd := offset + extsLen
	if extsEnd > len(body) {
		extsEnd = len(body)
	}

	for offset+4 <= extsEnd {
		extType := binary.BigEndian.Uint16(body[offset : offset+2])
		extLen := int(binary.BigEndian.Uint16(body[offset+2 : offset+4]))
		offset += 4

		if offset+extLen > extsEnd {
			break
		}

		extData := body[offset : offset+extLen]
		offset += extLen

		// Ext 43: supported_versions
		if extType == 43 && len(extData) >= 2 {
			selectedVer := binary.BigEndian.Uint16(extData[:2])
			if selectedVer == 0x0304 {
				v := "TLS1.3"
				s.TLSVersion = &v
			} else if selectedVer == 0x0303 {
				v := "TLS1.2"
				s.TLSVersion = &v
			}
		}

		// Ext 51: key_share (selected group)
		if extType == 51 && len(extData) >= 2 {
			groupID := binary.BigEndian.Uint16(extData[:2])
			grpName, pqc := lookupGroup(groupID)
			s.KexGroup = &grpName
			s.PQCClass = &pqc
		}
	}
}

func (s *TLSConnectionState) parseCertificate(body []byte) {
	if len(body) < 6 {
		return
	}
	// Certificates length: 3 bytes
	firstCertLen := int(body[3])<<16 | int(body[4])<<8 | int(body[5])
	if len(body) < 6+firstCertLen {
		return
	}

	certDER := body[6 : 6+firstCertLen]
	fpBytes := sha256.Sum256(certDER)
	fp := hex.EncodeToString(fpBytes[:])
	s.PeerCertFP = &fp

	src := "observed"
	s.PeerCertSource = &src

	xCert, err := x509.ParseCertificate(certDER)
	if err == nil {
		s.PeerCertificate = certinfo.ExtractCertificate(xCert)
	}
}

// HandshakeSequenceString returns comma-separated handshake steps.
func (s *TLSConnectionState) HandshakeSequenceString() string {
	if len(s.HandshakeSeq) == 0 {
		return ""
	}
	return strings.Join(s.HandshakeSeq, ",")
}

func lookupCipherSuite(id uint16) string {
	switch id {
	// TLS 1.3
	case 0x1301:
		return "TLS_AES_128_GCM_SHA256"
	case 0x1302:
		return "TLS_AES_256_GCM_SHA384"
	case 0x1303:
		return "TLS_CHACHA20_POLY1305_SHA256"
	case 0x1304:
		return "TLS_AES_128_CCM_SHA256"
	case 0x1305:
		return "TLS_AES_128_CCM_8_SHA256"

	// TLS 1.2 ECDHE
	case 0xc02f:
		return "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
	case 0xc030:
		return "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"
	case 0xc02b:
		return "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"
	case 0xc02c:
		return "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384"
	case 0xcca8:
		return "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256"
	case 0xcca9:
		return "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"
	case 0xc013:
		return "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA"
	case 0xc014:
		return "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA"

	// RSA
	case 0x009c:
		return "TLS_RSA_WITH_AES_128_GCM_SHA256"
	case 0x009d:
		return "TLS_RSA_WITH_AES_256_GCM_SHA384"
	case 0x002f:
		return "TLS_RSA_WITH_AES_128_CBC_SHA"
	case 0x0035:
		return "TLS_RSA_WITH_AES_256_CBC_SHA"

	default:
		return fmt.Sprintf("TLS_0x%04X", id)
	}
}

func lookupGroup(id uint16) (string, string) {
	switch id {
	// PQC Hybrids
	case 0x11EC:
		return "X25519MLKEM768", "hybrid"
	case 0x6399:
		return "X25519Kyber768Draft00", "hybrid"
	case 0x2F1F:
		return "SecP256r1MLKEM768", "hybrid"

	// Classical
	case 0x001d:
		return "x25519", "classical"
	case 0x0017:
		return "secp256r1", "classical"
	case 0x0018:
		return "secp384r1", "classical"
	case 0x001e:
		return "x448", "classical"

	default:
		return fmt.Sprintf("group_0x%04X", id), "unknown"
	}
}

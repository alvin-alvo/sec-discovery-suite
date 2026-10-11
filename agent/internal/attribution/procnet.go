package attribution

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProcessInfo represents attribution metadata for a network connection per docs/06.
type ProcessInfo struct {
	PID         *int    `json:"pid"`
	Exe         *string `json:"exe"`
	Comm        *string `json:"comm"`
	Unit        *string `json:"unit"`
	Attribution string  `json:"attribution"` // proc_fd | unavailable
}

// FourTuple uniquely identifies a TCP connection endpoint pair.
type FourTuple struct {
	LocalIP    string
	LocalPort  int
	RemoteIP   string
	RemotePort int
}

// ProcNetResolver caches inode and PID lookups.
type ProcNetResolver struct {
	mu           sync.Mutex
	inodeToPID   map[uint64]int
	lastFDScan   time.Time
	fdCacheTTL   time.Duration
}

var defaultResolver = NewProcNetResolver()

func NewProcNetResolver() *ProcNetResolver {
	return &ProcNetResolver{
		inodeToPID: make(map[uint64]int),
		fdCacheTTL: 5 * time.Second,
	}
}

// ParseHexAddress decodes a /proc/net/tcp{,6} hex address string like "0100007F:0050".
func ParseHexAddress(hexAddr string) (string, int, error) {
	parts := strings.Split(hexAddr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address format: %s", hexAddr)
	}

	port64, err := strconv.ParseUint(parts[1], 16, 16)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port hex: %w", err)
	}
	port := int(port64)

	ipHex := parts[0]
	if len(ipHex) == 8 {
		// IPv4: 4 bytes in little-endian order (e.g. 0100007F -> 127.0.0.1)
		b, err := hex.DecodeString(ipHex)
		if err != nil || len(b) != 4 {
			return "", 0, fmt.Errorf("invalid ipv4 hex %q: %v", ipHex, err)
		}
		ip := net.IPv4(b[3], b[2], b[1], b[0])
		return ip.String(), port, nil
	} else if len(ipHex) == 32 {
		// IPv6: four 32-bit words, each little-endian on x86
		b, err := hex.DecodeString(ipHex)
		if err != nil || len(b) != 16 {
			return "", 0, fmt.Errorf("invalid ipv6 hex %q: %v", ipHex, err)
		}
		var ipBytes [16]byte
		for i := 0; i < 4; i++ {
			w := binary.LittleEndian.Uint32(b[i*4 : (i+1)*4])
			binary.BigEndian.PutUint32(ipBytes[i*4:(i+1)*4], w)
		}
		ip := net.IP(ipBytes[:])
		return ip.String(), port, nil
	}

	return "", 0, fmt.Errorf("unsupported ip hex length: %d", len(ipHex))
}

// ResolveConnection maps a 4-tuple to process attribution info via /proc/net/tcp and /proc/*/fd.
func ResolveConnection(localIP string, localPort int, remoteIP string, remotePort int) *ProcessInfo {
	return defaultResolver.Resolve(localIP, localPort, remoteIP, remotePort)
}

func (r *ProcNetResolver) Resolve(localIP string, localPort int, remoteIP string, remotePort int) *ProcessInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	inode := r.findInode(localIP, localPort, remoteIP, remotePort)
	if inode == 0 {
		return &ProcessInfo{Attribution: "unavailable"}
	}

	pid, ok := r.inodeToPID[inode]
	if !ok || time.Since(r.lastFDScan) > r.fdCacheTTL {
		// Cache miss or expired: scan /proc/*/fd
		r.scanProcFD()
		pid, ok = r.inodeToPID[inode]
	}

	if !ok || pid <= 0 {
		return &ProcessInfo{Attribution: "unavailable"}
	}

	exe := getProcessExe(pid)
	comm := getProcessComm(pid)
	unit := getProcessUnit(pid)

	return &ProcessInfo{
		PID:         &pid,
		Exe:         exe,
		Comm:        comm,
		Unit:        unit,
		Attribution: "proc_fd",
	}
}

func (r *ProcNetResolver) findInode(localIP string, localPort int, remoteIP string, remotePort int) uint64 {
	// Check /proc/net/tcp then /proc/net/tcp6
	for _, procFile := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		inode := scanProcNetFile(procFile, localIP, localPort, remoteIP, remotePort)
		if inode != 0 {
			return inode
		}
	}
	return 0
}

func scanProcNetFile(filePath, targetLIP string, targetLPort int, targetRIP string, targetRPort int) uint64 {
	f, err := os.Open(filePath)
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return 0 // skip header
	}

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}

		lIP, lPort, err := ParseHexAddress(fields[1])
		if err != nil {
			continue
		}
		rIP, rPort, err := ParseHexAddress(fields[2])
		if err != nil {
			continue
		}

		// Match either forward or inverted
		matchDirect := (lIP == targetLIP && lPort == targetLPort && (targetRPort == 0 || (rIP == targetRIP && rPort == targetRPort)))
		matchReverse := (rIP == targetLIP && rPort == targetLPort && (targetRPort == 0 || (lIP == targetRIP && lPort == targetRPort)))

		if matchDirect || matchReverse {
			inode, err := strconv.ParseUint(fields[9], 10, 64)
			if err == nil && inode != 0 {
				return inode
			}
		}
	}
	return 0
}

func (r *ProcNetResolver) scanProcFD() {
	r.lastFDScan = time.Now()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}

			// socket:[12345]
			if strings.HasPrefix(link, "socket:[") && strings.HasSuffix(link, "]") {
				inodeStr := link[8 : len(link)-1]
				inode, err := strconv.ParseUint(inodeStr, 10, 64)
				if err == nil {
					r.inodeToPID[inode] = pid
				}
			}
		}
	}
}

func getProcessComm(pid int) *string {
	comm, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
	if err == nil {
		s := strings.TrimSpace(string(comm))
		if s != "" {
			return &s
		}
	}
	return nil
}

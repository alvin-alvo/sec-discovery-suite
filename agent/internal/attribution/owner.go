package attribution

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Owner represents the owner object in docs/06.
type Owner struct {
	PID        *int    `json:"pid"`
	Proc       *string `json:"proc"`
	Unit       *string `json:"unit"`
	Confidence string  `json:"confidence"` // high | medium | low
}

// Resolver performs efficient single-pass procfs inspection to attribute files.
type Resolver struct {
	mu        sync.RWMutex
	openFiles map[string]*Owner
	cmdlines  map[string]*Owner
}

// NewResolver scans /proc once and indexes open FDs and cmdlines.
func NewResolver() *Resolver {
	r := &Resolver{
		openFiles: make(map[string]*Owner),
		cmdlines:  make(map[string]*Owner),
	}
	r.refresh()
	return r
}

func (r *Resolver) refresh() {
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

		var procName *string
		var unitName *string

		// 1. Index open file descriptors
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err == nil {
			for _, fd := range fds {
				link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
				if err != nil || link == "" {
					continue
				}
				if procName == nil {
					procName = getProcessExe(pid)
					unitName = getProcessUnit(pid)
				}
				r.openFiles[link] = &Owner{
					PID:        &pid,
					Proc:       procName,
					Unit:       unitName,
					Confidence: "high",
				}
			}
		}

		// 2. Index cmdline
		cmdData, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err == nil && len(cmdData) > 0 {
			cmdStr := string(cmdData)
			if procName == nil {
				procName = getProcessExe(pid)
				unitName = getProcessUnit(pid)
			}
			r.cmdlines[cmdStr] = &Owner{
				PID:        &pid,
				Proc:       procName,
				Unit:       unitName,
				Confidence: "medium",
			}
		}
	}
}

// FindFileOwner attempts best-effort owner discovery for a file path.
func (r *Resolver) FindFileOwner(targetPath string) *Owner {
	cleanTarget := filepath.Clean(targetPath)
	realTarget, err := filepath.EvalSymlinks(cleanTarget)
	if err != nil {
		realTarget = cleanTarget
	}

	// 1. High confidence: /proc/*/fd
	r.mu.RLock()
	defer r.mu.RUnlock()

	if owner, ok := r.openFiles[cleanTarget]; ok {
		return owner
	}
	if owner, ok := r.openFiles[realTarget]; ok {
		return owner
	}

	// 2. Medium confidence: /proc/*/cmdline
	for cmdStr, owner := range r.cmdlines {
		if strings.Contains(cmdStr, cleanTarget) || strings.Contains(cmdStr, realTarget) {
			return owner
		}
	}

	// 3. Low confidence: known service directories
	return findKnownServiceOwner(cleanTarget)
}

// FindFileOwner standalone helper.
func FindFileOwner(targetPath string) *Owner {
	return NewResolver().FindFileOwner(targetPath)
}

func findKnownServiceOwner(path string) *Owner {
	switch {
	case strings.HasPrefix(path, "/etc/nginx"):
		proc := "/usr/sbin/nginx"
		unit := "nginx.service"
		return &Owner{
			PID:        nil,
			Proc:       &proc,
			Unit:       &unit,
			Confidence: "low",
		}
	case strings.HasPrefix(path, "/etc/apache2") || strings.HasPrefix(path, "/etc/httpd"):
		proc := "/usr/sbin/apache2"
		unit := "apache2.service"
		return &Owner{
			PID:        nil,
			Proc:       &proc,
			Unit:       &unit,
			Confidence: "low",
		}
	case strings.HasPrefix(path, "/etc/letsencrypt"):
		proc := "/usr/bin/certbot"
		unit := "certbot.timer"
		return &Owner{
			PID:        nil,
			Proc:       &proc,
			Unit:       &unit,
			Confidence: "low",
		}
	default:
		return nil
	}
}

func getProcessExe(pid int) *string {
	link, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err == nil && link != "" {
		return &link
	}
	comm, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
	if err == nil {
		s := strings.TrimSpace(string(comm))
		if s != "" {
			return &s
		}
	}
	return nil
}

func getProcessUnit(pid int) *string {
	cgroup, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return nil
	}

	lines := bytes.Split(cgroup, []byte("\n"))
	for _, line := range lines {
		s := string(line)
		parts := strings.Split(s, "/")
		for _, part := range parts {
			if strings.HasSuffix(part, ".service") || strings.HasSuffix(part, ".timer") {
				unit := strings.TrimSpace(part)
				return &unit
			}
		}
	}
	return nil
}

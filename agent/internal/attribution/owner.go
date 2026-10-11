package attribution

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Owner represents the owner object in docs/06.
type Owner struct {
	PID        *int    `json:"pid"`
	Proc       *string `json:"proc"`
	Unit       *string `json:"unit"`
	Confidence string  `json:"confidence"` // high | medium | low
}

// FindFileOwner attempts best-effort owner discovery for a file path per docs/02.
// Confidence levels:
// - high: process holds file open (/proc/*/fd)
// - medium: path appears in process cmdline (/proc/*/cmdline)
// - low: path under known service config dir (e.g. /etc/nginx)
func FindFileOwner(targetPath string) *Owner {
	cleanTarget := filepath.Clean(targetPath)
	realTarget, err := filepath.EvalSymlinks(cleanTarget)
	if err != nil {
		realTarget = cleanTarget
	}

	// 1. High confidence: /proc/*/fd
	if owner := findOpenFDOwner(cleanTarget, realTarget); owner != nil {
		return owner
	}

	// 2. Medium confidence: /proc/*/cmdline
	if owner := findCmdlineOwner(cleanTarget, realTarget); owner != nil {
		return owner
	}

	// 3. Low confidence: known service directories
	if owner := findKnownServiceOwner(cleanTarget); owner != nil {
		return owner
	}

	return nil
}

func findOpenFDOwner(cleanTarget, realTarget string) *Owner {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
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
			if link == cleanTarget || link == realTarget {
				procName := getProcessExe(pid)
				unitName := getProcessUnit(pid)
				return &Owner{
					PID:        &pid,
					Proc:       procName,
					Unit:       unitName,
					Confidence: "high",
				}
			}
		}
	}
	return nil
}

func findCmdlineOwner(cleanTarget, realTarget string) *Owner {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		cmdData, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil || len(cmdData) == 0 {
			continue
		}

		cmdStr := string(cmdData)
		if strings.Contains(cmdStr, cleanTarget) || strings.Contains(cmdStr, realTarget) {
			procName := getProcessExe(pid)
			unitName := getProcessUnit(pid)
			return &Owner{
				PID:        &pid,
				Proc:       procName,
				Unit:       unitName,
				Confidence: "medium",
			}
		}
	}
	return nil
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
		// e.g. 0::/system.slice/nginx.service
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

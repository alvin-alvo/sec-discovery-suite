package attribution

import (
	"testing"
)

func TestParseHexAddress_IPv4(t *testing.T) {
	// 0100007F:0050 -> 127.0.0.1:80
	ip, port, err := ParseHexAddress("0100007F:0050")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1, got %s", ip)
	}
	if port != 80 {
		t.Errorf("expected 80, got %d", port)
	}

	// 0F02000A:01BB -> 10.0.2.15:443
	ip, port, err = ParseHexAddress("0F02000A:01BB")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.2.15" {
		t.Errorf("expected 10.0.2.15, got %s", ip)
	}
	if port != 443 {
		t.Errorf("expected 443, got %d", port)
	}
}

func TestParseHexAddress_IPv6(t *testing.T) {
	// Loopback ::1 on 443
	// ::1 in /proc/net/tcp6 is 00000000000000000000000001000000:01BB
	ip, port, err := ParseHexAddress("00000000000000000000000001000000:01BB")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "::1" {
		t.Errorf("expected ::1, got %s", ip)
	}
	if port != 443 {
		t.Errorf("expected 443, got %d", port)
	}
}

func TestParseHexAddress_Invalid(t *testing.T) {
	_, _, err := ParseHexAddress("notanaddress")
	if err == nil {
		t.Error("expected error for malformed address")
	}

	_, _, err = ParseHexAddress("0100007F:ZZZZ")
	if err == nil {
		t.Error("expected error for invalid port")
	}
}

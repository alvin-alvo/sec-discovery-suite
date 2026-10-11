package runs

import (
	"crypto/rand"
	"encoding/binary"
	"io"
	"sync"
	"time"
)

// Crockford's Base32 character set
const encoding = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var (
	ulidMu       sync.Mutex
	lastTime     uint64
	lastEntropy  [10]byte
)

// NewULID generates a 26-character monotonically increasing ULID string.
func NewULID() string {
	return NewULIDAt(time.Now())
}

// NewULIDAt generates a ULID for a specific time.
func NewULIDAt(t time.Time) string {
	ulidMu.Lock()
	defer ulidMu.Unlock()

	ms := uint64(t.UnixMilli())
	var entropy [10]byte

	if ms == lastTime {
		// Increment entropy for monotonic ordering in same millisecond
		entropy = lastEntropy
		for i := len(entropy) - 1; i >= 0; i-- {
			entropy[i]++
			if entropy[i] != 0 {
				break
			}
		}
	} else {
		_, _ = io.ReadFull(rand.Reader, entropy[:])
		lastTime = ms
	}
	lastEntropy = entropy

	var raw [16]byte
	// 48 bits of timestamp
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	copy(raw[6:], entropy[:])

	return encodeULID(raw)
}

func encodeULID(raw [16]byte) string {
	var dst [26]byte

	// 128 bits into 26 base32 (5-bit) chars
	// Crockford Base32 encoding
	dst[0] = encoding[(raw[0]&224)>>5]
	dst[1] = encoding[raw[0]&31]
	dst[2] = encoding[(raw[1]&248)>>3]
	dst[3] = encoding[((raw[1]&7)<<2)|((raw[2]&192)>>6)]
	dst[4] = encoding[(raw[2]&62)>>1]
	dst[5] = encoding[((raw[2]&1)<<4)|((raw[3]&240)>>4)]
	dst[6] = encoding[((raw[3]&15)<<1)|((raw[4]&128)>>7)]
	dst[7] = encoding[(raw[4]&124)>>2]
	dst[8] = encoding[((raw[4]&3)<<3)|((raw[5]&224)>>5)]
	dst[9] = encoding[raw[5]&31]

	dst[10] = encoding[(raw[6]&248)>>3]
	dst[11] = encoding[((raw[6]&7)<<2)|((raw[7]&192)>>6)]
	dst[12] = encoding[(raw[7]&62)>>1]
	dst[13] = encoding[((raw[7]&1)<<4)|((raw[8]&240)>>4)]
	dst[14] = encoding[((raw[8]&15)<<1)|((raw[9]&128)>>7)]
	dst[15] = encoding[(raw[9]&124)>>2]
	dst[16] = encoding[((raw[9]&3)<<3)|((raw[10]&224)>>5)]
	dst[17] = encoding[raw[10]&31]

	dst[18] = encoding[(raw[11]&248)>>3]
	dst[19] = encoding[((raw[11]&7)<<2)|((raw[12]&192)>>6)]
	dst[20] = encoding[(raw[12]&62)>>1]
	dst[21] = encoding[((raw[12]&1)<<4)|((raw[13]&240)>>4)]
	dst[22] = encoding[((raw[13]&15)<<1)|((raw[14]&128)>>7)]
	dst[23] = encoding[(raw[14]&124)>>2]
	dst[24] = encoding[((raw[14]&3)<<3)|((raw[15]&224)>>5)]
	dst[25] = encoding[raw[15]&31]

	return string(dst[:])
}

// Unused import guard
var _ = binary.BigEndian

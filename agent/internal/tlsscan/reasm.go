package tlsscan

import (
	"bytes"
)

const maxReasmBuffer = 16384 // 16 KB per flow bounded limit per docs/02

// FlowReassembler buffers raw TCP payload bytes up to a strict bound.
type FlowReassembler struct {
	buf bytes.Buffer
}

func NewFlowReassembler() *FlowReassembler {
	return &FlowReassembler{}
}

// Append adds new incoming TCP payload bytes, guarding against memory bloat.
func (r *FlowReassembler) Append(data []byte) {
	if len(data) == 0 {
		return
	}
	if r.buf.Len()+len(data) > maxReasmBuffer {
		// Truncate to avoid memory exhaustion
		remaining := maxReasmBuffer - r.buf.Len()
		if remaining > 0 {
			r.buf.Write(data[:remaining])
		}
		return
	}
	r.buf.Write(data)
}

// Bytes returns the buffered bytes.
func (r *FlowReassembler) Bytes() []byte {
	return r.buf.Bytes()
}

// Discard advances the buffer by n bytes.
func (r *FlowReassembler) Discard(n int) {
	if n <= 0 {
		return
	}
	if n >= r.buf.Len() {
		r.buf.Reset()
		return
	}
	b := r.buf.Bytes()[n:]
	r.buf.Reset()
	r.buf.Write(b)
}

// Len returns current buffer length.
func (r *FlowReassembler) Len() int {
	return r.buf.Len()
}

// Reset clears the buffer.
func (r *FlowReassembler) Reset() {
	r.buf.Reset()
}

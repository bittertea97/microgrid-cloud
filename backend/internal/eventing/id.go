package eventing

import (
	"crypto/rand"
	"encoding/hex"
)

// NewEventID generates a random event identifier.
func NewEventID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return hex.EncodeToString(buf[:])
	}
	// UUIDv4 formatting (without external dependency).
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return hex.EncodeToString(buf[:])
}

func newEventID() string {
	return NewEventID()
}

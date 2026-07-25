package mcp

import (
	"encoding/hex"
	"fmt"
)

// PLC Data communication code.
// This item is operating byte order.
type Code int

const (
	// Ascii code is normal mode.
	// Stored from upper byte to lower byte.
	Ascii Code = iota

	//　Binary code is approximately half the amount of communication data compared to communication using ASCII code
	// Stored from lower byte to upper byte.
	Binary
)

func (c Code) EncodeHex(s string) ([]byte, error) {
	if c == Ascii {
		return []byte(s), nil
	}

	decode, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(decode)%2 != 0 {
		return nil, fmt.Errorf("binary word data must contain an even number of bytes")
	}

	for i := 0; i < len(decode); i += 2 {
		decode[i], decode[i+1] = decode[i+1], decode[i]
	}
	return decode, nil
}

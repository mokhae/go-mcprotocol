package mcp

import (
	"errors"
	"fmt"
)

var (
	ErrNotConnected     = errors.New("mcp client is not connected")
	ErrAlreadyConnected = errors.New("mcp client is already connected")
)

// ValidationError reports an invalid client configuration or command argument.
type ValidationError struct {
	Field  string
	Value  any
	Reason string
}

func (e *ValidationError) Error() string {
	if e.Value == nil {
		return fmt.Sprintf("invalid %s: %s", e.Field, e.Reason)
	}
	return fmt.Sprintf("invalid %s %v: %s", e.Field, e.Value, e.Reason)
}

// ProtocolError reports a malformed or unexpected MC protocol frame.
type ProtocolError struct {
	Reason string
}

func (e *ProtocolError) Error() string {
	return "invalid MC protocol response: " + e.Reason
}

// MCError reports an error response returned by the PLC.
type MCError struct {
	EndCode   uint16
	ErrorInfo []byte
}

func (e *MCError) Error() string {
	if len(e.ErrorInfo) == 0 {
		return fmt.Sprintf("PLC returned MC end code 0x%04X", e.EndCode)
	}
	return fmt.Sprintf("PLC returned MC end code 0x%04X (error information: %X)", e.EndCode, e.ErrorInfo)
}

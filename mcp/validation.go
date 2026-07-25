package mcp

import (
	"encoding/hex"
	"fmt"
)

const (
	maxDeviceAddress int64 = 0xFFFFFF
	maxWordPoints    int64 = 960
	maxBitPoints     int64 = 7168
)

func validateDeviceRequest(deviceName string, offset, numPoints, maxPoints int64) error {
	if _, ok := deviceCodes[deviceName]; !ok {
		return &ValidationError{Field: "deviceName", Value: deviceName, Reason: "is not a supported device code"}
	}
	if offset < 0 || offset > maxDeviceAddress {
		return &ValidationError{
			Field:  "offset",
			Value:  offset,
			Reason: fmt.Sprintf("must be between 0 and %d for a 3-byte device address", maxDeviceAddress),
		}
	}
	if numPoints < 1 || numPoints > maxPoints {
		return &ValidationError{
			Field:  "numPoints",
			Value:  numPoints,
			Reason: fmt.Sprintf("must be between 1 and %d", maxPoints),
		}
	}
	if numPoints-1 > maxDeviceAddress-offset {
		return &ValidationError{
			Field:  "numPoints",
			Value:  numPoints,
			Reason: "requested device range exceeds the 3-byte device address limit",
		}
	}
	return nil
}

func validateHexField(name, value string, size int) error {
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return &ValidationError{Field: name, Value: value, Reason: "must contain hexadecimal bytes"}
	}
	if len(decoded) != size {
		return &ValidationError{
			Field:  name,
			Value:  value,
			Reason: fmt.Sprintf("must contain exactly %d byte(s)", size),
		}
	}
	return nil
}

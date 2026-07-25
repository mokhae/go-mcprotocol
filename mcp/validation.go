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

// validateBitWriteRequest validates a batch write in bit units request.
// writeData holds 1 point per 4 bits, so it must be exactly (numPoints+1)/2 bytes
// and every 4-bit point must be 0 or 1.
func validateBitWriteRequest(deviceName string, offset, numPoints int64, writeData []byte) error {
	if err := validateDeviceRequest(deviceName, offset, numPoints, maxBitPoints); err != nil {
		return err
	}
	if !bitDeviceNames[deviceName] {
		return &ValidationError{
			Field:  "deviceName",
			Value:  deviceName,
			Reason: "is a word device, write in bit units accepts bit devices only",
		}
	}

	expectedLen := int((numPoints + 1) / 2)
	if len(writeData) != expectedLen {
		return &ValidationError{
			Field:  "writeData",
			Value:  len(writeData),
			Reason: fmt.Sprintf("length must be exactly %d bytes for %d bit points, 1 point per 4 bits", expectedLen, numPoints),
		}
	}
	for i, b := range writeData {
		if b&0xF0 > 0x10 || b&0x0F > 0x01 {
			return &ValidationError{
				Field:  "writeData",
				Value:  fmt.Sprintf("%#02x at index %d", b, i),
				Reason: "every 4-bit point must be 0 or 1",
			}
		}
	}
	// an odd point count leaves the last lower 4 bits as padding
	if numPoints%2 == 1 && writeData[expectedLen-1]&0x0F != 0x00 {
		return &ValidationError{
			Field:  "writeData",
			Value:  fmt.Sprintf("%#02x at index %d", writeData[expectedLen-1], expectedLen-1),
			Reason: fmt.Sprintf("lower 4 bits are padding for the odd point count %d so must be 0", numPoints),
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

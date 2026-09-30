package mcp

import (
	"encoding/hex"
	"fmt"
)

const (
	maxDeviceAddress int64 = 0xFFFFFF
	maxWordPoints    int64 = 960
	maxBitPoints     int64 = 7168
	// maxRandomBitPoints is a structural limit: a random write request carries the
	// number of points in a single byte. The CPU may accept fewer, and reports that
	// as an end code.
	maxRandomBitPoints = 255
	// maxRandomWordUnits bounds a random write in word units: the MELSEC
	// communication protocol reference allows 12 x word points + 14 x double
	// word points up to 1920 (160 words when writing words only).
	maxRandomWordUnits = 1920
	randomWordUnits    = 12
	randomDWordUnits   = 14
	// maxRandomWordPoints is the structural limit of each single-byte count.
	maxRandomWordPoints = 255
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

// validateBitWriteRandomRequest validates a random write in bit units request.
// Every point is addressed on its own, so each one is checked separately.
func validateBitWriteRandomRequest(points []BitPoint) error {
	if len(points) < 1 {
		return &ValidationError{
			Field:  "points",
			Value:  len(points),
			Reason: "must contain at least 1 device point",
		}
	}
	if len(points) > maxRandomBitPoints {
		return &ValidationError{
			Field:  "points",
			Value:  len(points),
			Reason: fmt.Sprintf("must not exceed %d, the request carries the number of points in a single byte", maxRandomBitPoints),
		}
	}
	for i, p := range points {
		if err := validateDeviceRequest(p.DeviceName, p.Offset, 1, maxBitPoints); err != nil {
			return fmt.Errorf("points[%d]: %w", i, err)
		}
		if !bitDeviceNames[p.DeviceName] {
			return &ValidationError{
				Field:  fmt.Sprintf("points[%d].DeviceName", i),
				Value:  p.DeviceName,
				Reason: "is a word device, random write in bit units accepts bit devices only",
			}
		}
	}
	return nil
}

// validateWordWriteRandomRequest validates a random write in word units request.
// Every point is addressed on its own, so each one is checked separately.
func validateWordWriteRandomRequest(words []WordPoint, dwords []DWordPoint) error {
	if len(words)+len(dwords) < 1 {
		return &ValidationError{Field: "points", Value: 0, Reason: "must contain at least 1 word or double word point"}
	}
	if len(words) > maxRandomWordPoints || len(dwords) > maxRandomWordPoints {
		return &ValidationError{
			Field:  "points",
			Value:  fmt.Sprintf("%d words, %d double words", len(words), len(dwords)),
			Reason: fmt.Sprintf("each count must not exceed %d, the request carries it in a single byte", maxRandomWordPoints),
		}
	}
	if units := randomWordUnits*len(words) + randomDWordUnits*len(dwords); units > maxRandomWordUnits {
		return &ValidationError{
			Field:  "points",
			Value:  units,
			Reason: fmt.Sprintf("12 x words + 14 x double words must not exceed %d", maxRandomWordUnits),
		}
	}
	check := func(field, deviceName string, offset, points int64) error {
		if err := validateDeviceRequest(deviceName, offset, points, maxWordPoints); err != nil {
			return fmt.Errorf("%s: %w", field, err)
		}
		if bitDeviceNames[deviceName] {
			return &ValidationError{
				Field:  field + ".DeviceName",
				Value:  deviceName,
				Reason: "is a bit device, random write in word units accepts word devices only",
			}
		}
		return nil
	}
	for i, p := range words {
		if err := check(fmt.Sprintf("words[%d]", i), p.DeviceName, p.Offset, 1); err != nil {
			return err
		}
	}
	for i, p := range dwords {
		if err := check(fmt.Sprintf("dwords[%d]", i), p.DeviceName, p.Offset, 2); err != nil {
			return err
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

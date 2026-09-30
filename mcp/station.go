package mcp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	SUB_HEADER = "5000" // 3Eフレームでは固定

	HEALTH_CHECK_COMMAND    = "1906" // binary mode expression. if ascii mode then 0619
	HEALTH_CHECK_SUBCOMMAND = "0000"

	READ_COMMAND         = "0104" // binary mode expression. if ascii mode then 0401
	READ_SUB_COMMAND     = "0000"
	BIT_READ_SUB_COMMAND = "0100"

	WRITE_COMMAND         = "0114" // binary mode expression. if ascii mode then 1401
	WRITE_SUB_COMMAND     = "0000"
	BIT_WRITE_SUB_COMMAND = "0100" // binary mode expression. if ascii mode then 0001

	RANDOM_WRITE_COMMAND          = "0214" // binary mode expression. if ascii mode then 1402
	RANDOM_BIT_WRITE_SUB_COMMAND  = "0100" // binary mode expression. if ascii mode then 0001
	RANDOM_WORD_WRITE_SUB_COMMAND = "0000" // binary mode expression. if ascii mode then 0000

	MONITORING_TIMER = "1000" // 3[sec]
)

// BitPoint is a single device point of a random write in bit units.
// DeviceName must be a bit device such as 'M', Offset is the device number and
// Value is the state to write, true for ON.
type BitPoint struct {
	DeviceName string
	Offset     int64
	Value      bool
}

// WordPoint is a single word of a random write in word units.
// DeviceName must be a word device such as 'D', Offset is the device number.
type WordPoint struct {
	DeviceName string
	Offset     int64
	Value      uint16
}

// DWordPoint is a double word (two consecutive devices, low word first) of a
// random write in word units.
type DWordPoint struct {
	DeviceName string
	Offset     int64
	Value      uint32
}

// bitDeviceNames is the set of device names that can be accessed in bit units.
// The MC protocol bit unit commands (1401/0001, 1402/0001) carry no bit position
// field, only a head device number and a device code, so word devices such as
// D / W / ZR cannot be addressed one bit at a time.
var bitDeviceNames = map[string]bool{
	"X": true,
	"Y": true,
	"M": true,
	"L": true,
	"F": true,
	"V": true,
	"B": true,
}

// deviceCodes is device name and hex value map
var deviceCodes = map[string]string{
	"X":  "9C",
	"Y":  "9D",
	"M":  "90",
	"L":  "92",
	"F":  "93",
	"V":  "94",
	"B":  "A0",
	"W":  "B4",
	"D":  "A8",
	"ZR": "B0",
}

// Each single PLC that is connected on MELSECNET and CC-Link IE is called a station.
type station struct {
	// PLC Network number
	networkNum string
	// PC Number
	pcNum string
	// PLC stn Unit I/O Number
	unitIONum string
	// PLC stn Unit Station Number
	unitStationNum string
}

func (h *station) validate() error {
	if h == nil {
		return &ValidationError{Field: "station", Reason: "must not be nil"}
	}
	fields := []struct {
		name  string
		value string
		size  int
	}{
		{name: "networkNum", value: h.networkNum, size: 1},
		{name: "pcNum", value: h.pcNum, size: 1},
		{name: "unitIONum", value: h.unitIONum, size: 2},
		{name: "unitStationNum", value: h.unitStationNum, size: 1},
	}
	for _, field := range fields {
		if err := validateHexField(field.name, field.value, field.size); err != nil {
			return err
		}
	}
	return nil
}

func NewStation(networkNum, pcNum, unitIONum, unitStationNum string) *station {
	return &station{
		networkNum:     networkNum,
		pcNum:          pcNum,
		unitIONum:      unitIONum,
		unitStationNum: unitStationNum,
	}
}

// local stn stn. local stn is 自局.
func NewLocalStation() *station {
	return &station{
		networkNum:     "00",   // 自局の場合は00固定
		pcNum:          "FF",   // 自局の場合はFF固定
		unitIONum:      "FF03", // マルチドロップ接続などでない場合はFF03固定値
		unitStationNum: "00",   // マルチドロップ接続などでない場合は00固定値
	}
}

func (h *station) BuildHealthCheckRequest() string {

	returnDataNum := "0500"    // 5 device. if ascii mode then 0005
	returnData := "4142434445" // value is "ABCDE".

	requestStr := HEALTH_CHECK_COMMAND + HEALTH_CHECK_SUBCOMMAND + returnDataNum + returnData

	// data length
	requestCharLen := len(MONITORING_TIMER+requestStr) / 2 // 1byte=2char
	dataLenBuff := new(bytes.Buffer)
	_ = binary.Write(dataLenBuff, binary.LittleEndian, int64(requestCharLen))
	dataLen := fmt.Sprintf("%X", dataLenBuff.Bytes()[0:2]) // 2byte固定

	return SUB_HEADER +
		h.networkNum +
		h.pcNum +
		h.unitIONum +
		h.unitStationNum +
		dataLen +
		MONITORING_TIMER +
		requestStr
}

// BuildReadRequest represents MCP read as word command.
// deviceName is device code name like 'D' register.
// offset is device offset addr.
// numPoints is number of read device points.
func (h *station) BuildReadRequest(deviceName string, offset, numPoints int64) string {

	// get device symbol hex layout
	deviceCode := deviceCodes[deviceName]

	// offset convert to little endian layout
	// MELSECコミュニケーションプロトコル リファレンス(p67) MELSEC-Q/L: 3[byte], MELSEC iQ-R: 4[byte]
	offsetBuff := new(bytes.Buffer)
	_ = binary.Write(offsetBuff, binary.LittleEndian, offset)
	offsetHex := fmt.Sprintf("%X", offsetBuff.Bytes()[0:3]) // 仮にQシリーズとするので3byte trim

	// read points
	pointsBuff := new(bytes.Buffer)
	_ = binary.Write(pointsBuff, binary.LittleEndian, numPoints)
	points := fmt.Sprintf("%X", pointsBuff.Bytes()[0:2]) // 2byte固定

	// data length
	requestCharLen := len(MONITORING_TIMER+READ_COMMAND+READ_SUB_COMMAND+deviceCode+offsetHex+points) / 2 // 1byte=2char
	dataLenBuff := new(bytes.Buffer)
	_ = binary.Write(dataLenBuff, binary.LittleEndian, int64(requestCharLen))
	dataLen := fmt.Sprintf("%X", dataLenBuff.Bytes()[0:2]) // 2byte固定

	return SUB_HEADER +
		h.networkNum +
		h.pcNum +
		h.unitIONum +
		h.unitStationNum +
		dataLen +
		MONITORING_TIMER +
		READ_COMMAND +
		READ_SUB_COMMAND +
		offsetHex +
		deviceCode +
		points
}

// BuildReadRequest represents MCP read as bit command.
// deviceName is device code name like 'D' register.
// offset is device offset addr.
// numPoints is number of read device points.
func (h *station) BuildBitReadRequest(deviceName string, offset, numPoints int64) string {

	// get device symbol hex layout
	deviceCode := deviceCodes[deviceName]

	// offset convert to little endian layout
	// MELSECコミュニケーションプロトコル リファレンス(p67) MELSEC-Q/L: 3[byte], MELSEC iQ-R: 4[byte]
	offsetBuff := new(bytes.Buffer)
	_ = binary.Write(offsetBuff, binary.LittleEndian, offset)
	offsetHex := fmt.Sprintf("%X", offsetBuff.Bytes()[0:3]) // 仮にQシリーズとするので3byte trim

	// read points
	pointsBuff := new(bytes.Buffer)
	_ = binary.Write(pointsBuff, binary.LittleEndian, numPoints)
	points := fmt.Sprintf("%X", pointsBuff.Bytes()[0:2]) // 2byte固定

	// data length
	requestCharLen := len(MONITORING_TIMER+READ_COMMAND+BIT_READ_SUB_COMMAND+deviceCode+offsetHex+points) / 2 // 1byte=2char
	dataLenBuff := new(bytes.Buffer)
	_ = binary.Write(dataLenBuff, binary.LittleEndian, int64(requestCharLen))
	dataLen := fmt.Sprintf("%X", dataLenBuff.Bytes()[0:2]) // 2byte固定

	return SUB_HEADER +
		h.networkNum +
		h.pcNum +
		h.unitIONum +
		h.unitStationNum +
		dataLen +
		MONITORING_TIMER +
		READ_COMMAND +
		BIT_READ_SUB_COMMAND +
		offsetHex +
		deviceCode +
		points
}

// BuildWriteRequest represents MCP write command.
// deviceName is device code name like 'D' register.
// offset is device offset addr.
// writeData is data to write.
// numPoints is number of write device points.
// writeData is the data to be written. If writeData is larger than 2*numPoints bytes,
// data larger than 2*numPoints bytes is ignored.
func (h *station) BuildWriteRequest(deviceName string, offset, numPoints int64, writeData []byte) string {

	// get device symbol hex layout
	deviceCode := deviceCodes[deviceName]

	// offset convert to little endian layout
	// MELSECコミュニケーションプロトコル リファレンス(p67) MELSEC-Q/L: 3[byte], MELSEC iQ-R: 4[byte]
	offsetBuff := new(bytes.Buffer)
	_ = binary.Write(offsetBuff, binary.LittleEndian, offset)
	offsetHex := fmt.Sprintf("%X", offsetBuff.Bytes()[0:3]) // 仮にQシリーズとするので3byte trim

	// convert write data to little endian word
	writeBuff := new(bytes.Buffer)
	_ = binary.Write(writeBuff, binary.LittleEndian, writeData)
	writeHex := fmt.Sprintf("%X", writeBuff.Bytes()[0:2*numPoints]) // 2 byte per 1 device point

	// write points
	pointsBuff := new(bytes.Buffer)
	_ = binary.Write(pointsBuff, binary.LittleEndian, numPoints)
	points := fmt.Sprintf("%X", pointsBuff.Bytes()[0:2]) // 2byte固定

	// data length
	requestCharLen := len(MONITORING_TIMER+WRITE_COMMAND+WRITE_SUB_COMMAND+deviceCode+offsetHex+points+writeHex) / 2 // 1byte=2char
	dataLenBuff := new(bytes.Buffer)
	_ = binary.Write(dataLenBuff, binary.LittleEndian, int64(requestCharLen))
	dataLen := fmt.Sprintf("%X", dataLenBuff.Bytes()[0:2]) // 2byte固定
	return SUB_HEADER +
		h.networkNum +
		h.pcNum +
		h.unitIONum +
		h.unitStationNum +
		dataLen +
		MONITORING_TIMER +
		WRITE_COMMAND +
		WRITE_SUB_COMMAND +
		offsetHex +
		deviceCode +
		points +
		writeHex
}

// PackBits converts one bool per point into the 4bit-per-point layout that the
// MC protocol batch write in bit units expects.
//
// The 1st point goes to the upper 4 bits of the 1st byte, the 2nd point to the
// lower 4 bits, and so on. When the number of points is odd the lower 4 bits of
// the last byte are filled with 0.
//
//	PackBits([]bool{true, false, true, false, false, false, true, true})
//	// -> []byte{0x10, 0x10, 0x00, 0x11}
func PackBits(bits []bool) []byte {
	packed := make([]byte, (len(bits)+1)/2)
	for i, b := range bits {
		if !b {
			continue
		}
		if i%2 == 0 {
			packed[i/2] |= 0x10 // upper 4 bits
		} else {
			packed[i/2] |= 0x01 // lower 4 bits
		}
	}
	return packed
}

// BuildBitWriteRequest represents MCP batch write in bit units command.
// deviceName is a bit device code name like 'M'.
// offset is device offset addr.
// numPoints is number of write device points, in bits.
// writeData is the write data in the 4bit-per-point layout, so it holds
// (numPoints+1)/2 bytes. Use PackBits to build it from []bool.
// Arguments are validated by validateBitWriteRequest before this is called.
func (h *station) BuildBitWriteRequest(deviceName string, offset, numPoints int64, writeData []byte) string {

	// get device symbol hex layout
	deviceCode := deviceCodes[deviceName]

	// offset convert to little endian layout
	// MELSECコミュニケーションプロトコル リファレンス(p67) MELSEC-Q/L: 3[byte], MELSEC iQ-R: 4[byte]
	offsetBuff := new(bytes.Buffer)
	_ = binary.Write(offsetBuff, binary.LittleEndian, offset)
	offsetHex := fmt.Sprintf("%X", offsetBuff.Bytes()[0:3]) // 仮にQシリーズとするので3byte trim

	// 1 point per 4 bits, so 2 points are packed into 1 byte
	writeHex := fmt.Sprintf("%X", writeData)

	// write points
	pointsBuff := new(bytes.Buffer)
	_ = binary.Write(pointsBuff, binary.LittleEndian, numPoints)
	points := fmt.Sprintf("%X", pointsBuff.Bytes()[0:2]) // 2byte固定

	// data length
	requestCharLen := len(MONITORING_TIMER+WRITE_COMMAND+BIT_WRITE_SUB_COMMAND+deviceCode+offsetHex+points+writeHex) / 2 // 1byte=2char
	dataLenBuff := new(bytes.Buffer)
	_ = binary.Write(dataLenBuff, binary.LittleEndian, int64(requestCharLen))
	dataLen := fmt.Sprintf("%X", dataLenBuff.Bytes()[0:2]) // 2byte固定

	return SUB_HEADER +
		h.networkNum +
		h.pcNum +
		h.unitIONum +
		h.unitStationNum +
		dataLen +
		MONITORING_TIMER +
		WRITE_COMMAND +
		BIT_WRITE_SUB_COMMAND +
		offsetHex +
		deviceCode +
		points +
		writeHex
}

// BuildBitWriteRandomRequest represents MCP random write in bit units command,
// called 'test' in the manual.
//
// Unlike the batch commands the points are not a contiguous range, so each point
// carries its own device: device number 3[byte] + device code 1[byte] + value
// 1[byte], where the value is 01 for ON and 00 for OFF. The number of points is
// sent as a single byte, so at most maxRandomBitPoints points fit in one request.
// Arguments are validated by validateBitWriteRandomRequest before this is called.
func (h *station) BuildBitWriteRandomRequest(points []BitPoint) string {

	// number of bit access points is 1[byte], unlike the 2[byte] points of the batch commands
	pointsHex := fmt.Sprintf("%02X", len(points))

	var body strings.Builder
	for _, p := range points {
		// offset convert to little endian layout
		// MELSECコミュニケーションプロトコル リファレンス(p67) MELSEC-Q/L: 3[byte], MELSEC iQ-R: 4[byte]
		offsetBuff := new(bytes.Buffer)
		_ = binary.Write(offsetBuff, binary.LittleEndian, p.Offset)
		body.WriteString(fmt.Sprintf("%X", offsetBuff.Bytes()[0:3])) // 仮にQシリーズとするので3byte trim
		body.WriteString(deviceCodes[p.DeviceName])
		if p.Value {
			body.WriteString("01")
		} else {
			body.WriteString("00")
		}
	}
	bodyHex := body.String()

	// data length
	requestCharLen := len(MONITORING_TIMER+RANDOM_WRITE_COMMAND+RANDOM_BIT_WRITE_SUB_COMMAND+pointsHex+bodyHex) / 2 // 1byte=2char
	dataLenBuff := new(bytes.Buffer)
	_ = binary.Write(dataLenBuff, binary.LittleEndian, int64(requestCharLen))
	dataLen := fmt.Sprintf("%X", dataLenBuff.Bytes()[0:2]) // 2byte固定

	return SUB_HEADER +
		h.networkNum +
		h.pcNum +
		h.unitIONum +
		h.unitStationNum +
		dataLen +
		MONITORING_TIMER +
		RANDOM_WRITE_COMMAND +
		RANDOM_BIT_WRITE_SUB_COMMAND +
		pointsHex +
		bodyHex
}

// BuildWordWriteRandomRequest represents MCP random write in word units command
// (1402/0000). Word points and double word points are counted separately, one
// byte each, then every point carries device number 3[byte] + device code
// 1[byte] + value (2[byte] for a word, 4[byte] for a double word), little endian.
// Arguments are validated by validateWordWriteRandomRequest before this is called.
func (h *station) BuildWordWriteRandomRequest(words []WordPoint, dwords []DWordPoint) string {

	// the numbers of word and double word access points are 1[byte] each
	countsHex := fmt.Sprintf("%02X%02X", len(words), len(dwords))

	var body strings.Builder
	writePoint := func(deviceName string, offset int64, value []byte) {
		// offset convert to little endian layout
		// MELSECコミュニケーションプロトコル リファレンス(p67) MELSEC-Q/L: 3[byte], MELSEC iQ-R: 4[byte]
		offsetBuff := new(bytes.Buffer)
		_ = binary.Write(offsetBuff, binary.LittleEndian, offset)
		body.WriteString(fmt.Sprintf("%X", offsetBuff.Bytes()[0:3])) // 仮にQシリーズとするので3byte trim
		body.WriteString(deviceCodes[deviceName])
		body.WriteString(fmt.Sprintf("%X", value))
	}
	for _, p := range words {
		value := make([]byte, 2)
		binary.LittleEndian.PutUint16(value, p.Value)
		writePoint(p.DeviceName, p.Offset, value)
	}
	for _, p := range dwords {
		value := make([]byte, 4)
		binary.LittleEndian.PutUint32(value, p.Value)
		writePoint(p.DeviceName, p.Offset, value)
	}
	bodyHex := body.String()

	// data length
	requestCharLen := len(MONITORING_TIMER+RANDOM_WRITE_COMMAND+RANDOM_WORD_WRITE_SUB_COMMAND+countsHex+bodyHex) / 2 // 1byte=2char
	dataLenBuff := new(bytes.Buffer)
	_ = binary.Write(dataLenBuff, binary.LittleEndian, int64(requestCharLen))
	dataLen := fmt.Sprintf("%X", dataLenBuff.Bytes()[0:2]) // 2byte固定

	return SUB_HEADER +
		h.networkNum +
		h.pcNum +
		h.unitIONum +
		h.unitStationNum +
		dataLen +
		MONITORING_TIMER +
		RANDOM_WRITE_COMMAND +
		RANDOM_WORD_WRITE_SUB_COMMAND +
		countsHex +
		bodyHex
}

func (h *station) BuildAccessPath() {

}

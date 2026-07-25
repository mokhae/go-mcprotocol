package mcp

import (
	"encoding/binary"
	"fmt"
)

type parser struct {
}

func NewParser() *parser {
	return &parser{}
}

// Response represents mcp response
type Response struct {
	// Sub header
	SubHeader string
	// network number
	NetworkNum string
	// PC number
	PCNum string
	// Request Unit I/O number
	UnitIONum string
	// Request Unit station number
	UnitStationNum string
	// Response data length
	DataLen string
	// Response data code
	EndCode uint16
	// Response data
	Payload []byte
	// error data
	ErrInfo []byte
}

func (p *parser) Do(resp []byte) (*Response, error) {
	if len(resp) < 11 {
		return nil, &ProtocolError{Reason: fmt.Sprintf("frame must be at least 11 bytes, got %d", len(resp))}
	}
	if resp[0] != 0xD0 || resp[1] != 0x00 {
		return nil, &ProtocolError{Reason: fmt.Sprintf("unexpected response subheader %X", resp[0:2])}
	}

	dataLen := int(binary.LittleEndian.Uint16(resp[7:9]))
	if dataLen < 2 {
		return nil, &ProtocolError{Reason: fmt.Sprintf("data length must include the 2-byte end code, got %d", dataLen)}
	}
	expectedLen := 9 + dataLen
	if len(resp) != expectedLen {
		return nil, &ProtocolError{
			Reason: fmt.Sprintf("frame length is %d bytes but data length declares %d bytes", len(resp), expectedLen),
		}
	}

	subHeaderB := resp[0:2]
	networkNumB := resp[2:3]
	pcNumB := resp[3:4]
	unitIONumB := resp[4:6]
	unitStationNumB := resp[6:7]
	dataLenB := resp[7:9]
	endCodeB := resp[9:11]

	endCode := binary.LittleEndian.Uint16(endCodeB)
	var payload, errInfo []byte
	if endCode == 0 {
		payload = append([]byte(nil), resp[11:]...)
	} else {
		errInfo = append([]byte(nil), resp[11:]...)
	}

	return &Response{
		SubHeader:      fmt.Sprintf("%X", subHeaderB),
		NetworkNum:     fmt.Sprintf("%X", networkNumB),
		PCNum:          fmt.Sprintf("%X", pcNumB),
		UnitIONum:      fmt.Sprintf("%X", unitIONumB),
		UnitStationNum: fmt.Sprintf("%X", unitStationNumB),
		DataLen:        fmt.Sprintf("%X", dataLenB),
		EndCode:        endCode, //fmt.Sprintf("%X", endCodeB),
		Payload:        payload,
		ErrInfo:        errInfo,
	}, nil
}

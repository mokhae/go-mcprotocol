package mcp

import (
	"encoding/binary"
	"errors"
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
		return nil, errors.New(fmt.Sprintf("length must be larger than 11 byte: %v", resp))
	}

	subHeaderB := resp[0:2]
	networkNumB := resp[2:3]
	pcNumB := resp[3:4]
	unitIONumB := resp[4:6]
	unitStationNumB := resp[6:7]
	dataLenB := resp[7:9]
	endCodeB := resp[9:11]
	payloadB := resp[11:]

	endCode := binary.BigEndian.Uint16(endCodeB)

	return &Response{
		SubHeader:      fmt.Sprintf("%X", subHeaderB),
		NetworkNum:     fmt.Sprintf("%X", networkNumB),
		PCNum:          fmt.Sprintf("%X", pcNumB),
		UnitIONum:      fmt.Sprintf("%X", unitIONumB),
		UnitStationNum: fmt.Sprintf("%X", unitStationNumB),
		DataLen:        fmt.Sprintf("%X", dataLenB),
		EndCode:        endCode, //fmt.Sprintf("%X", endCodeB),
		Payload:        payloadB,
	}, nil
}

package mcp

import (
	"bytes"
	"errors"
	"testing"
)

func TestStation_BuildRRequest(t *testing.T) {
	station := NewLocalStation()
	request := station.BuildReadRequest("D", 300, 3)

	if request != "500000FFFF03000C001000010400002C0100A80300" {
		t.Fatalf("expected %v but actual is %v", "500000FFFF03000C001000010400002C0100A80300", request)
	}

	request2 := station.BuildReadRequest("D", 500, 50)
	if request2 != "500000FFFF03000C00100001040000F40100A83200" {
		t.Fatalf("expected %v but actual is %v", "500000FFFF03000C00100001040000F40100A83200", request2)
	}
}

func TestPackBits(t *testing.T) {
	// MELSECコミュニケーションプロトコル リファレンス: 1 point per 4 bits,
	// the 1st point in the upper 4 bits of the 1st byte.
	got := PackBits([]bool{true, false, true, false, false, false, true, true})
	if want := []byte{0x10, 0x10, 0x00, 0x11}; !bytes.Equal(got, want) {
		t.Fatalf("expected %X but actual is %X", want, got)
	}

	// odd number of points: the trailing lower 4 bits are padded with 0
	got = PackBits([]bool{true, true, true})
	if want := []byte{0x11, 0x10}; !bytes.Equal(got, want) {
		t.Fatalf("expected %X but actual is %X", want, got)
	}

	if got := PackBits(nil); len(got) != 0 {
		t.Fatalf("expected empty but actual is %X", got)
	}
}

func TestStation_BuildBitWriteRequest(t *testing.T) {
	station := NewLocalStation()

	// M100..M107 = 1,0,1,0,0,0,1,1
	request := station.BuildBitWriteRequest("M", 100, 8, PackBits([]bool{true, false, true, false, false, false, true, true}))
	//              sub    net    pc     io       stn    len     timer   cmd     sub     offset    dev    pts     data
	want := "5000" + "00" + "FF" + "FF03" + "00" + "1000" + "1000" + "0114" + "0100" + "640000" + "90" + "0800" + "10100011"
	if request != want {
		t.Fatalf("expected %v but actual is %v", want, request)
	}

	// single point: M100 ON
	request = station.BuildBitWriteRequest("M", 100, 1, PackBits([]bool{true}))
	want = "5000" + "00" + "FF" + "FF03" + "00" + "0D00" + "1000" + "0114" + "0100" + "640000" + "90" + "0100" + "10"
	if request != want {
		t.Fatalf("expected %v but actual is %v", want, request)
	}
}

func TestValidateBitWriteRequest(t *testing.T) {
	tests := []struct {
		name       string
		deviceName string
		offset     int64
		numPoints  int64
		writeData  []byte
		wantErr    bool
	}{
		{name: "ok even points", deviceName: "M", offset: 100, numPoints: 8, writeData: []byte{0x10, 0x10, 0x00, 0x11}},
		{name: "ok odd points", deviceName: "B", offset: 0, numPoints: 3, writeData: []byte{0x11, 0x10}},

		{name: "word device D is rejected", deviceName: "D", offset: 100, numPoints: 1, writeData: []byte{0x10}, wantErr: true},
		{name: "word device ZR is rejected", deviceName: "ZR", offset: 100, numPoints: 1, writeData: []byte{0x10}, wantErr: true},
		{name: "word device W is rejected", deviceName: "W", offset: 100, numPoints: 1, writeData: []byte{0x10}, wantErr: true},
		{name: "unknown device", deviceName: "QQ", offset: 100, numPoints: 1, writeData: []byte{0x10}, wantErr: true},
		{name: "zero points", deviceName: "M", offset: 100, numPoints: 0, writeData: []byte{}, wantErr: true},
		{name: "too many points", deviceName: "M", offset: 100, numPoints: maxBitPoints + 1, writeData: make([]byte, (maxBitPoints+2)/2), wantErr: true},
		{name: "offset out of 3byte range", deviceName: "M", offset: maxDeviceAddress + 1, numPoints: 1, writeData: []byte{0x10}, wantErr: true},
		{name: "writeData too short", deviceName: "M", offset: 100, numPoints: 8, writeData: []byte{0x10, 0x10}, wantErr: true},
		{name: "writeData too long", deviceName: "M", offset: 100, numPoints: 8, writeData: []byte{0x10, 0x10, 0x00, 0x11, 0x00}, wantErr: true},
		{name: "upper 4 bits not 0 or 1", deviceName: "M", offset: 100, numPoints: 2, writeData: []byte{0xF0}, wantErr: true},
		{name: "lower 4 bits not 0 or 1", deviceName: "M", offset: 100, numPoints: 2, writeData: []byte{0x0F}, wantErr: true},
		{name: "odd points with dirty padding", deviceName: "M", offset: 100, numPoints: 1, writeData: []byte{0x11}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBitWriteRequest(tt.deviceName, tt.offset, tt.numPoints, tt.writeData)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %v, want *ValidationError", err)
			}
		})
	}
}

func TestStation_BuildBitWriteRandomRequest(t *testing.T) {
	station := NewLocalStation()

	// turn M50 ON and Y2F OFF in one request
	request := station.BuildBitWriteRandomRequest([]BitPoint{
		{DeviceName: "M", Offset: 0x32, Value: true},
		{DeviceName: "Y", Offset: 0x2F, Value: false},
	})
	//              sub    net    pc     io       stn    len     timer   cmd     sub     points
	want := "5000" + "00" + "FF" + "FF03" + "00" + "1100" + "1000" + "0214" + "0100" + "02" +
		"320000" + "90" + "01" + // M50 ON
		"2F0000" + "9D" + "00" // Y2F OFF
	if request != want {
		t.Fatalf("expected %v but actual is %v", want, request)
	}

	// a single point still carries its own device
	request = station.BuildBitWriteRandomRequest([]BitPoint{{DeviceName: "M", Offset: 100, Value: true}})
	want = "5000" + "00" + "FF" + "FF03" + "00" + "0C00" + "1000" + "0214" + "0100" + "01" +
		"640000" + "90" + "01"
	if request != want {
		t.Fatalf("expected %v but actual is %v", want, request)
	}
}

func TestValidateBitWriteRandomRequest(t *testing.T) {
	tooMany := make([]BitPoint, maxRandomBitPoints+1)
	for i := range tooMany {
		tooMany[i] = BitPoint{DeviceName: "M", Offset: int64(i), Value: true}
	}
	atLimit := tooMany[:maxRandomBitPoints]

	tests := []struct {
		name    string
		points  []BitPoint
		wantErr bool
	}{
		{name: "ok mixed devices", points: []BitPoint{{DeviceName: "M", Offset: 50, Value: true}, {DeviceName: "Y", Offset: 0x2F}, {DeviceName: "B", Offset: 7, Value: true}}},
		{name: "ok at the single byte limit", points: atLimit},

		{name: "nil points", points: nil, wantErr: true},
		{name: "empty points", points: []BitPoint{}, wantErr: true},
		{name: "over the single byte limit", points: tooMany, wantErr: true},
		{name: "word device D is rejected", points: []BitPoint{{DeviceName: "D", Offset: 100, Value: true}}, wantErr: true},
		{name: "word device ZR is rejected", points: []BitPoint{{DeviceName: "ZR", Offset: 100, Value: true}}, wantErr: true},
		{name: "unknown device", points: []BitPoint{{DeviceName: "QQ", Offset: 100, Value: true}}, wantErr: true},
		{name: "offset out of 3byte range", points: []BitPoint{{DeviceName: "M", Offset: maxDeviceAddress + 1, Value: true}}, wantErr: true},
		{name: "negative offset", points: []BitPoint{{DeviceName: "M", Offset: -1, Value: true}}, wantErr: true},
		{name: "a bad point after a good one", points: []BitPoint{{DeviceName: "M", Offset: 1, Value: true}, {DeviceName: "D", Offset: 2}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBitWriteRandomRequest(tt.points)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %v, want *ValidationError", err)
			}
		})
	}
}

func TestStation_BuildWordWriteRandomRequest(t *testing.T) {
	station := NewLocalStation()

	// D100 = 0x1234 as a word point and D200 = 0x12345678 as a double word point
	request := station.BuildWordWriteRandomRequest(
		[]WordPoint{{DeviceName: "D", Offset: 100, Value: 0x1234}},
		[]DWordPoint{{DeviceName: "D", Offset: 200, Value: 0x12345678}},
	)
	//              sub    net    pc     io       stn    len     timer   cmd     sub     words dwords
	want := "5000" + "00" + "FF" + "FF03" + "00" + "1600" + "1000" + "0214" + "0000" + "01" + "01" +
		"640000" + "A8" + "3412" + // D100
		"C80000" + "A8" + "78563412" // D200-D201
	if request != want {
		t.Fatalf("expected %v but actual is %v", want, request)
	}

	// words only: the double word count is still sent, as 00
	request = station.BuildWordWriteRandomRequest(
		[]WordPoint{{DeviceName: "ZR", Offset: 338001, Value: 0}, {DeviceName: "ZR", Offset: 338003, Value: 1}}, nil,
	)
	want = "5000" + "00" + "FF" + "FF03" + "00" + "1400" + "1000" + "0214" + "0000" + "02" + "00" +
		"512805" + "B0" + "0000" + // ZR338001 = 0x052851
		"532805" + "B0" + "0100" // ZR338003
	if request != want {
		t.Fatalf("expected %v but actual is %v", want, request)
	}
}

func TestValidateWordWriteRandomRequest(t *testing.T) {
	words := func(n int) []WordPoint {
		points := make([]WordPoint, n)
		for i := range points {
			points[i] = WordPoint{DeviceName: "D", Offset: int64(i)}
		}
		return points
	}
	dwords := func(n int) []DWordPoint {
		points := make([]DWordPoint, n)
		for i := range points {
			points[i] = DWordPoint{DeviceName: "D", Offset: int64(1000 + 2*i)}
		}
		return points
	}
	tests := []struct {
		name    string
		words   []WordPoint
		dwords  []DWordPoint
		wantErr bool
	}{
		{name: "ok words and dwords", words: words(2), dwords: dwords(1)},
		{name: "ok dwords only", dwords: dwords(3)},
		{name: "ok 160 words is the unit limit", words: words(160)},
		{name: "ok 137 dwords fits 1920 units", dwords: dwords(137)},

		{name: "no points", wantErr: true},
		{name: "161 words exceeds 1920 units", words: words(161), wantErr: true},
		{name: "150 words and 10 dwords exceeds 1920 units", words: words(150), dwords: dwords(10), wantErr: true},
		{name: "bit device M is rejected", words: []WordPoint{{DeviceName: "M", Offset: 1}}, wantErr: true},
		{name: "bit device in dwords is rejected", dwords: []DWordPoint{{DeviceName: "X", Offset: 1}}, wantErr: true},
		{name: "unknown device", words: []WordPoint{{DeviceName: "QQ", Offset: 1}}, wantErr: true},
		{name: "offset out of 3byte range", words: []WordPoint{{DeviceName: "D", Offset: maxDeviceAddress + 1}}, wantErr: true},
		{name: "dword crossing the 3byte limit", dwords: []DWordPoint{{DeviceName: "D", Offset: maxDeviceAddress}}, wantErr: true},
		{name: "negative offset", words: []WordPoint{{DeviceName: "D", Offset: -1}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWordWriteRandomRequest(tt.words, tt.dwords)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %v, want *ValidationError", err)
			}
		})
	}
}

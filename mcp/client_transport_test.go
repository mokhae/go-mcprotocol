package mcp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestReadContextHandlesFragmentedResponse(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newPipeClient(clientConn)
	defer client.Disconnect()

	serverErr := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		request, err := readTestFrame(serverConn)
		if err != nil {
			serverErr <- err
			return
		}
		response := testResponse(request, 0, []byte{0x34, 0x12})
		for _, b := range response {
			if _, err := serverConn.Write([]byte{b}); err != nil {
				serverErr <- err
				return
			}
		}
		serverErr <- nil
	}()

	raw, err := client.ReadContext(context.Background(), "D", 100, 1)
	if err != nil {
		t.Fatalf("ReadContext returned an error: %v", err)
	}
	if got, want := len(raw), 13; got != want {
		t.Fatalf("response length = %d, want %d", got, want)
	}
	if raw[11] != 0x34 || raw[12] != 0x12 {
		t.Fatalf("response payload = %X, want 3412", raw[11:])
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("test server failed: %v", err)
	}
}

func TestHealthCheckUsesFramedResponse(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newPipeClient(clientConn)
	defer client.Disconnect()

	go func() {
		defer serverConn.Close()
		request, err := readTestFrame(serverConn)
		if err != nil {
			return
		}
		response := testResponse(request, 0, []byte{0x05, 0x00, 'A', 'B', 'C', 'D', 'E'})
		for offset := 0; offset < len(response); {
			end := offset + 2
			if end > len(response) {
				end = len(response)
			}
			if _, err := serverConn.Write(response[offset:end]); err != nil {
				return
			}
			offset = end
		}
	}()

	if err := client.HealthCheck(); err != nil {
		t.Fatalf("HealthCheck returned an error: %v", err)
	}
}

func TestReadContextCancellationClosesDesynchronizedConnection(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newPipeClient(clientConn)
	defer serverConn.Close()

	requestRead := make(chan struct{})
	go func() {
		_, _ = readTestFrame(serverConn)
		close(requestRead)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := client.ReadContext(ctx, "D", 100, 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ReadContext error = %v, want context deadline exceeded", err)
	}
	<-requestRead
	if client.IsConnect() {
		t.Fatal("client remains connected after a canceled partial transaction")
	}
}

func TestTruncatedResponseTimesOutAndClosesConnection(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newPipeClient(clientConn)
	client.readTimeout = 30 * time.Millisecond
	defer serverConn.Close()

	go func() {
		request, err := readTestFrame(serverConn)
		if err != nil {
			return
		}
		response := testResponse(request, 0, []byte{0x01, 0x02})
		_, _ = serverConn.Write(response[:12])
	}()

	_, err := client.Read("D", 100, 1)
	if err == nil {
		t.Fatal("Read returned nil error for a truncated response")
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("Read error = %v, want a network timeout", err)
	}
	if client.IsConnect() {
		t.Fatal("client remains connected after a truncated response")
	}
}

func TestReadReturnsTypedMCError(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newPipeClient(clientConn)
	defer client.Disconnect()

	go func() {
		defer serverConn.Close()
		request, err := readTestFrame(serverConn)
		if err != nil {
			return
		}
		_, _ = serverConn.Write(testResponse(request, 0x1234, []byte{0xAA, 0xBB}))
	}()

	_, err := client.Read("D", 100, 1)
	var mcErr *MCError
	if !errors.As(err, &mcErr) {
		t.Fatalf("Read error = %v, want *MCError", err)
	}
	if mcErr.EndCode != 0x1234 {
		t.Fatalf("EndCode = 0x%04X, want 0x1234", mcErr.EndCode)
	}
	if len(mcErr.ErrorInfo) != 2 || mcErr.ErrorInfo[0] != 0xAA || mcErr.ErrorInfo[1] != 0xBB {
		t.Fatalf("ErrorInfo = %X, want AABB", mcErr.ErrorInfo)
	}
}

func TestConcurrentRequestsAreSerialized(t *testing.T) {
	const requestCount = 32

	clientConn, serverConn := net.Pipe()
	client := newPipeClient(clientConn)
	defer client.Disconnect()

	serverErr := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		for i := 0; i < requestCount; i++ {
			request, err := readTestFrame(serverConn)
			if err != nil {
				serverErr <- err
				return
			}
			if _, err := serverConn.Write(testResponse(request, 0, []byte{byte(i), 0})); err != nil {
				serverErr <- err
				return
			}
		}
		serverErr <- nil
	}()

	var wg sync.WaitGroup
	errs := make(chan error, requestCount)
	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.Read("D", 100, 1)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("concurrent Read returned an error: %v", err)
		}
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("test server failed: %v", err)
	}
}

func TestRequestValidation(t *testing.T) {
	client := newPipeClient(nil)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "unknown device",
			call: func() error {
				_, err := client.Read("UNKNOWN", 0, 1)
				return err
			},
		},
		{
			name: "negative offset",
			call: func() error {
				_, err := client.Read("D", -1, 1)
				return err
			},
		},
		{
			name: "zero points",
			call: func() error {
				_, err := client.Read("D", 0, 0)
				return err
			},
		},
		{
			name: "word read limit",
			call: func() error {
				_, err := client.Read("D", 0, maxWordPoints+1)
				return err
			},
		},
		{
			name: "write length",
			call: func() error {
				_, err := client.Write("D", 0, 2, []byte{1, 2})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var validationErr *ValidationError
			if err := tt.call(); !errors.As(err, &validationErr) {
				t.Fatalf("error = %v, want *ValidationError", err)
			}
		})
	}
}

func TestNew3EClientUsesDefaultRouteWhenInterfaceIsEmpty(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	address := listener.Addr().(*net.TCPAddr)
	client, err := New3EClient(
		"127.0.0.1",
		address.Port,
		NewLocalStation(),
		"",
		"",
		time.Second,
		time.Second,
		time.Second,
		0,
	)
	if err != nil {
		t.Fatalf("New3EClient failed: %v", err)
	}
	if err := client.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Disconnect()

	serverConn := <-accepted
	defer serverConn.Close()
}

func newPipeClient(conn net.Conn) *client3E {
	return &client3E{
		readTimeout:  time.Second,
		writeTimeout: time.Second,
		stn:          NewLocalStation(),
		conn:         conn,
	}
}

func readTestFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, responseHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	dataLen := int(binary.LittleEndian.Uint16(header[7:9]))
	body := make([]byte, dataLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return append(header, body...), nil
}

func testResponse(request []byte, endCode uint16, data []byte) []byte {
	response := make([]byte, 11+len(data))
	response[0] = 0xD0
	response[1] = 0x00
	copy(response[2:7], request[2:7])
	binary.LittleEndian.PutUint16(response[7:9], uint16(2+len(data)))
	binary.LittleEndian.PutUint16(response[9:11], endCode)
	copy(response[11:], data)
	return response
}

func TestBitWriteContextSendsBitUnitFrame(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newPipeClient(clientConn)
	defer client.Disconnect()

	requests := make(chan []byte, 1)
	go func() {
		defer serverConn.Close()
		request, err := readTestFrame(serverConn)
		if err != nil {
			close(requests)
			return
		}
		requests <- request
		// a bit write response carries the end code only
		_, _ = serverConn.Write(testResponse(request, 0, nil))
	}()

	// M100..M107 = 1,0,1,0,0,0,1,1
	raw, err := client.BitWriteContext(
		context.Background(),
		"M", 100, 8,
		PackBits([]bool{true, false, true, false, false, false, true, true}),
	)
	if err != nil {
		t.Fatalf("BitWriteContext returned an error: %v", err)
	}
	if got, want := len(raw), 11; got != want {
		t.Fatalf("response length = %d, want %d", got, want)
	}

	request, ok := <-requests
	if !ok {
		t.Fatal("test server did not read a request frame")
	}
	// sub header, access route, data length, monitoring timer,
	// command 1401, sub command 0001, head device M100, device code M, 8 points, packed data
	want := []byte{
		0x50, 0x00, 0x00, 0xFF, 0xFF, 0x03, 0x00,
		0x10, 0x00,
		0x10, 0x00,
		0x01, 0x14,
		0x01, 0x00,
		0x64, 0x00, 0x00,
		0x90,
		0x08, 0x00,
		0x10, 0x10, 0x00, 0x11,
	}
	if !bytes.Equal(request, want) {
		t.Fatalf("request frame = %X, want %X", request, want)
	}
}

func TestBitWriteContextReportsEndCodeAsMCError(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newPipeClient(clientConn)
	defer client.Disconnect()

	go func() {
		defer serverConn.Close()
		request, err := readTestFrame(serverConn)
		if err != nil {
			return
		}
		// 0xC059 is a device specification error
		_, _ = serverConn.Write(testResponse(request, 0xC059, []byte{0x01, 0x14}))
	}()

	_, err := client.BitWriteContext(context.Background(), "M", 100, 1, PackBits([]bool{true}))
	var mcErr *MCError
	if !errors.As(err, &mcErr) {
		t.Fatalf("error = %v, want *MCError", err)
	}
	if mcErr.EndCode != 0xC059 {
		t.Fatalf("end code = %#04X, want %#04X", mcErr.EndCode, 0xC059)
	}
}

func TestBitWriteRejectsWordDeviceBeforeSending(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newPipeClient(clientConn)
	defer client.Disconnect()
	defer serverConn.Close()

	// a single bit of a word device cannot be addressed by a 3E frame,
	// so this must fail without touching the connection
	for _, device := range []string{"D", "ZR", "W"} {
		_, err := client.BitWrite(device, 100, 1, PackBits([]bool{true}))
		var validationErr *ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("device %v: error = %v, want *ValidationError", device, err)
		}
	}
}

func TestBitWriteRandomContextSendsRandomBitFrame(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newPipeClient(clientConn)
	defer client.Disconnect()

	requests := make(chan []byte, 1)
	go func() {
		defer serverConn.Close()
		request, err := readTestFrame(serverConn)
		if err != nil {
			close(requests)
			return
		}
		requests <- request
		_, _ = serverConn.Write(testResponse(request, 0, nil))
	}()

	raw, err := client.BitWriteRandomContext(context.Background(), []BitPoint{
		{DeviceName: "M", Offset: 0x32, Value: true},
		{DeviceName: "Y", Offset: 0x2F, Value: false},
	})
	if err != nil {
		t.Fatalf("BitWriteRandomContext returned an error: %v", err)
	}
	if got, want := len(raw), 11; got != want {
		t.Fatalf("response length = %d, want %d", got, want)
	}

	request, ok := <-requests
	if !ok {
		t.Fatal("test server did not read a request frame")
	}
	// sub header, access route, data length, monitoring timer, command 1402,
	// sub command 0001, 2 points, then device number + device code + value per point
	want := []byte{
		0x50, 0x00, 0x00, 0xFF, 0xFF, 0x03, 0x00,
		0x11, 0x00,
		0x10, 0x00,
		0x02, 0x14,
		0x01, 0x00,
		0x02,
		0x32, 0x00, 0x00, 0x90, 0x01, // M50 ON
		0x2F, 0x00, 0x00, 0x9D, 0x00, // Y2F OFF
	}
	if !bytes.Equal(request, want) {
		t.Fatalf("request frame = %X, want %X", request, want)
	}
}

func TestBitWriteRandomContextReportsEndCodeAsMCError(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newPipeClient(clientConn)
	defer client.Disconnect()

	go func() {
		defer serverConn.Close()
		request, err := readTestFrame(serverConn)
		if err != nil {
			return
		}
		// 0xC059 is a device specification error
		_, _ = serverConn.Write(testResponse(request, 0xC059, []byte{0x02, 0x14}))
	}()

	_, err := client.BitWriteRandomContext(context.Background(), []BitPoint{{DeviceName: "M", Offset: 100, Value: true}})
	var mcErr *MCError
	if !errors.As(err, &mcErr) {
		t.Fatalf("error = %v, want *MCError", err)
	}
	if mcErr.EndCode != 0xC059 {
		t.Fatalf("end code = %#04X, want %#04X", mcErr.EndCode, 0xC059)
	}
}

func TestBitWriteRandomRejectsWordDeviceBeforeSending(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := newPipeClient(clientConn)
	defer client.Disconnect()
	defer serverConn.Close()

	// a word device point must fail without touching the connection,
	// even when it follows a valid bit device point
	for _, device := range []string{"D", "ZR", "W"} {
		_, err := client.BitWriteRandom([]BitPoint{
			{DeviceName: "M", Offset: 100, Value: true},
			{DeviceName: device, Offset: 100, Value: true},
		})
		var validationErr *ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("device %v: error = %v, want *ValidationError", device, err)
		}
	}
}

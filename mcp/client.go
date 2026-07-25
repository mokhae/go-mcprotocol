package mcp

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

const responseHeaderSize = 9

type Client interface {
	Read(deviceName string, offset, numPoints int64) ([]byte, error)
	ReadContext(ctx context.Context, deviceName string, offset, numPoints int64) ([]byte, error)
	BitRead(deviceName string, offset, numPoints int64) ([]byte, error)
	BitReadContext(ctx context.Context, deviceName string, offset, numPoints int64) ([]byte, error)
	Write(deviceName string, offset, numPoints int64, writeData []byte) ([]byte, error)
	WriteContext(ctx context.Context, deviceName string, offset, numPoints int64, writeData []byte) ([]byte, error)
	BitWrite(deviceName string, offset, numPoints int64, writeData []byte) ([]byte, error)
	BitWriteContext(ctx context.Context, deviceName string, offset, numPoints int64, writeData []byte) ([]byte, error)
	HealthCheck() error
	HealthCheckContext(ctx context.Context) error
	Connect() error
	ConnectContext(ctx context.Context) error
	Disconnect() error
	Reconnect() error
	ReconnectContext(ctx context.Context) error
	IsConnect() bool
}

// client3E is a MELSEC 3E binary frame client. A 3E frame has no
// transaction identifier, so operations on a connection must be serialized.
type client3E struct {
	tcpAddrStr string
	dialer     net.Dialer

	readTimeout  time.Duration
	writeTimeout time.Duration
	stn          *station

	opMu    sync.Mutex
	stateMu sync.RWMutex
	conn    net.Conn
}

func New3EClient(
	host string,
	port int,
	stn *station,
	ethDevice string,
	localIP string,
	conTimeout time.Duration,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	localPort int,
) (Client, error) {
	if host == "" {
		return nil, &ValidationError{Field: "host", Reason: "must not be empty"}
	}
	if port < 1 || port > 65535 {
		return nil, &ValidationError{Field: "port", Value: port, Reason: "must be between 1 and 65535"}
	}
	if stn == nil {
		return nil, &ValidationError{Field: "station", Reason: "must not be nil"}
	}
	if err := stn.validate(); err != nil {
		return nil, err
	}
	if conTimeout < 0 || readTimeout < 0 || writeTimeout < 0 {
		return nil, &ValidationError{Field: "timeout", Reason: "must not be negative"}
	}
	if localPort < 0 || localPort > 65535 {
		return nil, &ValidationError{Field: "localPort", Value: localPort, Reason: "must be between 0 and 65535"}
	}

	dialer := net.Dialer{Timeout: conTimeout}
	localAddr, err := resolveLocalAddr(ethDevice, localIP, localPort)
	if err != nil {
		return nil, err
	}
	if localAddr != nil {
		dialer.LocalAddr = localAddr
	}

	return &client3E{
		tcpAddrStr:   net.JoinHostPort(host, strconv.Itoa(port)),
		dialer:       dialer,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
		stn:          stn,
	}, nil
}

func resolveLocalAddr(ethDevice, localIP string, localPort int) (*net.TCPAddr, error) {
	var parsedIP net.IP
	if localIP != "" {
		parsedIP = net.ParseIP(localIP)
		if parsedIP == nil {
			return nil, &ValidationError{Field: "localIP", Value: localIP, Reason: "must be a valid IP address"}
		}
	}

	if ethDevice != "" {
		iface, err := net.InterfaceByName(ethDevice)
		if err != nil {
			return nil, fmt.Errorf("network interface %q: %w", ethDevice, err)
		}
		if parsedIP != nil {
			addrs, err := iface.Addrs()
			if err != nil {
				return nil, fmt.Errorf("addresses for network interface %q: %w", ethDevice, err)
			}
			found := false
			for _, addr := range addrs {
				ip, _, err := net.ParseCIDR(addr.String())
				if err == nil && ip.Equal(parsedIP) {
					found = true
					break
				}
			}
			if !found {
				return nil, &ValidationError{
					Field:  "localIP",
					Value:  localIP,
					Reason: fmt.Sprintf("is not assigned to network interface %q", ethDevice),
				}
			}
		}
	}

	if parsedIP == nil && localPort == 0 {
		return nil, nil
	}
	return &net.TCPAddr{IP: parsedIP, Port: localPort}, nil
}

func (c *client3E) Connect() error {
	return c.ConnectContext(context.Background())
}

func (c *client3E) ConnectContext(ctx context.Context) error {
	if ctx == nil {
		return &ValidationError{Field: "context", Reason: "must not be nil"}
	}

	c.opMu.Lock()
	defer c.opMu.Unlock()

	if c.currentConn() != nil {
		return ErrAlreadyConnected
	}
	conn, err := c.dialer.DialContext(ctx, "tcp", c.tcpAddrStr)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	c.setConn(conn)
	return nil
}

func (c *client3E) Disconnect() error {
	c.opMu.Lock()
	defer c.opMu.Unlock()

	conn := c.swapConn(nil)
	if conn == nil {
		return nil
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("disconnect: %w", err)
	}
	return nil
}

func (c *client3E) IsConnect() bool {
	return c.currentConn() != nil
}

func (c *client3E) Reconnect() error {
	return c.ReconnectContext(context.Background())
}

func (c *client3E) ReconnectContext(ctx context.Context) error {
	if ctx == nil {
		return &ValidationError{Field: "context", Reason: "must not be nil"}
	}

	c.opMu.Lock()
	defer c.opMu.Unlock()

	if old := c.swapConn(nil); old != nil {
		_ = old.Close()
	}
	conn, err := c.dialer.DialContext(ctx, "tcp", c.tcpAddrStr)
	if err != nil {
		return fmt.Errorf("reconnect: %w", err)
	}
	c.setConn(conn)
	return nil
}

func (c *client3E) HealthCheck() error {
	return c.HealthCheckContext(context.Background())
}

func (c *client3E) HealthCheckContext(ctx context.Context) error {
	payload, err := hex.DecodeString(c.stn.BuildHealthCheckRequest())
	if err != nil {
		return fmt.Errorf("build health check request: %w", err)
	}
	_, response, err := c.exchange(ctx, payload)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	expected := []byte{0x05, 0x00, 'A', 'B', 'C', 'D', 'E'}
	if !bytes.Equal(response.Payload, expected) {
		return &ProtocolError{Reason: fmt.Sprintf("unexpected health check payload: %X", response.Payload)}
	}
	return nil
}

func (c *client3E) Read(deviceName string, offset, numPoints int64) ([]byte, error) {
	return c.ReadContext(context.Background(), deviceName, offset, numPoints)
}

func (c *client3E) ReadContext(ctx context.Context, deviceName string, offset, numPoints int64) ([]byte, error) {
	if err := validateDeviceRequest(deviceName, offset, numPoints, maxWordPoints); err != nil {
		return nil, err
	}
	payload, err := hex.DecodeString(c.stn.BuildReadRequest(deviceName, offset, numPoints))
	if err != nil {
		return nil, fmt.Errorf("build read request: %w", err)
	}
	raw, _, err := c.exchange(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return raw, nil
}

func (c *client3E) BitRead(deviceName string, offset, numPoints int64) ([]byte, error) {
	return c.BitReadContext(context.Background(), deviceName, offset, numPoints)
}

func (c *client3E) BitReadContext(ctx context.Context, deviceName string, offset, numPoints int64) ([]byte, error) {
	if err := validateDeviceRequest(deviceName, offset, numPoints, maxBitPoints); err != nil {
		return nil, err
	}
	payload, err := hex.DecodeString(c.stn.BuildBitReadRequest(deviceName, offset, numPoints))
	if err != nil {
		return nil, fmt.Errorf("build bit read request: %w", err)
	}
	raw, _, err := c.exchange(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("bit read: %w", err)
	}
	return raw, nil
}

func (c *client3E) Write(deviceName string, offset, numPoints int64, writeData []byte) ([]byte, error) {
	return c.WriteContext(context.Background(), deviceName, offset, numPoints, writeData)
}

func (c *client3E) WriteContext(ctx context.Context, deviceName string, offset, numPoints int64, writeData []byte) ([]byte, error) {
	if err := validateDeviceRequest(deviceName, offset, numPoints, maxWordPoints); err != nil {
		return nil, err
	}
	expectedLen := int(numPoints) * 2
	if len(writeData) != expectedLen {
		return nil, &ValidationError{
			Field:  "writeData",
			Value:  len(writeData),
			Reason: fmt.Sprintf("length must be exactly %d bytes for %d word points", expectedLen, numPoints),
		}
	}
	payload, err := hex.DecodeString(c.stn.BuildWriteRequest(deviceName, offset, numPoints, writeData))
	if err != nil {
		return nil, fmt.Errorf("build write request: %w", err)
	}
	raw, _, err := c.exchange(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	return raw, nil
}

// BitWrite writes device points in bit units (command 1401, subcommand 0001).
//
// deviceName must be a bit device such as 'M' or 'B'. Word devices like D / W / ZR
// are rejected with a *ValidationError: a 3E frame addresses a device by head
// device number plus device code and carries no bit position, so a single bit of a
// word device cannot be addressed.
//
// Only the requested points are updated, the remaining bits of the same word are
// left untouched by the CPU. No client side read-modify-write is involved, so
// there is no race against the ladder scan.
//
// writeData holds 1 point per 4 bits, so it must be exactly (numPoints+1)/2 bytes.
// Use PackBits to build it from []bool. A write rejected by the PLC is reported as
// a *MCError carrying the end code.
func (c *client3E) BitWrite(deviceName string, offset, numPoints int64, writeData []byte) ([]byte, error) {
	return c.BitWriteContext(context.Background(), deviceName, offset, numPoints, writeData)
}

func (c *client3E) BitWriteContext(ctx context.Context, deviceName string, offset, numPoints int64, writeData []byte) ([]byte, error) {
	if err := validateBitWriteRequest(deviceName, offset, numPoints, writeData); err != nil {
		return nil, err
	}
	payload, err := hex.DecodeString(c.stn.BuildBitWriteRequest(deviceName, offset, numPoints, writeData))
	if err != nil {
		return nil, fmt.Errorf("build bit write request: %w", err)
	}
	raw, _, err := c.exchange(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("bit write: %w", err)
	}
	return raw, nil
}

func (c *client3E) exchange(ctx context.Context, request []byte) ([]byte, *Response, error) {
	if ctx == nil {
		return nil, nil, &ValidationError{Field: "context", Reason: "must not be nil"}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	c.opMu.Lock()
	defer c.opMu.Unlock()

	conn := c.currentConn()
	if conn == nil {
		return nil, nil, ErrNotConnected
	}

	cancelDone := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
		close(cancelDone)
	})
	defer func() {
		if !stopCancel() {
			<-cancelDone
		}
		_ = conn.SetDeadline(time.Time{})
	}()

	if err := conn.SetWriteDeadline(operationDeadline(ctx, c.writeTimeout)); err != nil {
		c.invalidateConn(conn)
		return nil, nil, fmt.Errorf("set write deadline: %w", err)
	}
	if err := writeFull(conn, request); err != nil {
		c.invalidateConn(conn)
		if ctxErr := contextOperationError(ctx); ctxErr != nil {
			return nil, nil, ctxErr
		}
		return nil, nil, fmt.Errorf("write request: %w", err)
	}

	if err := conn.SetReadDeadline(operationDeadline(ctx, c.readTimeout)); err != nil {
		c.invalidateConn(conn)
		return nil, nil, fmt.Errorf("set read deadline: %w", err)
	}
	header := make([]byte, responseHeaderSize)
	if _, err := io.ReadFull(conn, header); err != nil {
		c.invalidateConn(conn)
		if ctxErr := contextOperationError(ctx); ctxErr != nil {
			return nil, nil, ctxErr
		}
		return nil, nil, fmt.Errorf("read response header: %w", err)
	}

	dataLen := int(header[7]) | int(header[8])<<8
	if dataLen < 2 {
		c.invalidateConn(conn)
		return nil, nil, &ProtocolError{Reason: fmt.Sprintf("response data length must be at least 2, got %d", dataLen)}
	}
	body := make([]byte, dataLen)
	if _, err := io.ReadFull(conn, body); err != nil {
		c.invalidateConn(conn)
		if ctxErr := contextOperationError(ctx); ctxErr != nil {
			return nil, nil, ctxErr
		}
		return nil, nil, fmt.Errorf("read response body: %w", err)
	}

	raw := append(header, body...)
	response, err := NewParser().Do(raw)
	if err != nil {
		c.invalidateConn(conn)
		return nil, nil, err
	}
	if len(request) >= 7 && !bytes.Equal(raw[2:7], request[2:7]) {
		c.invalidateConn(conn)
		return nil, nil, &ProtocolError{Reason: "response access route does not match request"}
	}
	if response.EndCode != 0 {
		return nil, response, &MCError{EndCode: response.EndCode, ErrorInfo: response.ErrInfo}
	}
	return raw, response, nil
}

func operationDeadline(ctx context.Context, timeout time.Duration) time.Time {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	if contextDeadline, ok := ctx.Deadline(); ok && (deadline.IsZero() || contextDeadline.Before(deadline)) {
		deadline = contextDeadline
	}
	return deadline
}

func contextOperationError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return nil
}

func writeFull(w io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := w.Write(payload)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		payload = payload[n:]
	}
	return nil
}

func (c *client3E) currentConn() net.Conn {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.conn
}

func (c *client3E) setConn(conn net.Conn) {
	c.stateMu.Lock()
	c.conn = conn
	c.stateMu.Unlock()
}

func (c *client3E) swapConn(conn net.Conn) net.Conn {
	c.stateMu.Lock()
	old := c.conn
	c.conn = conn
	c.stateMu.Unlock()
	return old
}

func (c *client3E) invalidateConn(conn net.Conn) {
	c.stateMu.Lock()
	if c.conn == conn {
		c.conn = nil
	}
	c.stateMu.Unlock()
	_ = conn.Close()
}

var _ Client = (*client3E)(nil)

package mcp

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"time"
)

type Client interface {
	Read(deviceName string, offset, numPoints int64) ([]byte, error)
	BitRead(deviceName string, offset, numPoints int64) ([]byte, error)
	Write(deviceName string, offset, numPoints int64, writeData []byte) ([]byte, error)
	HealthCheck() error
	NewConnect() error
}

// client3E is 3E frame mcp client
type client3E struct {
	// PLC address
	tcpAddr *net.TCPAddr
	// PLC address string
	tcpAddrStr string
	// Timeout
	conTimeout   time.Duration
	readTimeout  time.Duration
	writeTimeout time.Duration
	// PLC station
	stn *station

	conn net.Conn
}

func New3EClient(host string, port int, stn *station, conTimeout time.Duration, readTimeout time.Duration, writeTimeout time.Duration) (Client, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%v:%v", host, port))
	if err != nil {
		return nil, err
	}
	tcpaddrstr := fmt.Sprintf("%v:%v", host, port)
	return &client3E{tcpAddr: tcpAddr, tcpAddrStr: tcpaddrstr, conTimeout: conTimeout, readTimeout: readTimeout, writeTimeout: writeTimeout, stn: stn}, nil
}

func (c *client3E) NewConnect() error {
	// TODO Keep-Alive
	//d := net.Dialer{Timeout: time.Duration(1000)}
	conn, err := net.DialTimeout("tcp", c.tcpAddrStr, c.conTimeout)
	//conn, err := net.DialTCP("tcp", nil, c.tcpAddr)
	if err != nil {
		return errors.New(fmt.Sprintf("Connect Error : %v", err))
	}
	//defer conn.Close()

	// Send message
	err = conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	if err != nil {
		return errors.New(fmt.Sprintf("SetWriteDeadline Error : %v", err))
	}

	c.conn = conn

	return nil
}

// MELSECコミュニケーションプロトコル p180
// 11.4折返しテスト
func (c *client3E) HealthCheck() error {
	requestStr := c.stn.BuildHealthCheckRequest()

	// binary protocol
	payload, err := hex.DecodeString(requestStr)
	if err != nil {
		return err
	}

	// TODO Keep-Alive
	////d := net.Dialer{Timeout: time.Duration(1000)}
	//conn, err := net.DialTimeout("tcp", c.tcpAddrStr, c.conTimeout)
	////conn, err := net.DialTCP("tcp", nil, c.tcpAddr)
	//if err != nil {
	//	return errors.New(fmt.Sprintf("Connect Error : %v", err))
	//}
	//defer conn.Close()
	//
	//// Send message
	//err = conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	//if err != nil {
	//	return errors.New(fmt.Sprintf("SetWriteDeadline Error : %v", err))
	//}

	if _, err = c.conn.Write(payload); err != nil {
		return errors.New(fmt.Sprintf("Write Error : %v", err))
	}

	// Receive message
	readBuff := make([]byte, 30)
	//err = c.conn.SetReadDeadline(time.Now().Add(c.readTimeout))
	//if err != nil {
	//	return err
	//}
	readLen, err := c.conn.Read(readBuff)
	if err != nil {
		return errors.New(fmt.Sprintf("Read Error : %v", err))
	}

	resp := readBuff[:readLen]

	if readLen != 18 {
		return errors.New("plc connect test is fail: return length is [" + fmt.Sprintf("%X", resp) + "]")
	}

	// decodeString is 折返しデータ数ヘッダ[1byte]
	if "0500" != fmt.Sprintf("%X", resp[11:13]) {
		return errors.New("plc connect test is fail: return header is [" + fmt.Sprintf("%X", resp[11:13]) + "]")
	}

	//  折返しデータ[5byte]=ABCDE
	if "4142434445" != fmt.Sprintf("%X", resp[13:18]) {
		return errors.New("plc connect test is fail: return body is [" + fmt.Sprintf("%X", resp[13:18]) + "]")
	}

	return nil
}

// Read is send read as word command to remote plc by mc protocol
// deviceName is device code name like 'D' register.
// offset is device offset addr.
// numPoints is number of read device points.
func (c *client3E) Read(deviceName string, offset, numPoints int64) ([]byte, error) {
	requestStr := c.stn.BuildReadRequest(deviceName, offset, numPoints)

	// TODO binary protocol
	payload, err := hex.DecodeString(requestStr)
	if err != nil {
		return nil, err
	}

	//conn, err := net.DialTimeout("tcp", c.tcpAddrStr, c.conTimeout)
	////conn, err := net.DialTCP("tcp", nil, c.tcpAddr)
	//if err != nil {
	//	return nil, errors.New(fmt.Sprintf("Connect Error : %v", err))
	//}
	//defer conn.Close()
	//
	//// Send message
	//err = conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	//if err != nil {
	//	return nil, err
	//}
	if _, err = c.conn.Write(payload); err != nil {
		return nil, errors.New(fmt.Sprintf("Write Error : %v", err))
	}

	// Receive message
	readBuff := make([]byte, 22+2*numPoints) // 22 is response header size. [sub header + network num + unit i/o num + unit station num + response length + response code]
	//err = c.conn.SetReadDeadline(time.Now().Add(c.readTimeout))
	//if err != nil {
	//	return nil, err
	//}
	readLen, err := c.conn.Read(readBuff)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Read Error : %v", err))
	}

	return readBuff[:readLen], nil
}

// BitRead is send read as bit command to remote plc by mc protocol
// deviceName is device code name like 'D' register.
// offset is device offset addr.
// numPoints is number of read device points.
// results of payload of BitRead will return []byte contains 0, 1, 16 or 17(hex encoded 00, 01, 10, 11)
func (c *client3E) BitRead(deviceName string, offset, numPoints int64) ([]byte, error) {
	requestStr := c.stn.BuildBitReadRequest(deviceName, offset, numPoints)
	// TODO binary protocol
	payload, err := hex.DecodeString(requestStr)
	if err != nil {
		return nil, err
	}

	//conn, err := net.DialTimeout("tcp", c.tcpAddrStr, c.conTimeout)
	////conn, err := net.DialTCP("tcp", nil, c.tcpAddr)
	//if err != nil {
	//	return nil, errors.New(fmt.Sprintf("Connect Error : %v", err))
	//}
	//defer conn.Close()
	//
	//// Send message
	//err = conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	//if err != nil {
	//	return nil, err
	//}
	if _, err = c.conn.Write(payload); err != nil {
		return nil, errors.New(fmt.Sprintf("Write Error : %v", err))
	}

	// Receive message
	readBuff := make([]byte, 22+2*numPoints) // 22 is response header size. [sub header + network num + unit i/o num + unit station num + response length + response code]
	//err = c.conn.SetReadDeadline(time.Now().Add(c.readTimeout))
	//if err != nil {
	//	return nil, err
	//}
	readLen, err := c.conn.Read(readBuff)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Read Error : %v", err))
	}

	return readBuff[:readLen], nil
}

// Write is send write command to remote plc by mc protocol
// deviceName is device code name like 'D' register.
// offset is device offset addr.
// writeData is data to write.
// numPoints is number of write device points.
// writeData is the data to be written. If writeData is larger than 2*numPoints bytes,
// data larger than 2*numPoints bytes is ignored.
func (c *client3E) Write(deviceName string, offset, numPoints int64, writeData []byte) ([]byte, error) {
	requestStr := c.stn.BuildWriteRequest(deviceName, offset, numPoints, writeData)
	payload, err := hex.DecodeString(requestStr)
	if err != nil {
		return nil, err
	}

	//conn, err := net.DialTimeout("tcp", c.tcpAddrStr, c.conTimeout)
	////conn, err := net.DialTCP("tcp", nil, c.tcpAddr)
	//if err != nil {
	//	return nil, errors.New(fmt.Sprintf("Connect Error : %v", err))
	//}
	//defer conn.Close()
	//
	//// Send message
	//err = conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	//if err != nil {
	//	return nil, err
	//}
	if _, err = c.conn.Write(payload); err != nil {
		return nil, errors.New(fmt.Sprintf("Write Error : %v", err))
	}

	//// Receive message
	readBuff := make([]byte, 22) // 22 is response header size. [sub header + network num + unit i/o num + unit station num + response length + response code]
	//err = c.conn.SetReadDeadline(time.Now().Add(c.readTimeout))
	//if err != nil {
	//	return nil, err
	//}

	readLen, err := c.conn.Read(readBuff)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Read Error : %v", err))
	}
	return readBuff[:readLen], nil
}

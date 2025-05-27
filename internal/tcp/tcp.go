package tcp

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/realtimelidar/tcpwsbridge/internal/cli"
	"github.com/realtimelidar/tcpwsbridge/internal/logger"
)

var (
	config cli.TcpConfigParams
	formattedAddr string

	recvBuffPool sync.Pool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 1024) // or a typical upper bound
		},
	}

	msgBuffPool sync.Pool = sync.Pool{
		New: func() any {
			return make([]byte, 10*1024*1024)
		},
	}
)

func Init(conf cli.TcpConfigParams,) {
	config = conf
	formattedAddr = fmt.Sprintf("%s:%d", config.Host, config.Port)
}

func NewConnection(ctx context.Context) (chan []byte, chan []byte, error) {
	addr, err := net.ResolveTCPAddr("tcp", formattedAddr)

	if err != nil {
		return nil, nil, fmt.Errorf("invalid TCP address: %s", formattedAddr)
	}

	conn, err := net.DialTCP("tcp", nil, addr)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to TCP server: %v", err)
	}

	logger.Info("New connection to TCP server")

	// Channel to send data to TCP (From web)
	s := make(chan []byte)

	// Channel to receive data from TCP (To web)
	r := make(chan []byte)

	// Goroutine to get data from websocket and send to TCP
	go func(ctx context.Context, conn *net.TCPConn) {
		loop: for {
			select {
			case <-ctx.Done():
				conn.Close()
				logger.Info("Closing this TCP connection")
				break loop
			case dataToSend := <-s:
				// Data to be sent to TCP server
				dataLen := len(dataToSend)
				sentBytes := 0

				if (dataLen < 1024) {
					_, err := conn.Write(dataToSend)
					if err != nil {
						logger.Errorf("Failed to send data to TCP server: %v", err)
						continue
					}

					continue
				}

				for sentBytes != dataLen {
					max := dataLen + 1024
					if max > len(dataToSend) {
						max = len(dataToSend)
					}

					n, err := conn.Write(dataToSend[sentBytes - 1:max])

					if err != nil {
						logger.Errorf("Failed to send data to TCP server: %v", err)
						continue
					}
					sentBytes += n
				}
			}
		}
	}(ctx, conn)

	// Get data from TCP and end to Websocket
	go func(ctx context.Context, conn *net.TCPConn) {
		buf := [8]byte{}
		magicNumBuf := [18]byte{}

		magicNum := false

		loop: for {
			select {
			case <-ctx.Done():
				conn.Close()
				logger.Info("Closing this TCP connection")
				break loop
			default:
				conn.SetReadDeadline(time.Now().Add(5 * time.Millisecond))

				if !magicNum {
					n, err := conn.Read(magicNumBuf[0:])

					if err != nil {
						neterr, ok := err.(net.Error)
						if !ok || !neterr.Timeout() {
							logger.Errorf("Failed to read from TCP server. %v", err)
						}
					}

					if n <= 0 {
						continue
					}

					// Magic number
					if n == 18 && bytes.Equal(magicNumBuf[:], []byte{76, 105, 100, 97, 114, 83, 101, 114, 118, 32, 80, 114, 111, 116, 111, 99, 111, 108}) {
						r <- magicNumBuf[:n]
						magicNum = true
						continue
					}
				} else {
					// Read message size
					n, err := conn.Read(buf[0:])

					if err != nil {
						neterr, ok := err.(net.Error)
						if !ok || !neterr.Timeout() {
							logger.Errorf("Failed to read from TCP server. %v", err)
							continue
						}
					}

					if n <= 0 {
						continue
					}

					encodedDataLen := binary.LittleEndian.Uint64(buf[0:])
					dataLen := encodedDataLen & 0xffffff

					reader := io.LimitReader(conn, int64(dataLen - 8))
					
					recvBuf := recvBuffPool.Get().([]byte)
					defer recvBuffPool.Put(recvBuf)
					// recvBuf := make([]byte, 1024)

					var msgBuf []byte

					if dataLen < 10*1024*1024 {
						msgBuf = msgBuffPool.Get().([]byte)[:dataLen]
						defer msgBuffPool.Put(msgBuf)
					} else {
						msgBuf = make([]byte, dataLen)
					}
					// msgBuf := make([]byte, dataLen)
					
					copy(msgBuf, buf[:])
					
					recvBytes := uint64(8)

					conn.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
					n, err = reader.Read(recvBuf)

					for err != io.EOF {
						copy(msgBuf[recvBytes:], recvBuf[:n])
						recvBytes += uint64(n)

						if recvBytes < (dataLen - 8) {
							conn.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
							n, err = reader.Read(recvBuf)
						} else {
							break
						}
					}

					r <- msgBuf
				}
			}	
		}
	}(ctx, conn)

	return r, s, nil
}
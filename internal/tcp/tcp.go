package tcp

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/realtimelidar/tcpwsbridge/internal/cli"
	"github.com/realtimelidar/tcpwsbridge/internal/logger"
)

var (
	config cli.TcpConfigParams
	formattedAddr string
)

const (
	BUFF_SIZE = 10*1024
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

	recvBuf := make([]byte, 1024)
	msgBuff := make([]byte, BUFF_SIZE)

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

		// Reading procedure is based in the Lidarserv Protocol
		// For my future self or those interested:
		// First message from the server is the "magic number", that is,
		// ASCII-encoded "LidarServ Protocol"
		//
		// Then, messages are encoded as:
		// [0..7] Messages size, first 4 bytes "Header Length", last 4 bytes whole message length (including payload)
		// [8..] Payload
		//
		// The message might be very big, depending on payload size which is not fixed
		// So it might, and will, come from different TCP "reads"
		//
		// We cannot send an unlimited message via websocket since we cannot reserve unlimited RAM for the message buffer to read it
		// So,
		// 1. We read magic number and set an internal flag
		// 2. We read 8 bytes from TCP and determine whole message length
		// 3. We create a LimitedReader to read *exactly* "len" bytes
		// 4. We might need multiple TCP reads, so every read is a websocket message sent
		// 5. The websocket client will receive all messages and keep concatenating all together
		// 6. Finally, the last websocket message will be sent. The client knows how many bytes it has to receive
		//    per message, so it will know when the last message is received.
		// 7. The whole message is built clientside and processed

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

					// Take lower 4 bytes, they are the whole message length
					dataLen := encodedDataLen & 0xffffffff

					// - 8 since we already read the length
					reader := io.LimitReader(conn, int64(dataLen - 8))

					buffLen := min(dataLen, BUFF_SIZE)
					msgBuf := msgBuff[:buffLen]
					
					// Copy "length", fist 8 bytes
					copy(msgBuf, buf[:])
					
					// We have already read 8 bytes...
					recvBytes := uint64(8)

					conn.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
					n, err = reader.Read(recvBuf)

					if dataLen > BUFF_SIZE {
						logger.Infof("Fragmentation needed! (%d)", dataLen)
					}

					// Keep reading until the end of this message
					// EOF = last byte received
					readInThisBuff := 8
					for err != io.EOF {
						recvBytes += uint64(n)
						freeSpace := BUFF_SIZE - readInThisBuff

						logger.Infof("Read %d from TCP, %d remaining, current buffer usage %d, free space %d", n, dataLen - recvBytes, readInThisBuff, freeSpace)
						// logger.Infof("%v", recvBuf[:n])

						// If we have space in the buffer for this read
						// then we copy contents to the buffer
						if freeSpace >= n {
							logger.Infof("Contents fit buffer")
							copy(msgBuf[readInThisBuff:], recvBuf[:n])

							readInThisBuff += n

							conn.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
							n, err = reader.Read(recvBuf)
						} else {
							logger.Infof("Contents DO NOT fit buffer")

							// Otherwise we need to send contents to websocket and reset buffer

							// Fill remaining space in msgBuf
							// (:freeSpace because n > freeSpace)
							copy(msgBuf[readInThisBuff:], recvBuf[:freeSpace])

							// Send this buffer
							logger.Infof("Sending to websocket fragmentated message")
							r <- msgBuf

							// Reset buffer, but keep bytes left after filling it (n - freeSpace)
							// freeSpace:n = We already processed "freeSpace" bytes (because we filled the free space already)
							// 				 So we want to take the remaing (n - freeSpace, that is, from "freeSpace" up to "n")
							readInThisBuff = n - freeSpace
							copy(msgBuf, recvBuf[freeSpace:n])

							logger.Infof("Remaining %d for next buffer iteration", readInThisBuff)

							
							// r <- msgBuf

							conn.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
							n, err = reader.Read(recvBuf)
						}
					}

					logger.Infof("Sending last (or only) websocket message")
					r <- msgBuf[:readInThisBuff]

					// for recvBytes < dataLen {
					// 	i := uint64(0)
					// 	for err != io.EOF && recvBytes < (i*100*1024) {
					// 		logger.Infof("%v", recvBuf[:n])
					// 		logger.Infof("%d", recvBytes % (100*1024))
					// 		copy(msgBuf[recvBytes % (100*1024):], recvBuf[:n])
					// 		recvBytes += uint64(n)

					// 		if recvBytes < (dataLen - 8) {
					// 			conn.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
					// 			n, err = reader.Read(recvBuf)
					// 		} else {
					// 			break
					// 		}
					// 	}

					// 	logger.Infof("Sending first chunk at %d", recvBytes % (100*1024))
					// 	logger.Infof("%v", msgBuf[:recvBytes % (100*1024)])
					// 	r <- msgBuf[:recvBytes % (100*1024)]
					// 	i++
					// }
				}
			}	
		}
	}(ctx, conn)

	return r, s, nil
}
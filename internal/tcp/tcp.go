package tcp

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/realtimelidar/tcpwsbridge/internal/cli"
	"github.com/realtimelidar/tcpwsbridge/internal/logger"
)

var (
	config cli.TcpConfigParams
	formattedAddr string
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

	s := make(chan []byte)
	r := make(chan []byte)

	go func(ctx context.Context, conn *net.TCPConn) {
		buf := [1024]byte{}
		loop: for {
			select {
			case <-ctx.Done():
				conn.Close()
				logger.Info("Closing this TCP connection")
				break loop
			case dataToSend := <-s:
				// Data to be sent to TCP server
				_, err := conn.Write(dataToSend)
				if err != nil {
					logger.Errorf("Failed to send data to TCP server: %v", err)
				}
			default:
				conn.SetReadDeadline(time.Now().Add(5 * time.Millisecond))

				n, err := conn.Read(buf[0:])
				if err != nil {
					neterr, ok := err.(net.Error)
					if !ok || !neterr.Timeout() {
						logger.Errorf("Failed to read from TCP server. %v", err)
					}
				}

				if n > 0 {
					r <- buf[:n]
				}
			}	
		}
	}(ctx, conn)

	return r, s, nil
}
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
	conn *net.TCPConn
	sendChannel chan []byte
	receiveChannel chan []byte
	buff = make([]byte, 1024)
)

func Init(config cli.TcpConfigParams, sendChan chan []byte, recvChan chan []byte) {
	formatted := fmt.Sprintf("%s:%d", config.Host, config.Port)
	addr, err := net.ResolveTCPAddr("tcp", formatted)

	if err != nil {
		logger.Fatalf("Invalid TCP address: %s", formatted)
	}

	conn, err = net.DialTCP("tcp", nil, addr)

	if err != nil {
		logger.Fatalf("Failed to connect to TCP server: %v", err)
	}

	sendChannel = sendChan
	receiveChannel = recvChan

	logger.Info("Connected to TCP server")
}

func Run(ctx context.Context) {
	loop: for {
		select {
		case <-ctx.Done():
			break loop
		case data := <-sendChannel:
			_, err := conn.Write(data)

			if err != nil {
				logger.Errorf("Failed to send data to TCP server: %v", err)
			}
		default:
			conn.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
			n, err := conn.Read(buff[0:])

			if err != nil {
				neterr, ok := err.(net.Error)
				if !ok || !neterr.Timeout() {
					logger.Errorf("Failed to read from TCP server. %v", err)
				}
			}
			
			receiveChannel <- buff[:n]
			logger.Debugf("Received %d bytes from server", n)
		}
	}
}

func Close() {
	if conn != nil {
		conn.Close()
		conn = nil
	}
}
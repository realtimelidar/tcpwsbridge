package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/realtimelidar/tcpwsbridge/internal/cli"
	"github.com/realtimelidar/tcpwsbridge/internal/http"
	"github.com/realtimelidar/tcpwsbridge/internal/logger"
	"github.com/realtimelidar/tcpwsbridge/internal/tcp"
	"github.com/realtimelidar/tcpwsbridge/internal/ws"
)

var (
	config cli.ConfigParams
)

func main() {
	if err := logger.Init("tcpwsbridge"); err != nil {
		panic(err)
	}

	// Listen to signals (SIGINT, SIGTERM) to handle Ctrl+C etc
	sigs := make(chan os.Signal, 1)
	rawArgs := os.Args

	// Default arg values
	config = cli.ConfigParams{
		DebugEnabled: false,

		Tcp: cli.TcpConfigParams {
			Host: "127.0.0.1",
			Port: 4567,
		},

		Websockets: cli.WebsocketConfigParams {
			Url: "ws://127.0.0.1/",
		},
	}

	cli.ParseConfig(rawArgs, &config)

	ctx, cancel := context.WithCancel(context.Background())
	tcpSendChan := make(chan []byte)
	tcpRecvChan := make(chan []byte)

	tcp.Init(config.Tcp, tcpSendChan, tcpRecvChan)
	go tcp.Run(ctx)

	go ws.Init(ctx, tcpSendChan, tcpRecvChan)
	http.Run(config.Websockets)

	// Listen for signals, then shutdown if detected
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	logger.Info("Exiting...")
	cancel()

	close(tcpSendChan)
	close(tcpRecvChan)

	tcp.Close()
}
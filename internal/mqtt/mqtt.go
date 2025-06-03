package mqtt

import (
	"context"
	"encoding/binary"
	"fmt"

	pMqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/realtimelidar/tcpwsbridge/internal/cli"
	"github.com/realtimelidar/tcpwsbridge/internal/logger"
	"github.com/realtimelidar/tcpwsbridge/internal/tcp"
)

var (
	client       pMqtt.Client                     = nil
)


// Initialize Mqtt middleware
// Open a connection with the Mqtt broker from config file and start waiting for messages
// topicInfo: Array with To/From topic pairs
// receivedMessagesChan: Read-only channel to receive Mqtt messages from localhost client
func Init(ctx context.Context, config cli.MqttConfigParams) {
	opts := pMqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", config.Host, config.Port))
	opts.SetClientID("serverbridge-" + uuid.NewString())
	opts.SetUsername(config.Username)
	opts.SetPassword(config.Password)
	opts.SetAutoReconnect(true)

	recvChan, sendChan, err := tcp.NewConnection(ctx)
	cborHeader := [13]byte{ 108, 73, 110, 115, 101, 114, 116, 80, 111, 105, 110, 116, 115 }

	if err != nil {
		logger.Fatalf("Could not create CaptureDevice connection: %v", err)
	}

	// First send magic number
	sendChan <- []byte{ 76, 105, 100, 97, 114, 83, 101, 114, 118, 32, 80, 114, 111, 116, 111, 99, 111, 108 }
	<- recvChan

	// HELLO message
	sendChan <- []byte{ 34, 0, 0, 0, 0, 0, 0, 0, 161, 101, 72, 101, 108, 108, 111, 161, 112, 112, 114, 111, 116, 111, 99, 111, 108, 95, 118, 101, 114, 115, 105, 111, 110, 4 }
	<- recvChan

	// ConnectionMode CaptureDevice message
	sendChan <- []byte{ 46, 0, 0, 0, 0, 0, 0, 0, 161, 110, 67, 111, 110, 110, 101, 99, 116, 105, 111, 110, 77, 111, 100, 101, 161, 102, 100, 101, 118, 105, 99, 101, 109, 67, 97, 112, 116, 117, 114, 101, 68, 101, 118, 105, 99, 101 }
	<- recvChan

	logger.Info("Opened CaptureDevice connection")

	opts.SetOnConnectHandler(func(c pMqtt.Client) {
		token := client.Subscribe(config.Topic, 0, func(client pMqtt.Client, msg pMqtt.Message) {
			payload := msg.Payload()
			payloadLen := len(payload)
			
			logger.Infof("[CaptureDevice] Received %d bytes", payloadLen)
			// logger.Infof("%v", payload)

			msgLen := uint64(8 + len(cborHeader) + payloadLen)
			// encodedLen := msgLen | (13 << 32)

			// InsertPoint messages are not big enough to overflow RAM
			msgBuff := make([]byte, msgLen)

			binary.LittleEndian.PutUint64(msgBuff, msgLen)
			
			for i := 0; i < 13; i++ {
				msgBuff[i + 8] = cborHeader[i]
			}

			for i := 0; i < payloadLen; i++ {
				msgBuff[i + 13 + 8] = payload[i]
			}

			sendChan <- msgBuff
		})

		if token.Wait() && token.Error() != nil {
			logger.Fatalf("Failed to subscribe to pointcloud topic: %v", token.Error())
		}
	})

	client = pMqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		logger.Fatalf("Mqtt: Could not connect to Mqtt remote host: %v", token.Error())
	}

	logger.Infof("Mqtt: Connected to Mqtt remote host (%s)", config.Host)


}

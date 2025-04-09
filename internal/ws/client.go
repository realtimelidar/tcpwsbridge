package ws

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/realtimelidar/tcpwsbridge/internal/logger"
	"github.com/realtimelidar/tcpwsbridge/internal/tcp"
)

var (
	ClientPool map[uint32]*Client = map[uint32]*Client{}
)

const (
	pongWait = 35
	pingPeriod = 30 * time.Second
)

type Client struct {
	Socket *websocket.Conn
	Id uint32

	SendChan chan []byte
	RecvChan chan []byte

	Mutex *sync.RWMutex

	Terminate chan struct{}
	ShouldTerminate bool

	LastPingTimestamp int64
}

func GetNextFreeId() (uint32, error) {
	id := uint32(0)
	mtx.RLock()
	for {
		if _, ok := ClientPool[id]; !ok {
			mtx.RUnlock()
			return id, nil
		}
		id++

		if id > uint32(len(ClientPool)) {
			break
		}
	}
	mtx.RUnlock()
	return  0, fmt.Errorf("client pool is full")
}

func (c *Client) Initilize(ctx context.Context) {
	childCtx, cancel := context.WithCancel(ctx)

	recv, send, err := tcp.NewConnection(childCtx)

	if err != nil {
		logger.Errorf("Could not create TCP connection for client %d", c.Id)

		cancel()
		c.Close()
		return
	}

	c.SendChan = send
	c.RecvChan = recv

	go c.Read(childCtx)
	go c.Write(childCtx)

	<- c.Terminate
	cancel()
	c.Close()
}

func (c *Client) Read(ctx context.Context){
	c.Socket.SetPongHandler(func(string) error {
		c.SetLastTimestamp(-1)
		return nil
	})

	loop: for {
		select {
		case <-ctx.Done():
			break loop
		default:
			if c.GetShouldTerminate() {
				break loop
			}

			mType, p, err := c.Socket.ReadMessage()

			if err != nil {
				c.Terminate <- struct{}{}
				c.SetShouldTerminate(true)
				continue
			}

			if mType == websocket.BinaryMessage {
				logger.Infof("[client %d] websocket > tcp (%d bytes)", c.Id, len(p))
				c.SendChan <- p
			} else {
				logger.Errorf("Received unsupported message type: %d", mType)
			}
		}
	}
}

func (c *Client) Write(ctx context.Context){
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	loop: for {
		select {
		case <-ctx.Done():
			break loop
		case <-ticker.C:
			if c.GetLastTimestamp() < 0 {
				err := c.Socket.WriteMessage(websocket.PingMessage, nil)

				if err != nil {
					if !c.GetShouldTerminate() {
						c.Terminate <- struct{}{}
						c.SetShouldTerminate(true)
					}

					continue
				}

				c.SetLastTimestamp(time.Now().Unix())
			} else {
				logger.Errorf("Lost connection (no ping) with client %d", c.Id)

				if !c.GetShouldTerminate() {
					c.Terminate <- struct{}{}
					c.SetShouldTerminate(true)
				}

				continue
			}

		case data := <-c.RecvChan:
			err := c.Socket.WriteMessage(websocket.BinaryMessage, data)
			if err != nil {
				logger.Errorf("Failed to send TCP data to websocket client %d: %v", c.Id, err)
			}
			logger.Infof("[client %d] websocket < tcp (%d bytes)", c.Id, len(data))
		}

	}
}

func (c *Client) Close() {
	c.Socket.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInvalidFramePayloadData, ""), time.Time{})
	c.Socket.Close()

	close(c.SendChan)
	close(c.RecvChan)

	delete(ClientPool, c.Id)
	logger.Infof("Disconnected client %d", c.Id)
}

func (c *Client) SetShouldTerminate(state bool) {
	c.Mutex.Lock()
	c.ShouldTerminate = state
	c.Mutex.Unlock()
}

func (c *Client) GetShouldTerminate() bool {
	var state bool
	c.Mutex.RLock()
	state = c.ShouldTerminate
	c.Mutex.RUnlock()
	return state
}

func (c *Client) SetLastTimestamp(ts int64) {
	c.Mutex.Lock()
	c.LastPingTimestamp = ts
	c.Mutex.Unlock()
}

func (c *Client) GetLastTimestamp() int64 {
	var value int64
	c.Mutex.RLock()
	value = c.LastPingTimestamp
	c.Mutex.RUnlock()
	return value
}
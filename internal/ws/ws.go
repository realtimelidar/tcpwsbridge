package ws

import (
	"context"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	myhttp "github.com/realtimelidar/tcpwsbridge/internal/http"
	"github.com/realtimelidar/tcpwsbridge/internal/logger"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  0,
		WriteBufferSize: 0,
		CheckOrigin: func (r *http.Request) bool {
			return true
		},
	}

	ctx context.Context
	mtx sync.RWMutex = sync.RWMutex{}
)

func wsHandler(w http.ResponseWriter, r *http.Request) {
	logger.Infof("Incoming request from %s", r.RemoteAddr)
	ws, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		logger.Errorf("Failed to upgrade websocket connection: %v", err)
		w.WriteHeader(500)
		return
	}

	clientId, err := GetNextFreeId()

	if err != nil {
		logger.Errorf("Server is full")
		w.WriteHeader(500)
		return
	}

	c := &Client{
		Socket: ws,
		Id: clientId,

		SendChan: make(chan []byte),
		Mutex: &sync.RWMutex{},

		LastPingTimestamp: -1,
		ShouldTerminate: false,
		Terminate: make(chan struct{}),
	}

	mtx.Lock()
	ClientPool[c.Id] = c
	mtx.Unlock()

	logger.Infof("New client connected with id %d", c.Id)

	go c.Initilize(ctx)
}

func attachToServer() {
	myhttp.AddFuncHandler("/", wsHandler)
}

func Init(c context.Context) {
	ctx = c
	attachToServer()
}
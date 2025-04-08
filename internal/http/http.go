package http

import (
	"net/http"

	"github.com/realtimelidar/tcpwsbridge/internal/cli"
	"github.com/realtimelidar/tcpwsbridge/internal/logger"
)

func AddFuncHandler(path string, handler func(http.ResponseWriter, *http.Request)) {
	http.HandleFunc(path, handler)
}

func AddHandler(path string, handler http.Handler) {
	http.Handle(path, handler)
}

func Run(config cli.WebsocketConfigParams) {
	logger.Infof("Running at %s", config.Url)
	http.ListenAndServe(config.Url, nil)
}
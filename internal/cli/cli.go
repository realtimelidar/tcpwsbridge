package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/realtimelidar/tcpwsbridge/internal/logger"
	"gopkg.in/yaml.v3"
)

type TcpConfigParams struct {
	Host string `yaml:"host"`
	Port int `yaml:"port"`
}

type WebsocketConfigParams struct {
	Url string `yaml:"url"`
}

type ConfigParams struct {
	DebugEnabled bool `yaml:"debug"`

	Tcp TcpConfigParams `yaml:"tcp"`
	Websockets WebsocketConfigParams `yaml:"ws"`
}

const (
	USAGE_TEXT string = `tcpwsbridge: Bridge between lidarserv TCP server and Potree viewer using websockets.
Usage: tcpwsbridge [options]

Options:
	--tcphost	Set lidarserv TCP host, default: 127.0.0.1
	--tcpport	Set lidarserv TCP port, default: 4567
	--wsurl		Set Potree websocket URL ([schema]://[host][:port][path]), default: ws://127.0.0.1/

	--help, -h	Show help and usage text
	
Example:
	tcpwsbridge
	tcpwsbridge --tcphost 10.0.0.12
	tcpwsbridge --tcphost 10.0.0.12 --wsurl ws://10.0.0.12
	tcpwsbridge --wsurl wss://example.com/bride`
)

func PrintHelp() {
	fmt.Println(USAGE_TEXT)
}

func ParseConfig(args []string, a *ConfigParams) {
	fileName := "config.yaml"

	for idx, arg := range args {
		// First arg is the executable path
		if idx == 0 {
			continue
		}

		// If "--help", just print help and exit
		if strings.Compare(arg, "--help") == 0 || strings.Compare(arg, "-h") == 0 {
			PrintHelp()
			os.Exit(0)
		}

		// More cli args should be added here
		// ...
	}

		// If the config file does not exist,
	// a file with the same name with template is created
	if _, err := os.Stat(fileName); os.IsNotExist(err) {
		logger.Infof("Config file %s not found, generated template", fileName)
		fd, err := os.Create(fileName)

		if err != nil {
			logger.Debugf("Could not create config file template: %v", err)
			os.Exit(1)
		}

		bytes, err := yaml.Marshal(a)

		if err != nil {
			logger.Debugf("Could not marshal config file template: %v", err)
			os.Exit(1)
		}

		n, err := fd.Write(bytes)

		if n <= 0 || err != nil {
			logger.Debugf("Could not write config file template: %v", err)
			os.Exit(1)
		}

		fd.Close()
		os.Exit(0)
	}

	content, err := os.ReadFile(fileName)

	if err != nil {
		logger.Fatalf("Could not read config (%s): %v", fileName, err)
	}

	err = yaml.Unmarshal(content, a)

	if err != nil {
		logger.Fatalf("Could not unmarshal (%s): %v", fileName, err)
	}

	logger.Debug("Loaded config from file " + fileName)
}
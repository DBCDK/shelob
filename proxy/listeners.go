package proxy

import (
	"net"
	"github.com/kavu/go_reuseport"
	"github.com/dbcdk/shelob/util"
	"strconv"
	"github.com/dbcdk/shelob/logging"
	"net/http"
	"go.uber.org/zap"
	"github.com/dbcdk/shelob/mux"
)

var (
	log = logging.GetInstance()
)

func CreateListener(proto string, laddr string, reusePort bool) (net.Listener, error) {
	if reusePort {
		return reuseport.Listen(proto, laddr)
	} else {
		return net.Listen(proto, laddr)
	}
}

func StartProxyServer(config *util.Config) {
	httpAddr := ":" + strconv.Itoa(config.HttpPort)

	listener, err := CreateListener("tcp", httpAddr, config.ReuseHttpPort)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer listener.Close()

	proxyServer := &http.Server{
		Handler: RedirectHandler(config),
	}

	log.Info("Shelob started on port "+strconv.Itoa(config.HttpPort),
		zap.String("event", "started"),
		zap.Int("port", config.HttpPort),
	)

	log.Fatal(proxyServer.Serve(listener).Error(),
		zap.String("event", "shutdown"),
		zap.Int("port", config.HttpPort),
	)
}

func StartAdminServer(config *util.Config) {
	httpAddr := ":" + strconv.Itoa(config.MetricsPort)

	listener, err := CreateListener("tcp", httpAddr, false)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer listener.Close()

	adminServer := &http.Server{
		Handler: mux.CreateAdminMux(config),
	}

	log.Info("Shelob metrics started on port "+strconv.Itoa(config.MetricsPort),
		zap.String("event", "started"),
		zap.Int("port", config.MetricsPort),
	)

	log.Fatal(adminServer.Serve(listener).Error(),
		zap.String("event", "shutdown"),
		zap.Int("port", config.MetricsPort),
	)
}
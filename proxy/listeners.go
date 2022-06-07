package proxy

import (
	"crypto/tls"
	"fmt"
	"github.com/dbcdk/shelob/certs"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/mux"
	"github.com/dbcdk/shelob/util"
	"github.com/kavu/go_reuseport"
	"go.uber.org/zap"
	"net"
	"net/http"
	"strconv"
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

	log.Info("Shelob started HTTP-listen",
		zap.String("event", "started"),
		zap.Int("port", config.HttpPort),
	)

	log.Fatal(proxyServer.Serve(listener).Error(),
		zap.String("event", "shutdown"),
		zap.Int("port", config.HttpPort),
	)
}

func StartTLSProxyServer(config *util.Config, cl certs.CertLookup) {
	httpsAddr := ":" + strconv.Itoa(config.HttpsPort)

	listener, err := CreateListener("tcp", httpsAddr, config.ReuseHttpPort)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer listener.Close()

	selfSigned, err := certs.SelfSignedCert()
	if err != nil {
		log.Warn("Failed to issue self-signed cert, tls-connections with no matching sni-cert will be disconnected",
			zap.String("error", err.Error()))
	}

	proxyServer := &http.Server{
		Handler: RedirectHandler(config),
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			GetCertificate: func(info *tls.ClientHelloInfo) (certificate *tls.Certificate, e error) {
				if cert := cl.Lookup(info.ServerName); cert != nil {
					return cert, nil
				}
				log.Warn("Unable to find and serve certificate for host",
					zap.String("host", info.ServerName),
					zap.Strings("available-certs", cl.CertKeys()),
				)
				if selfSigned != nil {
					return selfSigned, nil
				} else {
					return nil, fmt.Errorf("No matching sni-cert and no self-signed cert to serve")
				}
			},
		},
	}

	log.Info("Shelob started HTTPS-listen",
		zap.String("event", "started"),
		zap.Int("port", config.HttpsPort),
	)

	log.Fatal(proxyServer.ServeTLS(listener, "", "").Error(),
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

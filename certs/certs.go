package certs

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/dbcdk/shelob/kubernetes"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/util"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"strings"
	"sync"
	"time"
)

var log = logging.GetInstance()

type CertLookup interface {
	Lookup(hostName string) *tls.Certificate
	RegisterValidityMonitoring()
}

type CertHandler struct {
	config     *util.Config
	certs      map[string]*tls.Certificate
	queueMutex sync.Mutex
	queue      []util.Reload
	certValidity *prometheus.GaugeVec
	certValidityLastUpdated prometheus.Gauge
}

func New(config *util.Config, certUpdateChan chan util.Reload) CertLookup {
	handler := &CertHandler{
		config:     config,
		queueMutex: sync.Mutex{},
		queue:      make([]util.Reload, 0),
	}
	if config.CertNamespace != "" {
		go handler.reconcileCerts(certUpdateChan)
	} else {
		log.Info("Certificate loader disabled, namespace unset")
	}
	return handler
}

func (ch *CertHandler) Lookup(hostName string) (cert *tls.Certificate) {
	if cert, _ = ch.certs[hostName]; cert == nil && ch.config.WildcardCertPrefix != "" {
		parts := strings.Split(hostName, ".")[1:]
		cert = ch.certs[fmt.Sprintf("%s.%s", ch.config.WildcardCertPrefix, strings.Join(parts, "."))]
	}
	return
}

func (ch *CertHandler) reconcileCerts(certUpdateChan chan util.Reload) {

	ch.trigger(util.NewReload("initial"))
	go ch.poll(func(reload util.Reload) {
		delay := time.Now().Sub(reload.Time)
		log.Info("Loading certs",
			zap.String("delay", delay.String()),
			zap.String("reason", reload.Reason),
			zap.String("event", "reload-certs"),
		)
		certs, err := kubernetes.GetCerts(ch.config, ch.config.CertNamespace)
		if err != nil {
			log.Error("Failed to reload certificates",
				zap.String("error", err.Error()),
			)
			//sleep for an extra ReloadRollup time before retrying
			time.Sleep(time.Duration(ch.config.ReloadRollup) * time.Second)
			ch.trigger(util.NewReload("retry"))
			return
		}
		ch.certs = certs
		ch.checkValidity(certs)
	})

	go kubernetes.WatchSecrets(ch.config, certUpdateChan)

	for {
		select {
		case reload := <-certUpdateChan:
			delay := time.Now().Sub(reload.Time)
			log.Debug("Certificate reload requested",
				zap.String("delay", delay.String()),
				zap.String("reason", reload.Reason),
			)
			ch.trigger(reload)
		case <-time.After(time.Second * time.Duration(ch.config.ReloadEvery)):
			log.Debug("Reload-every time elapsed without updates, forcing reload of backends")
			ch.trigger(util.NewReload("reload-every-time-elapsed"))
		}
	}
}

func (ch *CertHandler) trigger(reload util.Reload) {
	ch.queueMutex.Lock()
	defer ch.queueMutex.Unlock()
	ch.queue = append(ch.queue, reload)
}

func (ch *CertHandler) poll(reload func(update util.Reload)) {
	var last util.Reload
	for {
		if len(ch.queue) > 0 {
			ch.queueMutex.Lock()
			discarded := len(ch.queue) - 1
			last, ch.queue = ch.queue[discarded], ch.queue[:discarded]
			if discarded > 0 {
				log.Info("Cert reload events throttled",
					zap.Int("discarded", discarded),
				)
				ch.queue = make([]util.Reload, 0)
			}
			ch.queueMutex.Unlock()
			reload(last)
		}

		time.Sleep(time.Duration(ch.config.ReloadRollup) * time.Second)
	}
}

func (ch *CertHandler) RegisterValidityMonitoring() {
	ch.certValidity = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shelob_cert_expiry_days",
		Help: "Number of days until expiry for shelob TLS-certificates",
	}, []string{"domain"})
	ch.certValidityLastUpdated = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "shelob_cert_expiry_last_update_epoch",
		Help: "Unix time/epoch of last successful certificate monitor update",
	})

	prometheus.MustRegister(ch.certValidity, ch.certValidityLastUpdated)
}

func (ch *CertHandler) checkValidity(certificates map[string]*tls.Certificate) {
	now := time.Now()
	for n, c := range certificates {
		if len(c.Certificate) > 0 {
			if cert, err := x509.ParseCertificate(c.Certificate[0]); err == nil {
				ch.certValidity.With(prometheus.Labels{
					"domain": n,
				}).Set(cert.NotAfter.Sub(now).Hours()/24)
			} else {
				log.Error("Parse of certificate for domain failed",
					zap.String("domain", n),
				)
				return
			}
		}
	}
	ch.certValidityLastUpdated.SetToCurrentTime()
}
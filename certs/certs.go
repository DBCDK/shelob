package certs

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/dbcdk/shelob/kubernetes"
	"github.com/dbcdk/shelob/localfs"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/util"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"math"
	"strings"
	"sync"
	"time"
)

var log = logging.GetInstance()

type CertLookup interface {
	CertKeys() []string
	Lookup(hostName string) *tls.Certificate
}

type CertHandler struct {
	config                  *util.Config
	certs                   map[string]*tls.Certificate
	queueMutex              sync.Mutex
	queue                   []util.Reload
	certValidity            *prometheus.GaugeVec
	certValidityLastUpdated prometheus.Gauge
	reconcileMethod         ReconcileMethod
}

type ReconcileMethod int64

const (
	RECONCILE_METHOD_DISABLED = iota
	RECONCILE_METHOD_KUBERNETES
	RECONCILE_METHOD_FILES
)

func New(config *util.Config, certUpdateChan chan util.Reload) (CertLookup, error) {
	var reconcileMethod ReconcileMethod
	if config.CertNamespace != "" {
		reconcileMethod = RECONCILE_METHOD_KUBERNETES
	} else if len(config.CertFilePairMap) > 0 {
		reconcileMethod = RECONCILE_METHOD_FILES
	} else {
		log.Info("Certificate loader disabled, neither namespace nor static file map is set")
		reconcileMethod = RECONCILE_METHOD_DISABLED
	}
	handler := &CertHandler{
		config:          config,
		queueMutex:      sync.Mutex{},
		queue:           make([]util.Reload, 0),
		reconcileMethod: reconcileMethod,
	}
	handler.registerValidityMonitoring()
	if reconcileMethod != RECONCILE_METHOD_DISABLED {
		return handler, handler.reconcileCerts(certUpdateChan)
	} else {
		return handler, nil
	}
}

func (ch *CertHandler) CertKeys() []string {
	keys := make([]string, len(ch.certs))
	for n, _ := range ch.certs {
		keys = append(keys, n)
	}
	return keys
}

func (ch *CertHandler) Lookup(hostName string) (cert *tls.Certificate) {
	if cert, _ = ch.certs[hostName]; cert == nil && ch.config.WildcardCertPrefix != "" {
		parts := strings.Split(hostName, ".")[1:]
		cert = ch.certs[fmt.Sprintf("%s.%s", ch.config.WildcardCertPrefix, strings.Join(parts, "."))]
	}
	return
}

func (ch *CertHandler) GetCerts() (map[string]*tls.Certificate, error) {
	if ch.reconcileMethod == RECONCILE_METHOD_KUBERNETES {
		return kubernetes.GetCerts(ch.config, ch.config.CertNamespace)
	} else {
		return localfs.GetCerts(ch.config)
	}
}

func (ch *CertHandler) WatchSecrets(certUpdateChan chan util.Reload) error {
	if ch.reconcileMethod == RECONCILE_METHOD_KUBERNETES {
		return kubernetes.WatchSecrets(ch.config, certUpdateChan)
	} else {
		return localfs.WatchSecrets(ch.config, certUpdateChan)
	}
}

func (ch *CertHandler) reconcileCerts(certUpdateChan chan util.Reload) error {

	ch.trigger(util.NewReload("initial"))
	go ch.poll(func(reload util.Reload) {
		delay := time.Now().Sub(reload.Time)
		log.Info("Loading certs",
			zap.String("delay", delay.String()),
			zap.String("reason", reload.Reason),
			zap.String("event", "reload-certs"),
		)
		certs, err := ch.GetCerts()
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

	// Watchers themselves will fork if no errors are returned here
	err := ch.WatchSecrets(certUpdateChan)
	if err != nil {
		return err
	}

	go func() {
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
	}()

	return nil
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

func (ch *CertHandler) registerValidityMonitoring() {
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

	ch.certValidity.Reset()
	//^^ Reset should be fine, because the block below shouldn't return.
	//Panics can still occur, but this should raise another alert
	for n, c := range certificates {
		if len(c.Certificate) > 0 {
			// by setting expiry days to something low, we assure an alert is triggered for errors
			var expiryDays = -1.
			if cert, err := x509.ParseCertificate(c.Certificate[0]); err == nil {
				expiryDays = cert.NotAfter.Sub(now).Hours() / 24
			} else {
				log.Error("Parse of certificate for domain failed",
					zap.String("domain", n),
				)
			}
			ch.certValidity.With(prometheus.Labels{
				"domain": n,
			}).Set(math.Floor(expiryDays))
		}
	}

	ch.certValidityLastUpdated.SetToCurrentTime()
}

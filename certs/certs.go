package certs

import (
	"crypto/tls"
	"github.com/dbcdk/shelob/kubernetes"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/util"
	"go.uber.org/zap"
	"sync"
	"time"
)

var log = logging.GetInstance()

type CertLookup interface {
	Lookup(hostName string) *tls.Certificate
}

type CertHandler struct {
	config     *util.Config
	certs      map[string]*tls.Certificate
	queueMutex sync.Mutex
	queue      []util.Reload
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

func (ch *CertHandler) Lookup(hostName string) *tls.Certificate {
	return ch.certs[hostName]
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

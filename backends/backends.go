package backends

import (
	"errors"
	"github.com/dbcdk/shelob/kubernetes"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/util"
	"go.uber.org/zap"
	"sync"
	"time"
)

var (
	log                = logging.GetInstance()
	queueMutex         = sync.Mutex{}
	queue              = make([]util.Reload, 0)
	consecutive_errors = 0
)

func GetFrontends(config *util.Config, timeout time.Duration) (map[string]*util.Frontend, error) {
	resChan := make(chan map[string]*util.Frontend, 1)
	errChan := make(chan error, 1)
	go func() {
		frontends, err := kubernetes.UpdateFrontends(config)
		if err != nil {
			errChan <- err
		} else {
			resChan <- frontends
		}
	}()

	select {
	case backends := <-resChan:
		return backends, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(timeout):
		return nil, errors.New("timeout waiting for Kubernetes")
	}
}

func BackendManager(config *util.Config, updateChan chan util.Reload) (err error) {
	go func() {
		for {
			select {
			case update := <-updateChan:
				delay := time.Now().Sub(update.Time)
				log.Debug("Backend reload requested",
					zap.String("delay", delay.String()),
					zap.String("reason", update.Reason),
				)
				trigger(update)
			case <-time.After(time.Second * time.Duration(config.ReloadEvery)):
				log.Debug("Reload-every time elapsed without updates, forcing reload of backends")
				trigger(util.NewReload("reload-every-time-elapsed"))
			}
		}
	}()

	// watch changes in kubernetes api and trigger update
	if !config.DisableWatch {
		go kubernetes.WatchBackends(config, updateChan)
	} else {
		log.Info("API watch has been disabled by config flag, reloading changes with fixed interval only",
			zap.Int("interval", config.ReloadEvery),
		)
	}

	trigger(util.NewReload("initial"))
	poll(config.ReloadRollup, func(update util.Reload) {
		delay := time.Now().Sub(update.Time)
		log.Info("Loading backends",
			zap.String("delay", delay.String()),
			zap.String("reason", update.Reason),
			zap.String("event", "reload"),
		)
		frontends, err := GetFrontends(config, 5*time.Second)

		if err != nil {
			log.Error(err.Error(),
				zap.String("event", "updateError"),
				zap.Int("consecutiveErrors", consecutive_errors),
			)

			consecutive_errors += 1
			trigger(util.NewReload("retry"))
		}

		consecutive_errors = 0

		config.Frontends = frontends
		config.Counters.Reloads.Inc()
		config.LastUpdate = time.Now()
		config.Counters.LastUpdate.Set(float64(config.LastUpdate.Unix()))
		config.HasBeenUpdated = true
	})

	return
}

func trigger(reload util.Reload) {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	queue = append(queue, reload)
}

func poll(reloadRollup int, reload func(update util.Reload)) {
	var last util.Reload
	for {
		if len(queue) > 0 {
			queueMutex.Lock()
			discarded := len(queue) - 1
			last, queue = queue[discarded], queue[:discarded]
			if discarded > 0 {
				log.Info("Backend reload events throttled",
					zap.Int("discarded", discarded),
				)
				queue = make([]util.Reload, 0)
			}
			queueMutex.Unlock()
			reload(last)
		}

		time.Sleep(time.Duration(reloadRollup) * time.Second)
	}
}

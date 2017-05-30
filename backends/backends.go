package backends

import (
	"errors"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/marathon"
	"github.com/dbcdk/shelob/proxy"
	"github.com/dbcdk/shelob/util"
	"github.com/vulcand/oxy/forward"
	"go.uber.org/zap"
	"time"
)

var (
	log = logging.GetInstance()
)

func GetBackends(config *util.Config, timeout time.Duration) (map[string][]util.Backend, error) {
	resChan := make(chan map[string][]util.Backend, 1)
	errChan := make(chan error, 1)
	go func() {
		backends, err := marathon.UpdateBackends(config)
		if err != nil {
			errChan <- err
		} else {
			resChan <- backends
		}
	}()

	select {
	case backends := <-resChan:
		return backends, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(timeout):
		return nil, errors.New("timeout waiting for Marathon")
	}
}

func BackendManager(config *util.Config, forwarder *forward.Forwarder, updateChan chan time.Time) error {
	consecutive_errors := 0
	for {
		backends, err := GetBackends(config, 5*time.Second)

		if err != nil {
			log.Error(err.Error(),
				zap.String("event", "updateError"),
				zap.Int("consecutiveErrors", consecutive_errors),
			)

			time.Sleep(time.Second)
			consecutive_errors += 1
			continue
		}

		consecutive_errors = 0

		config.Backends = backends
		config.RrbBackends = proxy.CreateRoundRobinBackends(forwarder, backends)
		config.Counters.Reloads.Inc()

		select {
		case eventTime := <-updateChan:
			delay := time.Now().Sub(eventTime)
			log.Info("Update requested",
				zap.String("event", "reload"),
				zap.String("delay", delay.String()),
			)
		case <-time.After(time.Second * time.Duration(config.UpdateInterval)):
			log.Info("No changes for a while, forcing reload",
				zap.String("event", "reload"),
			)
		}
	}
}

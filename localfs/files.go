package localfs

import (
	"crypto/tls"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/util"
	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
	"io/ioutil"
)

var log = logging.GetInstance()

func GetCerts(config *util.Config) (map[string]*tls.Certificate, error) {
	certs := make(map[string]*tls.Certificate)
	for name, files := range config.CertFilePairMap {
		pubRaw, err := ioutil.ReadFile(files.PublicKey)
		if err != nil {
			return nil, err
		}
		privRaw, err := ioutil.ReadFile(files.PrivateKey)
		if err != nil {
			return nil, err
		}
		cert, err := util.ParseX509(pubRaw, privRaw)
		if err != nil {
			return nil, err
		}
		certs[name] = cert
	}
	return certs, nil
}

func WatchSecrets(config *util.Config, updateChan chan util.Reload) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Info("Received inotify write event for file",
						zap.String("file", event.Name),
					)
					updateChan <- util.NewReload("inotify-write-event")
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Error("Inotify watch error",
					zap.String("error", err.Error()),
				)
			case stopping := <-config.State.ShutdownChan:
				if stopping {
					log.Info("Stopping inotify watch loop")
					watcher.Close()
					return
				}
			}
		}
	}()

	for _, pair := range config.CertFilePairMap {
		err = watcher.Add(pair.PublicKey)
		if err != nil {
			return err
		}
		err = watcher.Add(pair.PrivateKey)
		if err != nil {
			return err
		}
	}
	return nil
}

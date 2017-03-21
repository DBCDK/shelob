package signals

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/dbcdk/shelob/util"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func RegisterSignals(config *util.Config, shutdownChan chan bool) {
	if config.ShutdownDelay > 0 {
		signals := make(chan os.Signal, 1)

		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			signal := <-signals
			shutdownChan <- true

			delay := time.Second * time.Duration(config.ShutdownDelay)

			log.WithFields(log.Fields{
				"app":   "shelob",
				"event": signal.String(),
			}).Info(fmt.Sprintf("recieved signal '%v', will shutdown after %vs. Send '%v' again to shutdown now", signal, delay, signal))

			select {
			case <-signals:
			case <-time.After(delay):
			}

			os.Exit(0)
		}()
	}
}

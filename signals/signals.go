package signals

import (
	"fmt"
	"github.com/dbcdk/shelob/logging"
	"github.com/dbcdk/shelob/util"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	log = logging.GetInstance()
)

func RegisterSignals(config *util.Config) {
	if config.ShutdownDelay > 0 {
		signals := make(chan os.Signal, 1)

		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			signal := <-signals
			config.State.ShutdownInProgress = true

			delay := time.Second * time.Duration(config.ShutdownDelay)

			log.Info(fmt.Sprintf("recieved signal '%v', will shutdown after %vs. Send '%v' again to shutdown now", signal, delay, signal),
				zap.String("event", "signal.String()"),
			)

			select {
			case <-signals:
			case <-time.After(delay):
			}

			os.Exit(0)
		}()
	}
}

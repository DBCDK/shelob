package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func registerSignals() {
	if *shutdownDelay > 0 {
		signals := make(chan os.Signal, 1)

		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			signal := <-signals
			shutdownInProgress = true

			delay := time.Second * time.Duration(*shutdownDelay)

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

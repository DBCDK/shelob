package mux

import (
	"github.com/dbcdk/shelob/handlers"
	"github.com/dbcdk/shelob/util"
	"net/http"
)

func CreateWebMux(config *util.Config) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/status", http.HandlerFunc(handlers.CreateStatusHandler(config)))

	return mux
}

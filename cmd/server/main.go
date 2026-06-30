package main

import (
	"log/slog"
	"net/http"
	"os"

	"jarvis-agent/internal/api"
	"jarvis-agent/internal/config"
	"jarvis-agent/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()
	runtime := service.NewRuntime(cfg, logger)
	router := api.NewRouter(runtime, logger, api.Timeout(cfg.AgentTimeout))

	addr := ":" + cfg.AppPort
	logger.Info("starting jarvis-agent", "addr", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

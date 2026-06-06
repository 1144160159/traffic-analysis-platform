package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := &config.Config{}
	logger.Info("Starting Asset Service",
		zap.Int("grpc_port", cfg.Server.GRPCPort),
		zap.Int("http_port", cfg.Server.HTTPPort))

	svc := service.New(cfg, nil, logger)
	_ = svc // placeholder — gRPC server registers here when DB available

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("Asset Service stopped")
	_ = ctx
}

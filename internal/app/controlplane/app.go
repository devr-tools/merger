package controlplaneapp

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/mergerhq/merger/internal/config"
	"github.com/mergerhq/merger/internal/controlplane"
	controlplanegrpc "github.com/mergerhq/merger/internal/controlplanegrpc"
	"github.com/mergerhq/merger/internal/events"
	"github.com/mergerhq/merger/internal/store"
	"github.com/mergerhq/merger/internal/telemetry"
	mergerv1 "github.com/mergerhq/merger/proto/merger/v1"
	"google.golang.org/grpc"
)

type App struct {
	logger      *telemetry.Logger
	httpServer  *http.Server
	grpcServer  *grpc.Server
	grpcAddress string
	bus         events.Bus
}

func New(cfg config.Config, logger *telemetry.Logger, bus events.Bus, repository store.Repository) *App {
	service := controlplane.NewService(repository)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	controlplane.NewHTTPHandler(service).Register(mux)

	grpcServer := grpc.NewServer()
	mergerv1.RegisterChangeControlServiceServer(grpcServer, controlplanegrpc.NewServer(service))

	return &App{
		logger:      logger,
		bus:         bus,
		grpcServer:  grpcServer,
		grpcAddress: cfg.Service.ControlPlaneGRPC,
		httpServer: &http.Server{
			Addr:    cfg.Service.ControlPlaneAddress,
			Handler: mux,
		},
	}
}

func (a *App) Run() error {
	if err := a.bus.Subscribe(events.EventMergeLaneAssigned, func(_ context.Context, event events.Envelope) error {
		a.logger.Info("controlplane observed lane decision", "event", event)
		return nil
	}); err != nil {
		return err
	}

	grpcListener, err := net.Listen("tcp", a.grpcAddress)
	if err != nil {
		return err
	}
	defer grpcListener.Close()

	errCh := make(chan error, 2)

	go func() {
		if serveErr := a.grpcServer.Serve(grpcListener); serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	go func() {
		if serveErr := a.httpServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	for i := 0; i < 2; i++ {
		if serveErr := <-errCh; serveErr != nil {
			return serveErr
		}
	}

	return nil
}

func (a *App) Shutdown(ctx context.Context) error {
	a.grpcServer.GracefulStop()
	return a.httpServer.Shutdown(ctx)
}

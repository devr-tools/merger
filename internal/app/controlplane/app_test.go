package controlplaneapp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devr-tools/merger/internal/access"
	"github.com/devr-tools/merger/internal/config"
	"github.com/devr-tools/merger/internal/events"
	"github.com/devr-tools/merger/internal/store"
	"github.com/devr-tools/merger/internal/telemetry"
	mergerv1 "github.com/devr-tools/merger/proto/merger/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestNewWiresTransportAccess(t *testing.T) {
	t.Setenv("MERGER_APP_READER_TOKEN", "reader-secret")
	authenticator, err := access.NewStaticTokenAuthenticator([]access.StaticToken{{
		Subject:  "dashboard",
		TokenEnv: "MERGER_APP_READER_TOKEN",
		Roles:    []access.Role{access.RoleReader},
	}})
	if err != nil {
		t.Fatalf("construct authenticator: %v", err)
	}

	app := NewWithAccess(
		config.Defaults(),
		telemetry.NewLogger("error"),
		events.NewMemoryBus(),
		store.NewMemoryRepository(),
		authenticator,
	)

	healthRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthResponse := httptest.NewRecorder()
	app.httpServer.Handler.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("expected public health check, got %d", healthResponse.Code)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/change-packets", nil)
	listResponse := httptest.NewRecorder()
	app.httpServer.Handler.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expected protected HTTP API, got %d", listResponse.Code)
	}

	listener := bufconn.Listen(1024 * 1024)
	go func() {
		_ = app.grpcServer.Serve(listener)
	}()
	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial gRPC server: %v", err)
	}
	defer func() {
		_ = conn.Close()
		app.grpcServer.Stop()
		_ = listener.Close()
	}()

	client := mergerv1.NewChangeControlServiceClient(conn)
	_, err = client.ListChangePackets(context.Background(), &mergerv1.ListChangePacketsRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected protected gRPC API, got %v", err)
	}
	authorizedContext := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer reader-secret")
	if _, err := client.ListChangePackets(authorizedContext, &mergerv1.ListChangePacketsRequest{}); err != nil {
		t.Fatalf("authorized gRPC list: %v", err)
	}
}

func TestNewRequiresAuthenticator(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected missing authenticator to panic")
		}
	}()

	NewWithAccess(
		config.Defaults(),
		telemetry.NewLogger("error"),
		events.NewMemoryBus(),
		store.NewMemoryRepository(),
		nil,
	)
}

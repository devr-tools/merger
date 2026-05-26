package controlplanegrpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/mergerhq/merger/internal/controlplane"
	controlplanegrpc "github.com/mergerhq/merger/internal/controlplanegrpc"
	"github.com/mergerhq/merger/internal/domain"
	"github.com/mergerhq/merger/internal/store"
	mergerv1 "github.com/mergerhq/merger/proto/merger/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestServerGetChangePacket(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	response, err := client.GetChangePacket(context.Background(), &mergerv1.GetChangePacketRequest{
		Ref: &mergerv1.ChangePacketRef{Id: "cp_1"},
	})
	if err != nil {
		t.Fatalf("get change packet: %v", err)
	}

	if response.GetId() != "cp_1" {
		t.Fatalf("expected cp_1, got %q", response.GetId())
	}
	if len(response.GetEvidence()) != 1 {
		t.Fatalf("expected evidence execution, got %d", len(response.GetEvidence()))
	}
}

func TestServerListChangePackets(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	response, err := client.ListChangePackets(context.Background(), &mergerv1.ListChangePacketsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("list change packets: %v", err)
	}

	if len(response.GetItems()) != 1 {
		t.Fatalf("expected one change packet, got %d", len(response.GetItems()))
	}
	if response.GetItems()[0].GetId() != "cp_2" {
		t.Fatalf("expected newest packet first, got %q", response.GetItems()[0].GetId())
	}
}

func TestServerUpdateEvidenceExecution(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	response, err := client.UpdateEvidenceExecution(context.Background(), &mergerv1.UpdateEvidenceExecutionRequest{
		ChangePacketId: "cp_1",
		Name:           "integration_tests",
		Type:           "integration_tests",
		Status:         "satisfied",
		Required:       true,
		Summary:        "ci passed",
		UpdatedBy:      "ci",
	})
	if err != nil {
		t.Fatalf("update evidence execution: %v", err)
	}

	if response.GetEvidence().GetStatus() != "satisfied" {
		t.Fatalf("expected satisfied status, got %q", response.GetEvidence().GetStatus())
	}
	if response.GetEvidence().GetUpdatedAt() == "" {
		t.Fatal("expected updated timestamp")
	}
}

func TestServerReturnsNotFound(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	_, err := client.GetChangePacket(context.Background(), &mergerv1.GetChangePacketRequest{
		Ref: &mergerv1.ChangePacketRef{Id: "missing"},
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestServerValidatesRequests(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	_, err := client.UpdateEvidenceExecution(context.Background(), &mergerv1.UpdateEvidenceExecutionRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func newTestClient(t *testing.T) (mergerv1.ChangeControlServiceClient, func()) {
	t.Helper()

	repository := store.NewMemoryRepository()
	seedChangePacket(t, repository, "cp_1", 42, time.Now().UTC())
	seedChangePacket(t, repository, "cp_2", 43, time.Now().UTC().Add(time.Minute))

	service := controlplane.NewService(repository)
	server := grpc.NewServer()
	mergerv1.RegisterChangeControlServiceServer(server, controlplanegrpc.NewServer(service))

	listener := bufconn.Listen(1024 * 1024)
	go func() {
		_ = server.Serve(listener)
	}()

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial grpc server: %v", err)
	}

	cleanup := func() {
		_ = conn.Close()
		server.GracefulStop()
		_ = listener.Close()
	}

	return mergerv1.NewChangeControlServiceClient(conn), cleanup
}

func seedChangePacket(t *testing.T, repository *store.MemoryRepository, id string, prNumber int, updatedAt time.Time) {
	t.Helper()

	packet := domain.ChangePacket{
		ID:        id,
		Repo:      domain.RepoRef{FullName: "acme/repo"},
		PR:        domain.PullRequestRef{Number: prNumber},
		MergeLane: domain.MergeLaneYellow,
		RiskSummary: domain.RiskSummary{
			Score: 30,
		},
		Evidence: []domain.EvidenceRequirement{
			{Name: "integration_tests", Type: domain.EvidenceIntegrationTests, Required: true},
		},
		UpdatedAt: updatedAt,
	}

	if err := repository.SaveChangePacket(context.Background(), packet); err != nil {
		t.Fatalf("seed change packet: %v", err)
	}
}

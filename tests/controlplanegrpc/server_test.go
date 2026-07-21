package controlplanegrpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/devr-tools/merger/internal/access"
	"github.com/devr-tools/merger/internal/controlplane"
	controlplanegrpc "github.com/devr-tools/merger/internal/controlplanegrpc"
	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/internal/store"
	mergerv1 "github.com/devr-tools/merger/proto/merger/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
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

func TestServerCapsListChangePacketsLimit(t *testing.T) {
	repository := &limitRecordingRepository{MemoryRepository: store.NewMemoryRepository()}
	client, cleanup := newTestClientWithRepository(t, repository)
	defer cleanup()

	_, err := client.ListChangePackets(context.Background(), &mergerv1.ListChangePacketsRequest{Limit: 999})
	if err != nil {
		t.Fatalf("list change packets: %v", err)
	}
	if repository.limit != controlplane.MaxListLimit {
		t.Fatalf("expected repository limit %d, got %d", controlplane.MaxListLimit, repository.limit)
	}
}

func TestServerUpdateEvidenceExecution(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	response, err := client.UpdateEvidenceExecution(context.Background(), &mergerv1.UpdateEvidenceExecutionRequest{
		ChangePacketId: "cp_1",
		Name:           "integration_tests",
		Type:           "security_review",
		Status:         "satisfied",
		Required:       false,
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
	if response.GetEvidence().GetType() != string(domain.EvidenceIntegrationTests) || !response.GetEvidence().GetRequired() {
		t.Fatalf("expected policy-owned evidence fields, got type=%q required=%t", response.GetEvidence().GetType(), response.GetEvidence().GetRequired())
	}
}

func TestServerListsEvidenceAuditEntries(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	_, err := client.UpdateEvidenceExecution(context.Background(), &mergerv1.UpdateEvidenceExecutionRequest{
		ChangePacketId: "cp_1", Name: "integration_tests", Status: "running", Summary: "started", UpdatedBy: "ci",
	})
	if err != nil {
		t.Fatalf("start evidence: %v", err)
	}
	_, err = client.UpdateEvidenceExecution(context.Background(), &mergerv1.UpdateEvidenceExecutionRequest{
		ChangePacketId: "cp_1", Name: "integration_tests", Status: "satisfied", Summary: "passed", UpdatedBy: "ci",
	})
	if err != nil {
		t.Fatalf("satisfy evidence: %v", err)
	}

	response, err := client.ListEvidenceAuditEntries(context.Background(), &mergerv1.ListEvidenceAuditEntriesRequest{ChangePacketId: "cp_1"})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(response.GetItems()) != 2 {
		t.Fatalf("expected two audit entries, got %#v", response.GetItems())
	}
	if response.GetItems()[0].GetFromStatus() != "running" || response.GetItems()[0].GetToStatus() != "satisfied" || response.GetItems()[0].GetActor() != "ci" {
		t.Fatalf("unexpected latest audit entry: %#v", response.GetItems()[0])
	}
	if response.GetItems()[0].GetOccurredAt() == "" || response.GetItems()[0].GetId() == "" {
		t.Fatalf("expected audit timestamp and ID: %#v", response.GetItems()[0])
	}

	_, err = client.ListEvidenceAuditEntries(context.Background(), &mergerv1.ListEvidenceAuditEntriesRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
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

func TestServerMapsEvidenceUpdateErrors(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	tests := []struct {
		name    string
		request *mergerv1.UpdateEvidenceExecutionRequest
		code    codes.Code
	}{
		{
			name: "empty status",
			request: &mergerv1.UpdateEvidenceExecutionRequest{
				ChangePacketId: "cp_1",
				Name:           "integration_tests",
			},
			code: codes.InvalidArgument,
		},
		{
			name: "invalid status",
			request: &mergerv1.UpdateEvidenceExecutionRequest{
				ChangePacketId: "cp_1",
				Name:           "integration_tests",
				Status:         "unknown",
			},
			code: codes.InvalidArgument,
		},
		{
			name: "missing packet",
			request: &mergerv1.UpdateEvidenceExecutionRequest{
				ChangePacketId: "missing",
				Name:           "integration_tests",
				Status:         "satisfied",
			},
			code: codes.NotFound,
		},
		{
			name: "undeclared evidence",
			request: &mergerv1.UpdateEvidenceExecutionRequest{
				ChangePacketId: "cp_1",
				Name:           "security_review",
				Status:         "satisfied",
			},
			code: codes.NotFound,
		},
		{
			name: "unattributed waiver",
			request: &mergerv1.UpdateEvidenceExecutionRequest{
				ChangePacketId: "cp_1",
				Name:           "integration_tests",
				Status:         "waived",
			},
			code: codes.InvalidArgument,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.UpdateEvidenceExecution(context.Background(), test.request)
			if status.Code(err) != test.code {
				t.Fatalf("expected %s, got %v", test.code, err)
			}
		})
	}

	_, err := client.UpdateEvidenceExecution(context.Background(), &mergerv1.UpdateEvidenceExecutionRequest{
		ChangePacketId: "cp_1",
		Name:           "integration_tests",
		Status:         "satisfied",
	})
	if err != nil {
		t.Fatalf("satisfy evidence: %v", err)
	}
	_, err = client.UpdateEvidenceExecution(context.Background(), &mergerv1.UpdateEvidenceExecutionRequest{
		ChangePacketId: "cp_1",
		Name:           "integration_tests",
		Status:         "running",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestAccessUnaryServerInterceptorEnforcesRoles(t *testing.T) {
	t.Setenv("MERGER_GRPC_READER_TOKEN", "reader-secret")
	t.Setenv("MERGER_GRPC_WRITER_TOKEN", "writer-secret")
	t.Setenv("MERGER_GRPC_ADMIN_TOKEN", "admin-secret")
	authenticator, err := access.NewStaticTokenAuthenticator([]access.StaticToken{
		{Subject: "dashboard", TokenEnv: "MERGER_GRPC_READER_TOKEN", Roles: []access.Role{access.RoleReader}},
		{Subject: "ci", TokenEnv: "MERGER_GRPC_WRITER_TOKEN", Roles: []access.Role{access.RoleEvidenceWriter}},
		{Subject: "operator", TokenEnv: "MERGER_GRPC_ADMIN_TOKEN", Roles: []access.Role{access.RoleAdmin}},
	})
	if err != nil {
		t.Fatalf("construct authenticator: %v", err)
	}

	repository := store.NewMemoryRepository()
	seedChangePacket(t, repository, "cp_1", 42, time.Now().UTC())
	client, cleanup := newTestClientWithRepository(
		t,
		repository,
		grpc.UnaryInterceptor(controlplanegrpc.AccessUnaryServerInterceptor(authenticator)),
	)
	defer cleanup()

	getRequest := &mergerv1.GetChangePacketRequest{Ref: &mergerv1.ChangePacketRef{Id: "cp_1"}}
	_, err = client.GetChangePacket(context.Background(), getRequest)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected missing credentials to be unauthenticated, got %v", err)
	}
	_, err = client.GetChangePacket(outgoingAuthorization("wrong-secret"), getRequest)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected invalid credentials to be unauthenticated, got %v", err)
	}
	_, err = client.GetChangePacket(outgoingAuthorization("writer-secret"), getRequest)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected writer read to be denied, got %v", err)
	}
	_, err = client.UpdateEvidenceExecution(outgoingAuthorization("reader-secret"), &mergerv1.UpdateEvidenceExecutionRequest{
		ChangePacketId: "cp_1",
		Name:           "integration_tests",
		Status:         "running",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected reader update to be denied, got %v", err)
	}

	if _, err := client.GetChangePacket(outgoingAuthorization("reader-secret"), getRequest); err != nil {
		t.Fatalf("reader get change packet: %v", err)
	}
	if _, err := client.ListEvidenceAuditEntries(outgoingAuthorization("reader-secret"), &mergerv1.ListEvidenceAuditEntriesRequest{ChangePacketId: "cp_1"}); err != nil {
		t.Fatalf("reader list audit history: %v", err)
	}
	if _, err := client.UpdateEvidenceExecution(outgoingAuthorization("writer-secret"), &mergerv1.UpdateEvidenceExecutionRequest{
		ChangePacketId: "cp_1",
		Name:           "integration_tests",
		Status:         "running",
	}); err != nil {
		t.Fatalf("writer update evidence: %v", err)
	}
	if _, err := client.ListChangePackets(outgoingAuthorization("admin-secret"), &mergerv1.ListChangePacketsRequest{}); err != nil {
		t.Fatalf("admin list change packets: %v", err)
	}
	if _, err := client.UpdateEvidenceExecution(outgoingAuthorization("admin-secret"), &mergerv1.UpdateEvidenceExecutionRequest{
		ChangePacketId: "cp_1",
		Name:           "integration_tests",
		Status:         "satisfied",
	}); err != nil {
		t.Fatalf("admin update evidence: %v", err)
	}
}

func TestAccessUnaryServerInterceptorAllowsDisabledModeWithoutMetadata(t *testing.T) {
	repository := store.NewMemoryRepository()
	client, cleanup := newTestClientWithRepository(
		t,
		repository,
		grpc.UnaryInterceptor(controlplanegrpc.AccessUnaryServerInterceptor(access.NewDisabledAuthenticator())),
	)
	defer cleanup()

	if _, err := client.ListChangePackets(context.Background(), &mergerv1.ListChangePacketsRequest{}); err != nil {
		t.Fatalf("disabled access mode should allow local request without metadata: %v", err)
	}
}

func newTestClient(t *testing.T) (mergerv1.ChangeControlServiceClient, func()) {
	t.Helper()

	repository := store.NewMemoryRepository()
	seedChangePacket(t, repository, "cp_1", 42, time.Now().UTC())
	seedChangePacket(t, repository, "cp_2", 43, time.Now().UTC().Add(time.Minute))
	return newTestClientWithRepository(t, repository)
}

func newTestClientWithRepository(t *testing.T, repository store.Repository, serverOptions ...grpc.ServerOption) (mergerv1.ChangeControlServiceClient, func()) {
	t.Helper()

	service := controlplane.NewService(repository)
	server := grpc.NewServer(serverOptions...)
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

func outgoingAuthorization(token string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

type limitRecordingRepository struct {
	*store.MemoryRepository
	limit int
}

func (r *limitRecordingRepository) ListChangePackets(ctx context.Context, limit int) ([]domain.ChangePacket, error) {
	r.limit = limit
	return r.MemoryRepository.ListChangePackets(ctx, limit)
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

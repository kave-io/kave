package control

import (
	"context"
	"testing"

	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/core/store/fake"
)

// scopeStore records the id ListAgents/ListPolicies are filtered by, and maps a
// known env id to its project (workspace) id.
type scopeStore struct {
	fake.Store
	gotAgentScope  string
	gotPolicyScope string
}

func (s *scopeStore) GetEnvironment(_ context.Context, id string) (*controlmodel.Environment, error) {
	if id != "env-1" {
		return nil, nil
	}
	return &controlmodel.Environment{ID: "env-1", ProjectID: "ws-1"}, nil
}

func (s *scopeStore) ListAgents(_ context.Context, scope string, _ store.Page) (store.PageResult[*controlmodel.Agent], error) {
	s.gotAgentScope = scope
	return store.PageResult[*controlmodel.Agent]{}, nil
}

func (s *scopeStore) ListPolicies(_ context.Context, scope string, _ store.Page) (store.PageResult[*controlmodel.PolicyRecord], error) {
	s.gotPolicyScope = scope
	return store.PageResult[*controlmodel.PolicyRecord]{}, nil
}

// Agents/policies are workspace-scoped; the list handlers must translate the
// caller's env id to the env's project id before filtering. Passing the env id
// straight through returned an empty list and made EnsureAgent/EnsurePolicy
// create duplicates (kave 1.3.0 bug).
func TestListAgentsScopedByProjectNotEnv(t *testing.T) {
	st := &scopeStore{}
	srv := New(st, nil)

	if _, err := srv.ListAgents(context.Background(), &controlv1.ListAgentsRequest{EnvId: "env-1"}); err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if st.gotAgentScope != "ws-1" {
		t.Fatalf("ListAgents must filter by project id ws-1, got %q", st.gotAgentScope)
	}

	if _, err := srv.ListPolicies(context.Background(), &controlv1.ListPoliciesRequest{EnvId: "env-1"}); err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	if st.gotPolicyScope != "ws-1" {
		t.Fatalf("ListPolicies must filter by project id ws-1, got %q", st.gotPolicyScope)
	}
}

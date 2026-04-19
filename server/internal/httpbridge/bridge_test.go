package httpbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/kave-io/kave/server/internal/contract"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRegisterMapsGrpcErrors(t *testing.T) {
	cases := []struct {
		name     string
		code     codes.Code
		wantHTTP int
		wantCode string
	}{
		{name: "invalid argument", code: codes.InvalidArgument, wantHTTP: http.StatusBadRequest, wantCode: "request.invalid"},
		{name: "not found", code: codes.NotFound, wantHTTP: http.StatusNotFound, wantCode: "agent.not_found"},
		{name: "already exists", code: codes.AlreadyExists, wantHTTP: http.StatusConflict, wantCode: "agent.already_exists"},
		{name: "permission denied", code: codes.PermissionDenied, wantHTTP: http.StatusForbidden, wantCode: "auth.forbidden"},
		{name: "unauthenticated", code: codes.Unauthenticated, wantHTTP: http.StatusUnauthorized, wantCode: "auth.unauthenticated"},
		{name: "failed precondition", code: codes.FailedPrecondition, wantHTTP: http.StatusConflict, wantCode: "config.invalid"},
		{name: "unimplemented", code: codes.Unimplemented, wantHTTP: http.StatusNotImplemented, wantCode: "server.unimplemented"},
		{name: "internal", code: codes.Internal, wantHTTP: http.StatusInternalServerError, wantCode: "server.internal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			Register(mux, []Route{
				{
					Path: "GET /test",
					Invoke: func(_ context.Context, _ []byte, _ url.Values, _ map[string]string) (Outcome, error) {
						return Outcome{Kind: "Agent"}, status.Error(tc.code, "boom")
					},
				},
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tc.wantHTTP {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantHTTP)
			}

			var envelope contract.ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("unmarshal envelope: %v", err)
			}
			if envelope.Error.Code != tc.wantCode {
				t.Fatalf("error code = %q, want %q", envelope.Error.Code, tc.wantCode)
			}
		})
	}
}

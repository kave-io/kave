package httpbridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/kave-io/kave/server/internal/contract"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Outcome describes the body the bridge should write.
type Outcome struct {
	Kind    string
	Data    any
	Page    *contract.Page
	Status  int
	Headers map[string]string
}

// InvokeFn produces a bridge outcome for one HTTP request.
type InvokeFn func(ctx context.Context, body []byte, query url.Values, path map[string]string) (Outcome, error)

// Route binds one "METHOD /path" pattern to an InvokeFn.
// Path must include the method prefix (e.g. "GET /api/v1/orgs") — the
// standard library mux uses that to restrict the HTTP method.
type Route struct {
	Path       string
	PathParams []string
	Invoke     InvokeFn
}

// Register installs bridge routes onto mux.
func Register(mux *http.ServeMux, routes []Route) {
	for _, route := range routes {
		route := route
		mux.HandleFunc(route.Path, func(w http.ResponseWriter, r *http.Request) {
			body, err := readBody(r)
			if err != nil {
				writeError(w, http.StatusBadRequest, "request.invalid", "invalid request body", nil)
				return
			}

			pathValues := make(map[string]string, len(route.PathParams))
			for _, name := range route.PathParams {
				pathValues[name] = r.PathValue(name)
			}

			outcome, err := route.Invoke(r.Context(), body, r.URL.Query(), pathValues)
			if err != nil {
				mapBridgeError(w, err, outcome.Kind)
				return
			}

			statusCode := outcome.Status
			if statusCode == 0 {
				statusCode = http.StatusOK
			}
			for k, v := range outcome.Headers {
				w.Header().Set(k, v)
			}

			data, err := encodeData(outcome)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "server.internal", err.Error(), nil)
				return
			}
			contract.WriteSuccess(w, statusCode, outcome.Kind, data, outcome.Page, nil)
		})
	}
}

func encodeData(outcome Outcome) (any, error) {
	switch v := outcome.Data.(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		return v, nil
	case []byte:
		return json.RawMessage(v), nil
	case proto.Message:
		raw, err := protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}.Marshal(v)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(raw), nil
	default:
		return v, nil
	}
}

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

func mapBridgeError(w http.ResponseWriter, err error, kind string) {
	st, ok := status.FromError(err)
	if !ok {
		writeError(w, http.StatusInternalServerError, "server.internal", err.Error(), nil)
		return
	}

	code, httpStatus := bridgeErrorDetails(st.Code(), kind)
	writeError(w, httpStatus, code, st.Message(), nil)
}

func bridgeErrorDetails(code codes.Code, kind string) (string, int) {
	switch code {
	case codes.InvalidArgument:
		return "request.invalid", http.StatusBadRequest
	case codes.NotFound:
		return strings.ToLower(kind) + ".not_found", http.StatusNotFound
	case codes.AlreadyExists:
		return strings.ToLower(kind) + ".already_exists", http.StatusConflict
	case codes.PermissionDenied:
		return "auth.forbidden", http.StatusForbidden
	case codes.Unauthenticated:
		return "auth.unauthenticated", http.StatusUnauthorized
	case codes.FailedPrecondition:
		return "config.invalid", http.StatusConflict
	case codes.Unimplemented:
		return "server.unimplemented", http.StatusNotImplemented
	default:
		return "server.internal", http.StatusInternalServerError
	}
}

func writeError(w http.ResponseWriter, statusCode int, code, msg string, details map[string]any) {
	contract.WriteError(w, statusCode, code, msg, details)
}

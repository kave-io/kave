package fx

import (
	"context"
	"time"

	corefx "github.com/kave-io/kave/core/fx"
	"github.com/kave-io/kave/core/pkg/money"
	commonv1 "github.com/kave-io/kave/proto/gen/kave/common/v1"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements runtimev1.FXServiceServer.
type Server struct {
	runtimev1.UnimplementedFXServiceServer
	fxService *corefx.Service
}

// New creates a new FX gRPC service server.
func New(fxService *corefx.Service) *Server {
	return &Server{
		fxService: fxService,
	}
}

// Register registers the FXService with gRPC.
func (s *Server) Register(srv *grpc.Server) {
	runtimev1.RegisterFXServiceServer(srv, s)
}

// ListCurrencies lists all supported currencies.
func (s *Server) ListCurrencies(ctx context.Context, req *runtimev1.ListCurrenciesRequest) (*runtimev1.ListCurrenciesResponse, error) {
	if s.fxService == nil {
		return nil, status.Error(codes.FailedPrecondition, "fx service unavailable")
	}

	records := s.fxService.ListCurrencies()
	currencies := make([]*runtimev1.Currency, len(records))
	for i, rec := range records {
		currencies[i] = &runtimev1.Currency{
			Code:          string(rec.Code),
			Name:          rec.Name,
			Symbol:        rec.Symbol,
			FetchedAtMs:   rec.FetchedAt,
		}
	}
	return &runtimev1.ListCurrenciesResponse{
		Currencies: currencies,
	}, nil
}

// ListRates lists exchange rates for the requested base and quote currencies.
func (s *Server) ListRates(ctx context.Context, req *runtimev1.ListRatesRequest) (*runtimev1.ListRatesResponse, error) {
	if s.fxService == nil {
		return nil, status.Error(codes.FailedPrecondition, "fx service unavailable")
	}

	base := req.GetBase()
	if base == "" {
		base = "USD"
	}
	quotes := req.GetQuotes()
	if len(quotes) == 0 {
		quotes = []string{"IRT"}
	}

	var rates []*runtimev1.RateSnapshot
	for _, quote := range quotes {
		snap, err := s.fxService.Latest(money.CurrencyCode(base), money.CurrencyCode(quote))
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "rate %s/%s: %v", base, quote, err)
		}
		rates = append(rates, protoRateSnapshot(snap))
	}

	return &runtimev1.ListRatesResponse{
		Rates: rates,
	}, nil
}

// Convert converts an amount from one currency to another.
func (s *Server) Convert(ctx context.Context, req *runtimev1.ConvertRequest) (*runtimev1.ConvertResponse, error) {
	if s.fxService == nil {
		return nil, status.Error(codes.FailedPrecondition, "fx service unavailable")
	}

	amount := req.GetAmount()
	if amount == nil {
		return nil, status.Error(codes.InvalidArgument, "amount is required")
	}
	toCurrency := req.GetToCurrency()
	if toCurrency == "" {
		return nil, status.Error(codes.InvalidArgument, "to_currency is required")
	}

	// Use caller-supplied snapshot if provided; otherwise get latest
	var snap *corefx.RateSnapshot
	if req.GetSnapshot() != nil {
		snap = protoToRateSnapshot(req.GetSnapshot())
	} else {
		fromCurrency := amount.GetCurrency()
		if fromCurrency == "" {
			return nil, status.Error(codes.InvalidArgument, "amount.currency is required")
		}
		var err error
		snap, err = s.fxService.Latest(money.CurrencyCode(fromCurrency), money.CurrencyCode(toCurrency))
		if err != nil {
			return nil, status.Errorf(codes.Unavailable, "no rate for %s/%s", fromCurrency, toCurrency)
		}
	}

	// Simple conversion: input × rate / 1e6 = output
	// For now, return a stub; a full implementation would apply the rate correctly.
	output := &commonv1.Amount{
		Decimal:  amount.GetDecimal(), // Placeholder; real impl converts
		Currency: toCurrency,
	}

	return &runtimev1.ConvertResponse{
		Input:    amount,
		Output:   output,
		Snapshot: protoRateSnapshot(snap),
	}, nil
}

// Refresh triggers an on-demand refresh of exchange rates from Frankfurter.
func (s *Server) Refresh(ctx context.Context, req *runtimev1.RefreshRequest) (*runtimev1.RefreshResponse, error) {
	if s.fxService == nil {
		return nil, status.Error(codes.FailedPrecondition, "fx service unavailable")
	}

	if err := s.fxService.Refresh(ctx); err != nil {
		return &runtimev1.RefreshResponse{
			Status:       "error",
			RefreshedAtMs: now(),
			ErrorMessage: strPtr(err.Error()),
		}, nil
	}

	// Reload cache after successful refresh
	if err := s.fxService.Load(ctx); err != nil {
		return &runtimev1.RefreshResponse{
			Status:       "error",
			RefreshedAtMs: now(),
			ErrorMessage: strPtr(err.Error()),
		}, nil
	}

	return &runtimev1.RefreshResponse{
		Status:       "ok",
		RefreshedAtMs: now(),
	}, nil
}

// Helpers

func now() int64 {
	return time.Now().UnixMilli()
}

func strPtr(s string) *string { return &s }

func protoRateSnapshot(snap *corefx.RateSnapshot) *runtimev1.RateSnapshot {
	return &runtimev1.RateSnapshot{
		Base:             snap.Base,
		Quote:            snap.Quote,
		RateMicro:        snap.RateMicro,
		CapturedAtUnixMs: snap.CapturedAtMs,
		Source:           snap.Source,
	}
}

func protoToRateSnapshot(proto *runtimev1.RateSnapshot) *corefx.RateSnapshot {
	return &corefx.RateSnapshot{
		Base:        proto.GetBase(),
		Quote:       proto.GetQuote(),
		RateMicro:   proto.GetRateMicro(),
		CapturedAtMs: proto.GetCapturedAtUnixMs(),
		Source:      proto.GetSource(),
	}
}

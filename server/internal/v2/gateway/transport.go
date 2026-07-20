package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"
)

// ErrProviderAddressDenied identifies an upstream address rejected by Kave's
// provider-egress boundary. The gateway deliberately maps the concrete error
// to its generic provider_unavailable response rather than exposing network
// topology to callers.
var ErrProviderAddressDenied = errors.New("v2 gateway: provider address denied")

// ProviderEgressPolicy is deny-by-default for non-public provider addresses.
// AllowedPrivateIPs is intentionally a list of exact IP literals rather than
// hostnames, CIDRs, or a blanket allow-private switch. It exists for a local or
// self-hosted provider whose address is known to the operator.
//
// Only loopback, RFC 1918, and IPv6 unique-local addresses are eligible for an
// exception. Link-local, multicast, unspecified, reserved, and known metadata
// addresses cannot be enabled through this policy.
type ProviderEgressPolicy struct {
	AllowedPrivateIPs []string
}

type lookupNetIPFunc func(context.Context, string, string) ([]netip.Addr, error)
type dialContextFunc func(context.Context, string, string) (net.Conn, error)

type providerDialer struct {
	lookup         lookupNetIPFunc
	dial           dialContextFunc
	allowedPrivate map[netip.Addr]struct{}
}

// providerHTTPClient keeps all HTTP egress in this transport chokepoint.
// Redirects are forbidden because a redirect target would otherwise become a
// second, route-authorized provider destination.
type providerHTTPClient struct {
	client *http.Client
}

func newProviderHTTPClient(transport http.RoundTripper) *providerHTTPClient {
	return &providerHTTPClient{client: &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("v2 gateway: provider redirects are forbidden")
		},
	}}
}

func (c *providerHTTPClient) Do(request *http.Request) (*http.Response, error) {
	return c.client.Do(request)
}

// NewProviderTransport builds the only transport used by the V2 provider
// gateway. It never honors HTTP(S)_PROXY/NO_PROXY and pins each connection to
// an IP returned and validated by that dial's DNS lookup. The request URL is
// left untouched, so net/http still sends the original Host header and TLS
// SNI/server-name verification still use the configured provider hostname.
func NewProviderTransport(policy ProviderEgressPolicy) (*http.Transport, error) {
	resolver := &net.Resolver{PreferGo: true}
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return newProviderTransport(policy, resolver.LookupNetIP, dialer.DialContext)
}

func newProviderTransport(policy ProviderEgressPolicy, lookup lookupNetIPFunc, dial dialContextFunc) (*http.Transport, error) {
	if lookup == nil || dial == nil {
		return nil, errors.New("v2 gateway: provider resolver and dialer are required")
	}
	allowed, err := parseAllowedPrivateIPs(policy.AllowedPrivateIPs)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&providerDialer{
			lookup:         lookup,
			dial:           dial,
			allowedPrivate: allowed,
		}).DialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           100,
		MaxIdleConnsPerHost:    32,
		IdleConnTimeout:        90 * time.Second,
		TLSHandshakeTimeout:    10 * time.Second,
		ExpectContinueTimeout:  1 * time.Second,
		ResponseHeaderTimeout:  2 * time.Minute,
		MaxResponseHeaderBytes: 1 << 20,
	}
	// Never install a second TLS-specific dialer: it could bypass address
	// validation. net/http performs TLS over the connection returned above.
	transport.DialTLSContext = nil
	return transport, nil
}

func parseAllowedPrivateIPs(values []string) (map[netip.Addr]struct{}, error) {
	allowed := make(map[netip.Addr]struct{}, len(values))
	for i, raw := range values {
		if raw == "" || strings.TrimSpace(raw) != raw {
			return nil, fmt.Errorf("v2 gateway: allowed private IP %d must be a clean exact IP literal", i)
		}
		address, err := netip.ParseAddr(raw)
		if err != nil || address.Zone() != "" || address.Unmap() != address || address.String() != raw {
			return nil, fmt.Errorf("v2 gateway: allowed private IP %q must be a canonical exact IP literal, not a hostname or CIDR", raw)
		}
		class := classifyProviderAddress(address)
		if class != addressPrivateOptIn {
			return nil, fmt.Errorf("v2 gateway: allowed private IP %q is not an eligible loopback or private address", raw)
		}
		if _, duplicate := allowed[address]; duplicate {
			return nil, fmt.Errorf("v2 gateway: allowed private IP %q is duplicated", raw)
		}
		allowed[address] = struct{}{}
	}
	return allowed, nil
}

func (d *providerDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("v2 gateway: invalid provider dial address: %w", err)
	}
	if host == "" || port == "" {
		return nil, errors.New("v2 gateway: invalid empty provider host or port")
	}

	candidates, err := d.resolve(ctx, network, host)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("v2 gateway: provider host %q resolved to no usable addresses", host)
	}

	// Reject a mixed public/private answer in its entirety. Merely skipping the
	// unsafe record makes behavior depend on resolver order and is needlessly
	// permissive when a provider DNS name has been poisoned or rebound.
	for _, candidate := range candidates {
		if err := d.authorize(candidate); err != nil {
			return nil, fmt.Errorf("%w: host %q resolved to %s", err, host, candidate)
		}
	}

	var dialErrors []error
	for _, candidate := range candidates {
		target := net.JoinHostPort(candidate.String(), port)
		conn, dialErr := d.dial(ctx, network, target)
		if dialErr == nil {
			return conn, nil
		}
		dialErrors = append(dialErrors, fmt.Errorf("dial validated provider address %s: %w", candidate, dialErr))
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.Join(dialErrors...)
}

func (d *providerDialer) resolve(ctx context.Context, network, host string) ([]netip.Addr, error) {
	if literal, err := netip.ParseAddr(host); err == nil {
		if literal.Zone() != "" {
			return nil, fmt.Errorf("%w: scoped IP literals are forbidden", ErrProviderAddressDenied)
		}
		literal = literal.Unmap()
		if addressMatchesNetwork(literal, network) {
			return []netip.Addr{literal}, nil
		}
		return nil, fmt.Errorf("v2 gateway: provider address %q does not match network %q", host, network)
	}

	lookupNetwork, err := ipLookupNetwork(network)
	if err != nil {
		return nil, err
	}
	addresses, err := d.lookup(ctx, lookupNetwork, host)
	if err != nil {
		return nil, fmt.Errorf("v2 gateway: resolve provider host %q: %w", host, err)
	}
	result := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		if !address.IsValid() || address.Zone() != "" {
			return nil, fmt.Errorf("%w: provider host %q resolved to an invalid or scoped address", ErrProviderAddressDenied, host)
		}
		address = address.Unmap()
		if !addressMatchesNetwork(address, network) || slices.Contains(result, address) {
			continue
		}
		result = append(result, address)
	}
	return result, nil
}

func (d *providerDialer) authorize(address netip.Addr) error {
	switch classifyProviderAddress(address) {
	case addressPublic:
		return nil
	case addressPrivateOptIn:
		if _, ok := d.allowedPrivate[address]; ok {
			return nil
		}
	}
	return ErrProviderAddressDenied
}

func ipLookupNetwork(network string) (string, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
		return strings.Replace(network, "tcp", "ip", 1), nil
	default:
		return "", fmt.Errorf("v2 gateway: unsupported provider network %q", network)
	}
}

func addressMatchesNetwork(address netip.Addr, network string) bool {
	switch network {
	case "tcp4":
		return address.Is4()
	case "tcp6":
		return address.Is6()
	default:
		return true
	}
}

type providerAddressClass uint8

const (
	addressDenied providerAddressClass = iota
	addressPrivateOptIn
	addressPublic
)

var permanentlyDeniedProviderPrefixes = mustProviderPrefixes(
	// IPv4 special-use ranges that are neither a normal public destination nor
	// an operator-eligible RFC 1918/loopback address.
	"0.0.0.0/8",
	"100.64.0.0/10",
	"169.254.0.0/16",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.31.196.0/24",
	"192.52.193.0/24",
	"192.88.99.0/24",
	"192.175.48.0/24",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	// IPv6 translation, discard, protocol-assignment, documentation, 6to4,
	// link-local, and multicast space. IPv4-mapped addresses are unmapped and
	// evaluated against the IPv4 rules before reaching this list.
	"::/96",
	"64:ff9b::/96",
	"64:ff9b:1::/48",
	"100::/64",
	"2001::/23",
	"2001:db8::/32",
	"2002::/16",
	"2620:4f:8000::/48",
	"3fff::/20",
	"5f00::/16",
	"fec0::/10",
	"fe80::/10",
	"ff00::/8",
)

var permanentlyDeniedMetadataAddresses = map[netip.Addr]struct{}{
	netip.MustParseAddr("100.100.100.200"): {},
	netip.MustParseAddr("168.63.129.16"):   {},
	netip.MustParseAddr("169.254.169.254"): {},
	netip.MustParseAddr("169.254.170.2"):   {},
	netip.MustParseAddr("192.0.0.192"):     {},
	netip.MustParseAddr("fd00:ec2::254"):   {},
	netip.MustParseAddr("fd20:ce::254"):    {},
}

func classifyProviderAddress(address netip.Addr) providerAddressClass {
	address = address.Unmap()
	if !address.IsValid() || address.Zone() != "" || address.IsUnspecified() || address.IsMulticast() || address.IsLinkLocalUnicast() {
		return addressDenied
	}
	if _, metadata := permanentlyDeniedMetadataAddresses[address]; metadata {
		return addressDenied
	}
	if address.IsLoopback() || address.IsPrivate() {
		return addressPrivateOptIn
	}
	if !address.IsGlobalUnicast() {
		return addressDenied
	}
	for _, prefix := range permanentlyDeniedProviderPrefixes {
		if prefix.Contains(address) {
			return addressDenied
		}
	}
	return addressPublic
}

func mustProviderPrefixes(values ...string) []netip.Prefix {
	prefixes := make([]netip.Prefix, len(values))
	for i, value := range values {
		prefixes[i] = netip.MustParsePrefix(value)
	}
	return prefixes
}

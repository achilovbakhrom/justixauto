package auth

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

var (
	ErrMissingPeerIdentity   = errors.New("auth: missing verified peer identity")
	ErrUnknownPeerIdentity   = errors.New("auth: unknown peer identity")
	ErrDisabledPeerIdentity  = errors.New("auth: disabled peer identity")
	ErrAmbiguousPeerIdentity = errors.New("auth: ambiguous peer identity")
	ErrSpoofedIdentityHeader = errors.New("auth: spoofed internal identity header")
)

// IdentityBinding maps one exact certificate URI SAN to a service. TLS trust,
// certificate issuance, and rotation remain deployment concerns.
type IdentityBinding struct {
	URI     string
	Service Service
	Enabled bool
}

type boundIdentity struct {
	service Service
	enabled bool
}

// MTLSResolver resolves verified client certificates using an immutable SAN
// map. It is safe for concurrent use.
type MTLSResolver struct {
	bindings map[string]boundIdentity
}

func NewMTLSResolver(bindings []IdentityBinding) (*MTLSResolver, error) {
	copied := make(map[string]boundIdentity, len(bindings))
	for index, binding := range bindings {
		uri, err := url.Parse(binding.URI)
		if err != nil || !uri.IsAbs() || uri.User != nil || uri.Fragment != "" || uri.String() != binding.URI ||
			(uri.Host == "" && uri.Opaque == "" && uri.Path == "") {
			return nil, fmt.Errorf("%w: invalid URI SAN binding at index %d", ErrInvalidConfiguration, index)
		}
		if !binding.Service.valid() {
			return nil, fmt.Errorf("%w: invalid service at index %d", ErrInvalidConfiguration, index)
		}
		if _, duplicate := copied[binding.URI]; duplicate {
			return nil, fmt.Errorf("%w: duplicate URI SAN binding at index %d", ErrInvalidConfiguration, index)
		}
		copied[binding.URI] = boundIdentity{service: binding.Service, enabled: binding.Enabled}
	}
	return &MTLSResolver{bindings: copied}, nil
}

// Resolve accepts only the leaf certificate from a verified TLS chain. Exactly
// one configured URI SAN must identify the peer.
func (r *MTLSResolver) Resolve(state *tls.ConnectionState) (Caller, error) {
	if r == nil || state == nil || !state.HandshakeComplete || len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
		return Caller{}, ErrMissingPeerIdentity
	}
	leaf := state.PeerCertificates[0]
	if leaf == nil || !verifiedLeaf(state.VerifiedChains, leaf.Raw) {
		return Caller{}, ErrMissingPeerIdentity
	}

	matches := make(map[string]boundIdentity)
	for _, uri := range leaf.URIs {
		if uri == nil {
			continue
		}
		value := uri.String()
		if binding, ok := r.bindings[value]; ok {
			matches[value] = binding
		}
	}
	if len(matches) == 0 {
		return Caller{}, ErrUnknownPeerIdentity
	}
	if len(matches) != 1 {
		return Caller{}, ErrAmbiguousPeerIdentity
	}
	for _, match := range matches {
		if !match.enabled {
			return Caller{}, ErrDisabledPeerIdentity
		}
		return Caller{service: match.service}, nil
	}
	panic("unreachable")
}

func verifiedLeaf(chains [][]*x509.Certificate, raw []byte) bool {
	for _, chain := range chains {
		if len(chain) != 0 && chain[0] != nil && bytes.Equal(chain[0].Raw, raw) {
			return true
		}
	}
	return false
}

// Authenticate resolves the mTLS caller and applies the exact grant in one
// fail-closed operation.
func Authenticate(resolver *MTLSResolver, authorizer *Authorizer, state *tls.ConnectionState, action Action, purpose Purpose) (Caller, error) {
	caller, err := resolver.Resolve(state)
	if err != nil {
		return Caller{}, err
	}
	if err := authorizer.Authorize(caller, action, purpose); err != nil {
		return Caller{}, err
	}
	return caller, nil
}

func isInternalIdentityHeader(name string) bool {
	name = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "_", "-")
	if strings.HasPrefix(name, "x-justix-internal-") || strings.HasPrefix(name, "x-internal-") {
		return true
	}
	switch name {
	case "x-actor", "x-actor-id", "x-actor-kind", "x-permission", "x-permissions":
		return true
	default:
		return false
	}
}

// SpoofedIdentityHeaders returns sorted header names that public transport must
// not trust. X-Context-Revision is deliberately public and is not included.
func SpoofedIdentityHeaders(header http.Header) []string {
	var found []string
	for name := range header {
		if isInternalIdentityHeader(name) {
			found = append(found, name)
		}
	}
	sort.Strings(found)
	return found
}

// RejectSpoofedIdentityHeaders fails closed when a public request supplies
// internal actor, permission, or service identity headers.
func RejectSpoofedIdentityHeaders(header http.Header) error {
	found := SpoofedIdentityHeaders(header)
	if len(found) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrSpoofedIdentityHeader, strings.Join(found, ", "))
}

// StripIdentityHeaders removes spoofable internal identity headers in place
// and returns the exact removed keys. It is intended for a sanitized proxy copy.
func StripIdentityHeaders(header http.Header) []string {
	found := SpoofedIdentityHeaders(header)
	for _, name := range found {
		delete(header, name)
	}
	return found
}

// SanitizedProxyHeaders returns an independent copy suitable for forwarding.
// The inbound request headers are never mutated.
func SanitizedProxyHeaders(header http.Header) http.Header {
	cloned := header.Clone()
	StripIdentityHeaders(cloned)
	return cloned
}

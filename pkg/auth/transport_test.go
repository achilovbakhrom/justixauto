package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"
)

const (
	retailSAN   = "spiffe://justix-auto/service/retail"
	commerceSAN = "spiffe://justix-auto/service/commerce"
	disabledSAN = "spiffe://justix-auto/service/legacy-retail"
)

func mustURI(t *testing.T, value string) *url.URL {
	t.Helper()
	uri, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return uri
}

func peerState(t *testing.T, sans ...string) *tls.ConnectionState {
	t.Helper()
	leaf := &x509.Certificate{Raw: []byte("verified-leaf")}
	for _, san := range sans {
		leaf.URIs = append(leaf.URIs, mustURI(t, san))
	}
	return &tls.ConnectionState{
		HandshakeComplete: true,
		PeerCertificates:  []*x509.Certificate{leaf},
		VerifiedChains:    [][]*x509.Certificate{{leaf}},
	}
}

func testResolver(t *testing.T) *MTLSResolver {
	t.Helper()
	resolver, err := NewMTLSResolver([]IdentityBinding{
		{URI: retailSAN, Service: ServiceRetail, Enabled: true},
		{URI: commerceSAN, Service: ServiceCommerce, Enabled: true},
		{URI: disabledSAN, Service: ServiceRetail, Enabled: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func TestMTLSResolverRequiresVerifiedKnownEnabledIdentity(t *testing.T) {
	resolver := testResolver(t)
	valid := peerState(t, retailSAN)
	caller, err := resolver.Resolve(valid)
	if err != nil {
		t.Fatal(err)
	}
	if caller.Service() != ServiceRetail || !caller.Valid() {
		t.Fatalf("unexpected caller: %#v", caller)
	}

	unverified := peerState(t, retailSAN)
	unverified.VerifiedChains = nil
	wrongLeaf := peerState(t, retailSAN)
	wrongLeaf.VerifiedChains = [][]*x509.Certificate{{{Raw: []byte("another-leaf")}}}
	tests := []struct {
		name  string
		state *tls.ConnectionState
		want  error
	}{
		{"nil state", nil, ErrMissingPeerIdentity},
		{"handshake incomplete", &tls.ConnectionState{}, ErrMissingPeerIdentity},
		{"unverified certificate", unverified, ErrMissingPeerIdentity},
		{"verified chain for different leaf", wrongLeaf, ErrMissingPeerIdentity},
		{"unknown SAN", peerState(t, "spiffe://justix-auto/service/unknown"), ErrUnknownPeerIdentity},
		{"no URI SAN", peerState(t), ErrUnknownPeerIdentity},
		{"disabled identity", peerState(t, disabledSAN), ErrDisabledPeerIdentity},
		{"ambiguous identity", peerState(t, retailSAN, commerceSAN), ErrAmbiguousPeerIdentity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller, err := resolver.Resolve(test.state)
			if !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got caller=%#v err=%v", test.want, caller, err)
			}
			if caller.Valid() {
				t.Fatalf("failure returned authenticated caller: %#v", caller)
			}
		})
	}
}

func TestMTLSResolverConfigurationIsExactAndImmutable(t *testing.T) {
	invalid := [][]IdentityBinding{
		{{URI: "retail", Service: ServiceRetail, Enabled: true}},
		{{URI: "spiffe:", Service: ServiceRetail, Enabled: true}},
		{{URI: "spiffe://user@justix-auto/service/retail", Service: ServiceRetail, Enabled: true}},
		{{URI: retailSAN, Service: "unknown", Enabled: true}},
		{{URI: retailSAN, Service: ServiceRetail, Enabled: true}, {URI: retailSAN, Service: ServiceCommerce, Enabled: true}},
	}
	for index, bindings := range invalid {
		if _, err := NewMTLSResolver(bindings); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("case %d: expected invalid configuration, got %v", index, err)
		}
	}

	bindings := []IdentityBinding{{URI: retailSAN, Service: ServiceRetail, Enabled: true}}
	resolver, err := NewMTLSResolver(bindings)
	if err != nil {
		t.Fatal(err)
	}
	bindings[0].Enabled = false
	bindings[0].Service = ServiceCommerce
	caller, err := resolver.Resolve(peerState(t, retailSAN))
	if err != nil || caller.Service() != ServiceRetail {
		t.Fatalf("constructor input mutation changed resolver: caller=%#v err=%v", caller, err)
	}
}

func TestAuthenticateCombinesPeerAndPurposeGrant(t *testing.T) {
	resolver := testResolver(t)
	authorizer, err := NewAuthorizer([]Grant{{
		Caller: ServiceRetail, Action: "inventory.reservation.acquire", Purpose: "retail-sale",
	}})
	if err != nil {
		t.Fatal(err)
	}
	caller, err := Authenticate(resolver, authorizer, peerState(t, retailSAN), "inventory.reservation.acquire", "retail-sale")
	if err != nil || caller.Service() != ServiceRetail {
		t.Fatalf("expected authenticated admission, caller=%#v err=%v", caller, err)
	}
	if caller, err = Authenticate(resolver, authorizer, peerState(t, retailSAN), "inventory.reservation.acquire", "wholesale-order"); !errors.Is(err, ErrUnauthorized) || caller.Valid() {
		t.Fatalf("wrong purpose must return no caller and deny, caller=%#v err=%v", caller, err)
	}
	if caller, err = Authenticate(resolver, authorizer, peerState(t, commerceSAN), "inventory.reservation.acquire", "retail-sale"); !errors.Is(err, ErrUnauthorized) || caller.Valid() {
		t.Fatalf("wrong caller must return no caller and deny, caller=%#v err=%v", caller, err)
	}
}

func TestResolverWithVerifiedMutualTLSHandshake(t *testing.T) {
	caCertificate, caKey, caPool := testCertificateAuthority(t)
	serverCertificate := issueCertificate(t, caCertificate, caKey, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-server"},
		DNSNames:     []string{"test-server"},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	clientCertificate := issueCertificate(t, caCertificate, caKey, &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "ignored-common-name"},
		URIs:         []*url.URL{mustURI(t, retailSAN)},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})

	resolver := testResolver(t)
	serverConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCertificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	clientConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      caPool,
		Certificates: []tls.Certificate{clientCertificate},
		ServerName:   "test-server",
	}
	serverSide, clientSide := net.Pipe()
	server := tls.Server(serverSide, serverConfig)
	client := tls.Client(clientSide, clientConfig)
	defer server.Close()
	defer client.Close()

	serverResult := make(chan error, 1)
	go func() { serverResult <- server.Handshake() }()
	if err := client.Handshake(); err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	if err := <-serverResult; err != nil {
		t.Fatalf("server handshake: %v", err)
	}
	state := server.ConnectionState()
	caller, err := resolver.Resolve(&state)
	if err != nil {
		t.Fatalf("resolver rejected verified handshake: %v", err)
	}
	if caller.Service() != ServiceRetail {
		t.Fatalf("handshake resolved %q, want %q", caller.Service(), ServiceRetail)
	}
}

func testCertificateAuthority(t *testing.T) (*x509.Certificate, ed25519.PrivateKey, *x509.CertPool) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "JustixAuto synthetic test CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, privateKey.Public(), privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(certificate)
	return certificate, privateKey, pool
}

func issueCertificate(t *testing.T, authority *x509.Certificate, authorityKey ed25519.PrivateKey, template *x509.Certificate) tls.Certificate {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template.NotBefore = now.Add(-time.Minute)
	template.NotAfter = now.Add(time.Hour)
	template.KeyUsage = x509.KeyUsageDigitalSignature
	der, err := x509.CreateCertificate(rand.Reader, template, authority, privateKey.Public(), authorityKey)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return certificate
}

func TestResolverConcurrentReads(t *testing.T) {
	resolver := testResolver(t)
	state := peerState(t, retailSAN)
	const readers = 32
	var wait sync.WaitGroup
	wait.Add(readers)
	for range readers {
		go func() {
			defer wait.Done()
			for range 100 {
				caller, err := resolver.Resolve(state)
				if err != nil || caller.Service() != ServiceRetail {
					t.Errorf("concurrent resolution failed: caller=%#v err=%v", caller, err)
					return
				}
			}
		}()
	}
	wait.Wait()
}

func TestPublicIdentityHeaderRejectionAndStripping(t *testing.T) {
	headers := http.Header{
		"Authorization":            {"Bearer public-session-token"},
		"X-Context-Revision":       {"7"},
		"x-actor-id":               {"spoofed-user"},
		"X-Permissions":            {"admin:*"},
		"X-Justix-Internal-Caller": {"retail"},
		"x-internal-purpose":       {"wholesale-order"},
		"X_Actor_Kind":             {"user"},
	}
	wantSpoofed := []string{"X-Justix-Internal-Caller", "X-Permissions", "X_Actor_Kind", "x-actor-id", "x-internal-purpose"}
	if got := SpoofedIdentityHeaders(headers); !reflect.DeepEqual(got, wantSpoofed) {
		t.Fatalf("spoof list mismatch: got %q want %q", got, wantSpoofed)
	}
	if err := RejectSpoofedIdentityHeaders(headers); !errors.Is(err, ErrSpoofedIdentityHeader) {
		t.Fatalf("expected spoof rejection, got %v", err)
	}

	proxy := SanitizedProxyHeaders(headers)
	if got := SpoofedIdentityHeaders(proxy); len(got) != 0 {
		t.Fatalf("proxy retained spoofable headers: %q", got)
	}
	if proxy.Get("Authorization") == "" || proxy.Get("X-Context-Revision") != "7" {
		t.Fatalf("proxy removed legitimate public transport headers: %#v", proxy)
	}
	if got := SpoofedIdentityHeaders(headers); !reflect.DeepEqual(got, wantSpoofed) {
		t.Fatalf("sanitizing proxy copy mutated inbound headers: %q", got)
	}
	proxy.Set("Authorization", "changed")
	if headers.Get("Authorization") == "changed" {
		t.Fatal("proxy header values alias inbound values")
	}
	if removed := StripIdentityHeaders(headers); !reflect.DeepEqual(removed, wantSpoofed) {
		t.Fatalf("removed keys mismatch: %q", removed)
	}
	if err := RejectSpoofedIdentityHeaders(headers); err != nil {
		t.Fatalf("sanitized public headers rejected: %v", err)
	}
}

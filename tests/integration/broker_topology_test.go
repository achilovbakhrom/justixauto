package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	vhost               = "/justixauto"
	integrationExchange = "justix.integration.v1"
	quarantineExchange  = "justix.quarantine.v1"
)

var owners = []string{"identity", "inventory", "commerce", "retail", "financing", "insurance", "documents"}

type definitions struct {
	RabbitVersion string `json:"rabbit_version"`
	Users         []struct {
		Name         string   `json:"name"`
		PasswordHash string   `json:"password_hash"`
		Algorithm    string   `json:"hashing_algorithm"`
		Tags         []string `json:"tags"`
	} `json:"users"`
	Vhosts []struct {
		Name string `json:"name"`
	} `json:"vhosts"`
	Permissions []permission         `json:"permissions"`
	Topic       []topicPermission    `json:"topic_permissions"`
	Queues      []queueDefinition    `json:"queues"`
	Exchanges   []exchangeDefinition `json:"exchanges"`
	Bindings    []bindingDefinition  `json:"bindings"`
}

type permission struct {
	User      string `json:"user"`
	Vhost     string `json:"vhost"`
	Configure string `json:"configure"`
	Write     string `json:"write"`
	Read      string `json:"read"`
}

type topicPermission struct {
	User     string `json:"user"`
	Vhost    string `json:"vhost"`
	Exchange string `json:"exchange"`
	Write    string `json:"write"`
	Read     string `json:"read"`
}

type queueDefinition struct {
	Name       string         `json:"name"`
	Vhost      string         `json:"vhost"`
	Durable    bool           `json:"durable"`
	AutoDelete bool           `json:"auto_delete"`
	Arguments  map[string]any `json:"arguments"`
}

type exchangeDefinition struct {
	Name       string `json:"name"`
	Vhost      string `json:"vhost"`
	Type       string `json:"type"`
	Durable    bool   `json:"durable"`
	AutoDelete bool   `json:"auto_delete"`
	Internal   bool   `json:"internal"`
}

type bindingDefinition struct {
	Source          string `json:"source"`
	Vhost           string `json:"vhost"`
	Destination     string `json:"destination"`
	DestinationType string `json:"destination_type"`
	RoutingKey      string `json:"routing_key"`
}

// routeContracts is migration-authority input, not source/object authorization.
// Production activation needs reviewed metadata plus fenced channels, audited
// topology and receiver/source admission. This compiler is deliberately a test
// oracle in the assigned leaf; it is not an unreviewed runtime provisioner.
type routeContracts struct {
	Format      int                `json:"format_version"`
	RouteFormat int                `json:"route_format"`
	Integration []integrationRoute `json:"activated_integration"`
	Quarantine  []quarantineRoute  `json:"activated_quarantine"`
}
type integrationRoute struct {
	Source        string `json:"source"`
	Target        string `json:"target"`
	EventType     string `json:"event_type"`
	SchemaVersion uint32 `json:"schema_version"`
	ReviewRef     string `json:"review_ref"`
}
type quarantineRoute struct {
	Owner       string `json:"owner"`
	Source      string `json:"source"`
	FailureType string `json:"failure_type"`
	ReviewRef   string `json:"review_ref"`
}

// Reject ambiguous JSON keys before decoding: encoding/json otherwise accepts
// duplicate and case-insensitive field names. Canonical field spelling matters
// for a reviewed permission artifact.
func decodeRouteContracts(raw []byte) (routeContracts, error) {
	var c routeContracts
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		if delim, ok := tok.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					key, err := d.Token()
					if err != nil {
						return err
					}
					k, ok := key.(string)
					if !ok || seen[k] || strings.ToLower(k) != k {
						return fmt.Errorf("noncanonical or duplicate key %v", key)
					}
					seen[k] = true
					if err := walk(); err != nil {
						return err
					}
				}
			case '[':
				for d.More() {
					if err := walk(); err != nil {
						return err
					}
				}
			default:
				return fmt.Errorf("unexpected delimiter")
			}
			_, err = d.Token()
			return err
		}
		return nil
	}
	if err := walk(); err != nil {
		return c, err
	}
	if _, err := d.Token(); err != io.EOF {
		return c, fmt.Errorf("trailing JSON")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	return c, nil
}

func compileRoutePermissions(c routeContracts) ([]permission, []topicPermission, error) {
	if c.Format != 1 || c.RouteFormat != 1 || c.Integration == nil || c.Quarantine == nil {
		return nil, nil, fmt.Errorf("explicit version-1 activated arrays required")
	}
	validOwner := set(owners)
	eventPattern := regexp.MustCompile(`^([a-z][a-z0-9-]*)(?:\.[a-z][a-z0-9-]*)+\.v([1-9][0-9]*)$`)
	failurePattern := regexp.MustCompile(`^[a-z][a-z0-9-]*(?:\.[a-z][a-z0-9-]*)*\.v[1-9][0-9]*$`)
	review := func(s string) bool {
		return s != "" && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\r\n\t\x00")
	}
	writes := map[string][]string{}
	seen := map[string]bool{}
	for _, r := range c.Integration {
		m := eventPattern.FindStringSubmatch(r.EventType)
		key := r.Source + "." + r.Target + "." + r.EventType
		if !validOwner[r.Source] || !validOwner[r.Target] || len(m) != 3 || m[1] != r.Source || !review(r.ReviewRef) || len(key) > 255 {
			return nil, nil, fmt.Errorf("invalid reviewed integration route")
		}
		version, err := strconv.ParseUint(m[2], 10, 32)
		if err != nil || version != uint64(r.SchemaVersion) || r.SchemaVersion == 0 || seen["i:"+key] {
			return nil, nil, fmt.Errorf("noncanonical schema or duplicate route")
		}
		seen["i:"+key] = true
		writes["relay_"+r.Source+"|"+integrationExchange] = append(writes["relay_"+r.Source+"|"+integrationExchange], key)
	}
	for _, r := range c.Quarantine {
		key := r.Owner + "." + r.Source + "." + r.FailureType
		if !validOwner[r.Owner] || !validOwner[r.Source] || !failurePattern.MatchString(r.FailureType) || !review(r.ReviewRef) || len(key) > 255 || seen["q:"+key] {
			return nil, nil, fmt.Errorf("invalid reviewed quarantine route")
		}
		seen["q:"+key] = true
		writes["intake_"+r.Owner+"|"+quarantineExchange] = append(writes["intake_"+r.Owner+"|"+quarantineExchange], key)
	}
	var ps []permission
	var ts []topicPermission
	for _, owner := range owners {
		for _, role := range []string{"svc", "relay", "intake"} {
			user := role + "_" + owner
			p := permission{user, vhost, "a^", "a^", "a^"}
			if role == "relay" {
				p.Write = exactRoutes([]string{integrationExchange})
			}
			if role == "intake" {
				p.Read = exactRoutes([]string{"justix." + owner + ".inbox.v1"})
				if len(writes[user+"|"+quarantineExchange]) > 0 {
					p.Write = exactRoutes([]string{quarantineExchange})
				}
			}
			ps = append(ps, p)
			for _, exchange := range []string{integrationExchange, quarantineExchange} {
				// Topic READ checks binding keys, not delivery routing keys.
				// Services never bind; the migration authority owns all bindings.
				ts = append(ts, topicPermission{user, vhost, exchange, exactRoutes(writes[user+"|"+exchange]), "a^"})
			}
		}
	}
	return ps, ts, nil
}

func exactRoutes(routes []string) string {
	if len(routes) == 0 {
		return "a^"
	}
	routes = append([]string(nil), routes...)
	sort.Strings(routes)
	for i := range routes {
		routes[i] = regexp.QuoteMeta(routes[i])
	}
	if len(routes) == 1 {
		return "^" + routes[0] + `\z`
	}
	// PCRE's $ can match before a final newline; \z is an absolute end anchor
	// in both RabbitMQ's Erlang regex engine and Go's regexp oracle.
	return "^(?:" + strings.Join(routes, "|") + `)\z`
}

func TestBrokerDefinitionsAreDurableAndRestricted(t *testing.T) {
	d := readDefinitions(t)
	if d.RabbitVersion != "4.3.5" || len(d.Vhosts) != 1 || d.Vhosts[0].Name != vhost {
		t.Fatal("unexpected pinned broker/vhost")
	}
	raw, err := os.ReadFile(filepath.Join(brokerDirectory(t), "route-contracts.json"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := decodeRouteContracts(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic routes exist only in test functions, never in activated metadata.
	if len(c.Integration) != 0 || len(c.Quarantine) != 0 {
		t.Fatal("business activation requires a separately reviewed change")
	}
	ps, ts, err := compileRoutePermissions(c)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ps, d.Permissions) || !reflect.DeepEqual(ts, d.Topic) {
		t.Fatal("definitions drift from exact activated contract permissions")
	}
	want := map[string]bool{}
	seen := map[string]bool{}
	for _, o := range owners {
		for _, role := range []string{"svc", "relay", "intake"} {
			want[role+"_"+o] = true
		}
	}
	for _, u := range d.Users {
		if seen[u.Name] || !want[u.Name] || u.PasswordHash != "" || u.Algorithm != "rabbit_password_hashing_sha256" || len(u.Tags) != 0 {
			t.Fatalf("unexpected/credentialed/privileged user %s", u.Name)
		}
		seen[u.Name] = true
	}
	assertExactSet(t, "split users including explicitly revoked legacy users", seen, want)
	assertTopologyObjects(t, d)
}

func TestBrokerRouteCompiler(t *testing.T) {
	empty := func() routeContracts { return routeContracts{1, 1, []integrationRoute{}, []quarantineRoute{}} }
	for _, source := range owners {
		for _, target := range owners {
			c := empty()
			typ := source + ".fixture.changed.v1"
			route := source + "." + target + "." + typ
			c.Integration = []integrationRoute{{source, target, typ, 1, "synthetic-review-only"}}
			_, ts, err := compileRoutePermissions(c)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range ts {
				allowed := []string{}
				denied := []string{"", route + ".extra", route + "\n", strings.ReplaceAll(route, ".", "x"), source + "." + target + ".fixture.changed.v1", source + "." + target + "." + source + ".fixture.changed.v2"}
				if p.User == "relay_"+source && p.Exchange == integrationExchange {
					allowed = append(allowed, route)
				} else {
					denied = append(denied, route)
				}
				assertRegexMatchesOnly(t, p.Write, allowed, denied)
				assertRegexMatchesOnly(t, p.Read, nil, []string{"*." + target + ".#", route})
			}
		}
	}
	t.Run("reject malformed and ambiguous contracts", func(t *testing.T) {
		base := integrationRoute{"inventory", "retail", "inventory.fixture.changed.v1", 1, "synthetic-review-only"}
		cases := []integrationRoute{}
		for _, source := range []string{"Inventory", "unknown", "inventory ", "inventory|retail"} {
			r := base
			r.Source = source
			cases = append(cases, r)
		}
		for _, target := range []string{"*", "#", "retail ", "unknown"} {
			r := base
			r.Target = target
			cases = append(cases, r)
		}
		for _, typ := range []string{"fixture.changed.v1", "retail.fixture.changed.v1", "inventory.fixture.changed.v01", "inventory.fixture.changed.v0", "inventory.fixture.changed.v4294967296", "inventory..changed.v1", "inventory.fixture.*.v1", "inventory.fixture.changed.v1\n", "inventory.é.v1", "inventory." + strings.Repeat("a", 250) + ".v1"} {
			r := base
			r.EventType = typ
			cases = append(cases, r)
		}
		for _, version := range []uint32{0, 2} {
			r := base
			r.SchemaVersion = version
			cases = append(cases, r)
		}
		for _, ref := range []string{"", " review", "review\n", "review\x00"} {
			r := base
			r.ReviewRef = ref
			cases = append(cases, r)
		}
		for i, r := range cases {
			c := empty()
			c.Integration = []integrationRoute{r}
			if _, _, err := compileRoutePermissions(c); err == nil {
				t.Fatalf("accepted invalid route %d: %#v", i, r)
			}
		}
		c := empty()
		c.Integration = []integrationRoute{base, base}
		if _, _, err := compileRoutePermissions(c); err == nil {
			t.Fatal("duplicate accepted")
		}
		for _, raw := range []string{`{"format_version":1,"format_version":1}`, `{"FORMAT_VERSION":1}`, `{"unknown":1}`, `{} {}`, `null`, `{"format_version":1,"route_format":1,"activated_integration":null,"activated_quarantine":[]}`} {
			c, err := decodeRouteContracts([]byte(raw))
			if err == nil {
				_, _, err = compileRoutePermissions(c)
			}
			if err == nil {
				t.Fatalf("accepted %s", raw)
			}
		}
	})
	t.Run("byte boundary repeated owner and deterministic complete set", func(t *testing.T) {
		c := empty()
		prefix := "inventory.retail.inventory."
		typ := "inventory." + strings.Repeat("a", 255-len(prefix)-3) + ".v1"
		c.Integration = []integrationRoute{{"inventory", "retail", typ, 1, "synthetic"}, {"inventory", "retail", "inventory.inventory.changed.v1", 1, "synthetic"}}
		_, first, err := compileRoutePermissions(c)
		if err != nil {
			t.Fatal(err)
		}
		c.Integration[0], c.Integration[1] = c.Integration[1], c.Integration[0]
		_, second, err := compileRoutePermissions(c)
		if err != nil || !reflect.DeepEqual(first, second) {
			t.Fatal("order changed grants")
		}
		c.Integration[1].EventType = strings.Replace(typ, ".v1", "a.v1", 1)
		if _, _, err := compileRoutePermissions(c); err == nil {
			t.Fatal("256-byte route accepted")
		}
	})
	t.Run("quarantine requires an exact owner source failure review", func(t *testing.T) {
		base := quarantineRoute{"retail", "inventory", "malformed.v1", "synthetic"}
		for _, owner := range owners {
			for _, source := range owners {
				c := empty()
				c.Quarantine = []quarantineRoute{{owner, source, "malformed.v1", "synthetic"}}
				ps, ts, err := compileRoutePermissions(c)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range ps {
					if p.User == "intake_"+owner {
						assertRegexMatchesOnly(t, p.Write, []string{quarantineExchange}, []string{integrationExchange, "", "amq.default"})
					}
				}
				for _, p := range ts {
					route := owner + "." + source + ".malformed.v1"
					if p.User == "intake_"+owner && p.Exchange == quarantineExchange {
						assertRegexMatchesOnly(t, p.Write, []string{route}, []string{route + "\n", route + ".extra", owner + "." + source + ".other.v1"})
					} else {
						assertRegexMatchesOnly(t, p.Write, nil, []string{route})
					}
				}
			}
		}
		for _, bad := range []quarantineRoute{{"unknown", base.Source, base.FailureType, base.ReviewRef}, {base.Owner, "*", base.FailureType, base.ReviewRef}, {base.Owner, base.Source, "*.v1", base.ReviewRef}, {base.Owner, base.Source, "malformed.v01", base.ReviewRef}, {base.Owner, base.Source, "malformed.v1\n", base.ReviewRef}, {base.Owner, base.Source, strings.Repeat("a", 250) + ".v1", base.ReviewRef}, {base.Owner, base.Source, base.FailureType, ""}} {
			c := empty()
			c.Quarantine = []quarantineRoute{bad}
			if _, _, err := compileRoutePermissions(c); err == nil {
				t.Fatalf("accepted invalid quarantine %#v", bad)
			}
		}
		c := empty()
		c.Quarantine = []quarantineRoute{base, base}
		if _, _, err := compileRoutePermissions(c); err == nil {
			t.Fatal("duplicate quarantine route accepted")
		}
	})
}

type brokerFixture struct {
	container, amqpAddress, managementAddress, password string
	client                                              *http.Client
}

// Every live run owns its broker and generated credentials. An arbitrary DSN is
// intentionally unsupported: this test changes permissions and queue bindings.
func newBrokerFixture(t *testing.T) *brokerFixture {
	t.Helper()
	if os.Getenv("JUSTIXAUTO_TEST_BROKER_TOPOLOGY") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_BROKER_TOPOLOGY=1 for a disposable pinned broker")
	}
	f := &brokerFixture{client: &http.Client{Timeout: 10 * time.Second}}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	f.container = fmt.Sprintf("justixauto-t924-%x", nonce[:6])
	f.password = fmt.Sprintf("synthetic-%x", nonce[:])
	d := readDefinitions(t)
	// Model only the former broker capabilities. No production payload/history
	// is read; retained sentinel bytes below belong to this disposable fixture.
	for i, p := range d.Permissions {
		if strings.HasPrefix(p.User, "svc_") {
			d.Permissions[i].Write = `^justix\.(integration|quarantine)\.v1$`
			o := strings.TrimPrefix(p.User, "svc_")
			d.Permissions[i].Read = `^justix\.` + o + `\.(inbox|quarantine)\.v1$`
		}
	}
	for i, p := range d.Topic {
		if strings.HasPrefix(p.User, "svc_") {
			o := strings.TrimPrefix(p.User, "svc_")
			d.Topic[i].Write = "^" + o + `\..+$`
			d.Topic[i].Read = ".*"
		}
	}
	raw := f.importBytes(t, d)
	dir := t.TempDir()
	file := filepath.Join(dir, "definitions.json")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// -v removes only anonymous volumes attached to this owned fixture;
		// there are no named volumes or external data directories.
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", "-v", f.container).CombinedOutput(); err != nil {
			t.Errorf("remove own broker: %v %s", err, out)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"run", "-d", "--name", f.container, "-p", "127.0.0.1::5672", "-p", "127.0.0.1::15672", "--mount", "type=bind,src=" + file + ",dst=/etc/rabbitmq/definitions.json,readonly", "--mount", "type=bind,src=" + filepath.Join(brokerDirectory(t), "rabbitmq.conf") + ",dst=/etc/rabbitmq/rabbitmq.conf,readonly", "docker.io/library/rabbitmq@sha256:b3b8b7f95f5382a19f9ea33540e604f30aad081d37ad9aba72255135765373a1"}
	if out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("start own pinned broker: %v %s", err, out)
	}
	port := func(which string) string {
		out, err := exec.CommandContext(ctx, "docker", "port", f.container, which).Output()
		if err != nil {
			t.Fatal(err)
		}
		address := strings.TrimSpace(string(out))
		if !strings.HasPrefix(address, "127.0.0.1:") || strings.Contains(address, "\n") {
			t.Fatalf("non-loopback mapping %q", address)
		}
		return address
	}
	f.amqpAddress = port("5672/tcp")
	f.managementAddress = "http://" + port("15672/tcp")
	deadline := time.Now().Add(90 * time.Second)
	for {
		if _, err := f.api("GET", "/api/overview", nil); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("own broker did not become ready")
		}
		time.Sleep(250 * time.Millisecond)
	}
	conn := f.connect(t, "t924_admin")
	if fmt.Sprint(conn.Properties["version"]) != "4.3.5" {
		t.Fatal("unexpected broker version")
	}
	declareTopologyPassive(t, conn)
	_ = conn.Close()
	return f
}

func (f *brokerFixture) importBytes(t *testing.T, d definitions) []byte {
	t.Helper()
	// Hashes and bootstrap administrator exist only in this temporary fixture.
	salt := []byte{0x92, 0x40, 0x17, 0x01}
	digest := sha256.Sum256(append(append([]byte(nil), salt...), []byte(f.password)...))
	hash := base64.StdEncoding.EncodeToString(append(salt, digest[:]...))
	for i := range d.Users {
		d.Users[i].PasswordHash = hash
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err = json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	object["users"] = append(object["users"].([]any), map[string]any{"name": "t924_admin", "password_hash": hash, "hashing_algorithm": "rabbit_password_hashing_sha256", "tags": []string{"administrator"}})
	object["permissions"] = append(object["permissions"].([]any), map[string]any{"user": "t924_admin", "vhost": vhost, "configure": ".*", "write": ".*", "read": ".*"})
	raw, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func (f *brokerFixture) api(method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, f.managementAddress+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("t924_admin", f.password)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("broker API %s %s status %d: %s", method, path, resp.StatusCode, raw)
	}
	return raw, nil
}
func (f *brokerFixture) install(t *testing.T, d definitions) {
	t.Helper()
	if _, err := f.api("POST", "/api/definitions", f.importBytes(t, d)); err != nil {
		t.Fatal(err)
	}
}
func (f *brokerFixture) connect(t *testing.T, user string) *amqp.Connection {
	t.Helper()
	u := url.URL{Scheme: "amqp", Host: f.amqpAddress, User: url.UserPassword(user, f.password), Path: "/" + vhost, RawPath: "/%2Fjustixauto"}
	conn, err := amqp.DialConfig(u.String(), amqp.Config{Heartbeat: 5 * time.Second, Dial: amqp.DefaultDial(5 * time.Second)})
	if err != nil {
		t.Fatalf("connect synthetic %s: %v", user, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
func (f *brokerFixture) channel(t *testing.T, user string) *amqp.Channel {
	t.Helper()
	ch, err := f.connect(t, user).Channel()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	return ch
}
func (f *brokerFixture) denied(t *testing.T, user string, action func(*amqp.Channel) error) {
	t.Helper()
	ch := f.channel(t, user)
	closed := ch.NotifyClose(make(chan *amqp.Error, 1))
	err := action(ch)
	if e, ok := err.(*amqp.Error); ok {
		if e.Code != 403 {
			t.Fatalf("denial code=%d want ACCESS_REFUSED: %v", e.Code, e)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected transport error instead of permission denial: %v", err)
	}
	select {
	case e := <-closed:
		if e == nil || e.Code != 403 {
			t.Fatalf("expected ACCESS_REFUSED, got %v", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("forbidden action was not rejected")
	}
}
func brokerPublish(ch *amqp.Channel, exchange, key, id, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return ch.PublishWithContext(ctx, exchange, key, true, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: id, Body: []byte(body)})
}
func (f *brokerFixture) confirmed(t *testing.T, user, exchange, key, id, body string, returned bool) {
	t.Helper()
	ch := f.channel(t, user)
	if err := ch.Confirm(false); err != nil {
		t.Fatal(err)
	}
	returns := ch.NotifyReturn(make(chan amqp.Return, 1))
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	if err := brokerPublish(ch, exchange, key, id, body); err != nil {
		t.Fatal(err)
	}
	select {
	case c := <-confirms:
		if !c.Ack || c.DeliveryTag != 1 {
			t.Fatalf("unexpected confirm %#v", c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("missing confirm")
	}
	// amqp091-go dispatches basic.return before the corresponding confirm.
	select {
	case r := <-returns:
		if !returned || r.ReplyCode != 312 || r.ReplyText != "NO_ROUTE" || r.MessageId != id || string(r.Body) != body || r.RoutingKey != key || r.Exchange != exchange || r.DeliveryMode != amqp.Persistent {
			t.Fatalf("unexpected return %#v", r)
		}
	default:
		if returned {
			t.Fatal("confirmed unroutable message was not returned")
		}
	}
}
func (f *brokerFixture) get(t *testing.T, user, queue string) *amqp.Delivery {
	t.Helper()
	ch := f.channel(t, user)
	deadline := time.Now().Add(5 * time.Second)
	for {
		d, ok, err := ch.Get(queue, false)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			if err = d.Ack(false); err != nil {
				t.Fatal(err)
			}
			return &d
		}
		if time.Now().After(deadline) {
			t.Fatal("expected retained/routed message missing")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPinnedBrokerSplitRolesAndRetainedCutover(t *testing.T) {
	f := newBrokerFixture(t)
	const route = "inventory.retail.inventory.fixture.changed.v1"
	const body = `{"synthetic":"retained-original-bytes"}`
	t.Run("legacy backlog retained across explicit permission replacement", func(t *testing.T) {
		f.confirmed(t, "svc_inventory", integrationExchange, route, "legacy-retained", body, false)
		f.confirmed(t, "svc_retail", quarantineExchange, "retail.inventory.malformed.v1", "legacy-quarantine", body, false)
		// Close every old connection before replacing permissions. Definition
		// import alone is not a fence for cached ACLs or in-flight publications.
		connections, err := f.api("GET", "/api/connections", nil)
		if err != nil {
			t.Fatal(err)
		}
		var list []struct {
			Name string `json:"name"`
		}
		if err = json.Unmarshal(connections, &list); err != nil {
			t.Fatal(err)
		}
		for _, c := range list {
			if _, err = f.api("DELETE", "/api/connections/"+url.PathEscape(c.Name), nil); err != nil {
				t.Fatal(err)
			}
		}
		f.install(t, readDefinitions(t))
		d := f.get(t, "intake_retail", "justix.retail.inbox.v1")
		if d.MessageId != "legacy-retained" || string(d.Body) != body || d.RoutingKey != route || d.DeliveryMode != amqp.Persistent {
			t.Fatal("retained bytes/identity/properties changed")
		}
		quarantined := f.get(t, "t924_admin", "justix.retail.quarantine.v1")
		if quarantined.MessageId != "legacy-quarantine" || string(quarantined.Body) != body || quarantined.DeliveryMode != amqp.Persistent {
			t.Fatal("retained quarantine content changed")
		}
		for _, o := range owners {
			f.denied(t, "svc_"+o, func(ch *amqp.Channel) error {
				return brokerPublish(ch, integrationExchange, o+".retail."+o+".fixture.changed.v1", "denied", body)
			})
			f.denied(t, "svc_"+o, func(ch *amqp.Channel) error { _, _, err := ch.Get("justix."+o+".inbox.v1", false); return err })
			f.denied(t, "relay_"+o, func(ch *amqp.Channel) error {
				return brokerPublish(ch, integrationExchange, o+".retail."+o+".fixture.changed.v1", "empty-set", body)
			})
			f.denied(t, "relay_"+o, func(ch *amqp.Channel) error { return brokerPublish(ch, integrationExchange, "", "empty-key", body) })
		}
	})
	c := routeContracts{1, 1, []integrationRoute{{"inventory", "retail", "inventory.fixture.changed.v1", 1, "synthetic-test-only-review"}, {"inventory", "inventory", "inventory.inventory.changed.v1", 1, "synthetic-test-only-review"}}, []quarantineRoute{{"retail", "inventory", "malformed.v1", "synthetic-test-only-review"}}}
	d := readDefinitions(t)
	var err error
	d.Permissions, d.Topic, err = compileRoutePermissions(c)
	if err != nil {
		t.Fatal(err)
	}
	f.install(t, d)
	t.Run("exact synthetic routes persistent confirms and owner-local intake", func(t *testing.T) {
		f.confirmed(t, "relay_inventory", integrationExchange, route, "split-valid", body, false)
		received := f.get(t, "intake_retail", "justix.retail.inbox.v1")
		if received.MessageId != "split-valid" || string(received.Body) != body || received.RoutingKey != route || received.DeliveryMode != amqp.Persistent {
			t.Fatal("routed message changed")
		}
		f.confirmed(t, "relay_inventory", integrationExchange, "inventory.inventory.inventory.inventory.changed.v1", "self-valid", body, false)
		if f.get(t, "intake_inventory", "justix.inventory.inbox.v1").MessageId != "self-valid" {
			t.Fatal("nested owner route failed")
		}
		f.confirmed(t, "intake_retail", quarantineExchange, "retail.inventory.malformed.v1", "quarantine-valid", body, false)
		if f.get(t, "t924_admin", "justix.retail.quarantine.v1").MessageId != "quarantine-valid" {
			t.Fatal("quarantine route failed")
		}
	})
	t.Run("forbidden capabilities close channels with access-refused", func(t *testing.T) {
		for _, key := range []string{"", route + "\n", route + "\r\n", route + ".extra", strings.ReplaceAll(route, ".", "x"), "inventory.retail.fixture.changed.v1", "inventory.retail.inventory.fixture.changed.v2", "inventory.documents.inventory.fixture.changed.v1", "retail.retail.inventory.fixture.changed.v1", "inventory.retail.retail.fixture.changed.v1"} {
			f.denied(t, "relay_inventory", func(ch *amqp.Channel) error { return brokerPublish(ch, integrationExchange, key, "denied", body) })
		}
		for _, o := range owners {
			for _, role := range []string{"relay", "intake", "svc"} {
				user := role + "_" + o
				f.denied(t, user, func(ch *amqp.Channel) error {
					return brokerPublish(ch, integrationExchange, "", "empty-integration-key-denied", body)
				})
				f.denied(t, user, func(ch *amqp.Channel) error { return brokerPublish(ch, "", "", "empty-default-key-denied", body) })
				f.denied(t, user, func(ch *amqp.Channel) error {
					return brokerPublish(ch, "", "justix."+o+".inbox.v1", "default-denied", body)
				})
				f.denied(t, user, func(ch *amqp.Channel) error {
					_, _, err := ch.Get("justix."+otherOwner(o)+".inbox.v1", false)
					return err
				})
				f.denied(t, user, func(ch *amqp.Channel) error {
					_, err := ch.QueueDeclare("justix."+o+".inbox.v1", true, false, false, false, amqp.Table{"x-queue-type": "quorum"})
					return err
				})
				f.denied(t, user, func(ch *amqp.Channel) error {
					return ch.ExchangeDeclare(integrationExchange, "topic", true, false, false, false, nil)
				})
				f.denied(t, user, func(ch *amqp.Channel) error {
					return ch.QueueBind("justix."+o+".inbox.v1", "*."+o+".#", integrationExchange, false, nil)
				})
			}
			f.denied(t, "relay_"+o, func(ch *amqp.Channel) error { _, _, err := ch.Get("justix."+o+".inbox.v1", false); return err })
			f.denied(t, "relay_"+o, func(ch *amqp.Channel) error {
				_, err := ch.Consume("justix."+o+".inbox.v1", "forbidden-relay", false, false, false, false, nil)
				return err
			})
			f.denied(t, "intake_"+o, func(ch *amqp.Channel) error {
				return brokerPublish(ch, integrationExchange, route, "intake-denied", body)
			})
			f.denied(t, "intake_"+o, func(ch *amqp.Channel) error { _, _, err := ch.Get("justix."+o+".quarantine.v1", false); return err })
		}
		for _, key := range []string{"retail.inventory.other.v1", "inventory.inventory.malformed.v1", "retail.identity.malformed.v1", ""} {
			f.denied(t, "intake_retail", func(ch *amqp.Channel) error {
				return brokerPublish(ch, quarantineExchange, key, "quarantine-denied", body)
			})
		}
		f.denied(t, "relay_inventory", func(ch *amqp.Channel) error {
			return brokerPublish(ch, quarantineExchange, "retail.inventory.malformed.v1", "relay-denied", body)
		})
	})
	t.Run("approved publication with missing binding returns despite positive confirm", func(t *testing.T) {
		// Only the fixture migration authority may remove and restore this
		// synthetic binding; never delete a queue or its retained messages.
		ch := f.channel(t, "t924_admin")
		if err := ch.QueueUnbind("justix.retail.inbox.v1", "*.retail.#", integrationExchange, nil); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := ch.QueueBind("justix.retail.inbox.v1", "*.retail.#", integrationExchange, false, nil); err != nil {
				t.Error(err)
			}
		}()
		f.confirmed(t, "relay_inventory", integrationExchange, route, "mandatory-return", body, true)
	})
	t.Run("topology audit rejects added cross-owner binding", func(t *testing.T) {
		// Topic READ is evaluated against literal binding keys. A dedicated
		// temporary provisioning probe gets resources needed for binding, then
		// an exact escaped binding-key grant, to prove that semantic distinction.
		probe := permission{"svc_documents", vhost, "a^", `^justix\.documents\.inbox\.v1$`, `^justix\.integration\.v1$`}
		raw, _ := json.Marshal(probe)
		if _, err := f.api("PUT", "/api/permissions/%2Fjustixauto/svc_documents", raw); err != nil {
			t.Fatal(err)
		}
		tp := topicPermission{"svc_documents", vhost, integrationExchange, "a^", `^\*\.documents\.#$`}
		raw, _ = json.Marshal(tp)
		if _, err := f.api("PUT", "/api/topic-permissions/%2Fjustixauto/svc_documents", raw); err != nil {
			t.Fatal(err)
		}
		ch := f.channel(t, "svc_documents")
		if err := ch.QueueBind("justix.documents.inbox.v1", "*.documents.#", integrationExchange, false, nil); err != nil {
			t.Fatal(err)
		}
		f.denied(t, "svc_documents", func(ch *amqp.Channel) error {
			return ch.QueueBind("justix.documents.inbox.v1", "*.retail.#", integrationExchange, false, nil)
		})
		_ = ch.Close()
		f.install(t, d)
		admin := f.channel(t, "t924_admin")
		if err := admin.QueueBind("justix.documents.inbox.v1", "*.retail.#", integrationExchange, false, nil); err != nil {
			t.Fatal(err)
		}
		if err := f.auditBindings(d); err == nil {
			t.Fatal("cross-owner copy not detected")
		}
		if err := admin.QueueUnbind("justix.documents.inbox.v1", "*.retail.#", integrationExchange, nil); err != nil {
			t.Fatal(err)
		}
		if err := f.auditBindings(d); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("every owner destination and maximum byte route uses full schema", func(t *testing.T) {
		all := routeContracts{1, 1, []integrationRoute{}, []quarantineRoute{}}
		for _, source := range owners {
			for _, target := range owners {
				all.Integration = append(all.Integration, integrationRoute{source, target, source + ".fixture.changed.v1", 1, "synthetic-all-pairs-only"})
			}
		}
		longType := "inventory." + strings.Repeat("a", 255-len("inventory.retail.inventory.")-3) + ".v1"
		all.Integration = append(all.Integration, integrationRoute{"inventory", "retail", longType, 1, "synthetic-byte-bound-only"})
		matrix := readDefinitions(t)
		var err error
		matrix.Permissions, matrix.Topic, err = compileRoutePermissions(all)
		if err != nil {
			t.Fatal(err)
		}
		f.install(t, matrix)
		for i, r := range all.Integration {
			key := r.Source + "." + r.Target + "." + r.EventType
			id := fmt.Sprintf("matrix-%d", i)
			f.confirmed(t, "relay_"+r.Source, integrationExchange, key, id, body, false)
			// Exercise basic.consume as well as basic.get; this ACK only removes
			// synthetic broker evidence, and is not a business-effect ACK claim.
			ch := f.channel(t, "intake_"+r.Target)
			deliveries, err := ch.Consume("justix."+r.Target+".inbox.v1", id, false, false, false, false, nil)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case msg := <-deliveries:
				if msg.MessageId != id || string(msg.Body) != body || msg.RoutingKey != key || msg.DeliveryMode != amqp.Persistent {
					t.Fatalf("wrong matrix delivery %d", i)
				}
				if err := msg.Ack(false); err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("missing matrix delivery %d", i)
			}
			if err := ch.Cancel(id, false); err != nil {
				t.Fatal(err)
			}
			_ = ch.Close()
		}
		f.install(t, readDefinitions(t))
		f.denied(t, "relay_inventory", func(ch *amqp.Channel) error {
			return brokerPublish(ch, integrationExchange, route, "revoked-synthetic-route", body)
		})
		if err := f.auditBindings(d); err != nil {
			t.Fatal(err)
		}
	})
}

func (f *brokerFixture) auditBindings(d definitions) error {
	raw, err := f.api("GET", "/api/bindings/%2Fjustixauto", nil)
	if err != nil {
		return err
	}
	var actual []bindingDefinition
	if err = json.Unmarshal(raw, &actual); err != nil {
		return err
	}
	key := func(b bindingDefinition) string {
		return b.Source + "|" + b.DestinationType + "|" + b.Destination + "|" + b.RoutingKey
	}
	want := map[string]bool{}
	for _, b := range d.Bindings {
		want[key(b)] = true
	}
	for _, b := range actual {
		if b.Source == "" {
			continue
		}
		if b.Vhost != vhost || !want[key(b)] {
			return fmt.Errorf("unexpected binding %s", key(b))
		}
		delete(want, key(b))
	}
	if len(want) != 0 {
		return fmt.Errorf("missing bindings %v", want)
	}
	return nil
}

func brokerDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve source")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "infra", "local", "rabbitmq")
}

func readDefinitions(t *testing.T) definitions {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "infra", "local", "rabbitmq", "definitions.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read definitions: %v", err)
	}
	var d definitions
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("parse definitions: %v", err)
	}
	return d
}

func assertTopologyObjects(t *testing.T, d definitions) {
	t.Helper()
	if len(d.Exchanges) != 2 || len(d.Queues) != 2*len(owners) {
		t.Fatal("duplicate or unexpected topology object count")
	}
	wantExchanges := set([]string{integrationExchange, quarantineExchange})
	seenExchanges := make(map[string]bool, len(d.Exchanges))
	for _, exchange := range d.Exchanges {
		if exchange.Vhost != vhost || exchange.Type != "topic" || !exchange.Durable || exchange.AutoDelete || exchange.Internal {
			t.Fatalf("exchange is not a durable public topic in the restricted vhost: %#v", exchange)
		}
		seenExchanges[exchange.Name] = true
	}
	assertExactSet(t, "exchanges", seenExchanges, wantExchanges)

	wantQueues := make(map[string]bool, 2*len(owners))
	for _, owner := range owners {
		wantQueues["justix."+owner+".inbox.v1"] = true
		wantQueues["justix."+owner+".quarantine.v1"] = true
	}
	seenQueues := make(map[string]bool, len(d.Queues))
	for _, queue := range d.Queues {
		if queue.Vhost != vhost || !queue.Durable || queue.AutoDelete || queue.Arguments["x-queue-type"] != "quorum" || len(queue.Arguments) != 1 {
			t.Fatalf("queue is not a durable quorum queue in the restricted vhost: %#v", queue)
		}
		seenQueues[queue.Name] = true
	}
	assertExactSet(t, "queues", seenQueues, wantQueues)

	if len(d.Bindings) != 2*len(owners) {
		t.Fatalf("bindings = %d, want %d", len(d.Bindings), 2*len(owners))
	}
	seenBindings := make(map[string]bool, len(d.Bindings))
	for _, binding := range d.Bindings {
		if binding.Vhost != vhost || binding.DestinationType != "queue" {
			t.Fatalf("unexpected binding scope/type: %#v", binding)
		}
		seenBindings[binding.Source+"|"+binding.Destination+"|"+binding.RoutingKey] = true
	}
	for _, owner := range owners {
		for _, key := range []string{
			integrationExchange + "|justix." + owner + ".inbox.v1|*." + owner + ".#",
			quarantineExchange + "|justix." + owner + ".quarantine.v1|" + owner + ".#",
		} {
			if !seenBindings[key] {
				t.Errorf("missing binding %q", key)
			}
		}
	}
}

func declareTopologyPassive(t *testing.T, conn *amqp.Connection) {
	t.Helper()
	for _, exchange := range []string{integrationExchange, quarantineExchange} {
		ch, err := conn.Channel()
		if err != nil {
			t.Fatalf("open channel for exchange %s: %v", exchange, err)
		}
		err = ch.ExchangeDeclarePassive(exchange, "topic", true, false, false, false, nil)
		_ = ch.Close()
		if err != nil {
			t.Fatalf("passive declare exchange %s: %v", exchange, err)
		}
	}
	for _, owner := range owners {
		for _, kind := range []string{"inbox", "quarantine"} {
			name := "justix." + owner + "." + kind + ".v1"
			ch, err := conn.Channel()
			if err != nil {
				t.Fatalf("open channel for queue %s: %v", name, err)
			}
			_, err = ch.QueueDeclarePassive(name, true, false, false, false, amqp.Table{"x-queue-type": "quorum"})
			_ = ch.Close()
			if err != nil {
				t.Fatalf("passive declare queue %s: %v", name, err)
			}
		}
	}
}

func assertRegexMatchesOnly(t *testing.T, expression string, allowed, denied []string) {
	t.Helper()
	re, err := regexp.Compile(expression)
	if err != nil {
		t.Fatalf("invalid permission regex %q: %v", expression, err)
	}
	for _, value := range allowed {
		if !re.MatchString(value) {
			t.Errorf("permission %q unexpectedly denies %q", expression, value)
		}
	}
	for _, value := range denied {
		if re.MatchString(value) {
			t.Errorf("permission %q unexpectedly allows %q", expression, value)
		}
	}
}

func set(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func assertExactSet(t *testing.T, label string, got, want map[string]bool) {
	t.Helper()
	if len(got) == len(want) {
		equal := true
		for value := range want {
			equal = equal && got[value]
		}
		if equal {
			return
		}
	}
	keys := func(values map[string]bool) []string {
		result := make([]string, 0, len(values))
		for value := range values {
			result = append(result, value)
		}
		sort.Strings(result)
		return result
	}
	t.Fatalf("%s = %v, want %v", label, keys(got), keys(want))
}

func otherOwner(owner string) string {
	for _, candidate := range owners {
		if candidate != owner {
			return candidate
		}
	}
	panic("owners list has no alternate")
}

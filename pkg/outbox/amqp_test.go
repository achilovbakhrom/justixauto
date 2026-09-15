package outbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"justixauto/pkg/events"
)

func testPublication() Publication {
	p := Publication{eventID: uuid.NewString(), exchange: IntegrationExchange, routingKey: "inventory.retail.inventory.fixture.changed.v1", attempt: uuid.NewString(), body: []byte(`{"ref":"synthetic"}`), committed: true}
	p.hash = sha256.Sum256(p.body)
	p.until = time.Now().Add(time.Minute)
	return p
}

func TestAMQPReturnConfirmCorrelation(t *testing.T) {
	for _, kind := range []string{"ack", "return then ack", "nack", "wrong sequence", "wrong return identity", "wrong return bytes", "closed confirms", "closed returns", "closed channel", "timeout"} {
		t.Run(kind, func(t *testing.T) {
			p := testPublication()
			returns := make(chan amqp.Return, 1)
			confirms := make(chan amqp.Confirmation, 1)
			closes := make(chan *amqp.Error, 1)
			want := error(nil)
			switch kind {
			case "ack":
				confirms <- amqp.Confirmation{DeliveryTag: 1, Ack: true}
			case "return then ack":
				returns <- amqp.Return{MessageId: p.eventID, Exchange: p.exchange, RoutingKey: p.routingKey, Body: p.Bytes()}
				confirms <- amqp.Confirmation{DeliveryTag: 1, Ack: true}
				want = ErrPublishReturned
			case "nack":
				confirms <- amqp.Confirmation{DeliveryTag: 1, Ack: false}
				want = ErrPublishNack
			case "wrong sequence":
				confirms <- amqp.Confirmation{DeliveryTag: 2, Ack: true}
				want = ErrPublishCorrelation
			case "wrong return identity":
				returns <- amqp.Return{MessageId: uuid.NewString(), Exchange: p.exchange, RoutingKey: p.routingKey, Body: p.Bytes()}
				confirms <- amqp.Confirmation{DeliveryTag: 1, Ack: true}
				want = ErrPublishCorrelation
			case "wrong return bytes":
				returns <- amqp.Return{MessageId: p.eventID, Exchange: p.exchange, RoutingKey: p.routingKey, Body: []byte("changed")}
				confirms <- amqp.Confirmation{DeliveryTag: 1, Ack: true}
				want = ErrPublishCorrelation
			case "closed confirms":
				close(confirms)
				want = ErrPublishUnavailable
			case "closed returns":
				close(returns)
				want = ErrPublishUnavailable
			case "closed channel":
				close(closes)
				want = ErrPublishUnavailable
			case "timeout":
				want = ErrPublishUnavailable
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			c, err := awaitRouted(ctx, p, 1, returns, confirms, closes)
			if want == nil {
				if err != nil || !c.matches(p) {
					t.Fatal("routed confirm", err)
				}
			} else if !errors.Is(err, want) || c.matches(p) {
				t.Fatal("false success", err, want)
			}
		})
	}
	// Both channels are readable: select may choose either. No schedule can turn
	// a return plus ACK into success, even when ACK is selected first.
	for range 1000 {
		p := testPublication()
		r := make(chan amqp.Return, 1)
		c := make(chan amqp.Confirmation, 1)
		r <- amqp.Return{MessageId: p.eventID, Exchange: p.exchange, RoutingKey: p.routingKey, Body: p.Bytes()}
		c <- amqp.Confirmation{DeliveryTag: 1, Ack: true}
		receipt, err := awaitRouted(context.Background(), p, 1, r, c, make(chan *amqp.Error))
		if !errors.Is(err, ErrPublishReturned) || receipt.routed {
			t.Fatal("return/ACK race", err)
		}
	}
}

func TestAMQPPublisherRejectsBeforeDialAndScrubsDialErrors(t *testing.T) {
	called := false
	publisher, err := NewAMQPPublisher(func(context.Context) (*amqp.Connection, error) {
		called = true
		return nil, errors.New("synthetic-url-with-password")
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(context.Background(), Publication{}); !errors.Is(err, ErrInvalidRecord) || called {
		t.Fatal("unsealed publication", err)
	}
	expired := testPublication()
	expired.until = time.Now().Add(-time.Second)
	if _, err := publisher.Publish(context.Background(), expired); !errors.Is(err, ErrRelayLease) || called {
		t.Fatal("expired publication dialed", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := publisher.Publish(ctx, testPublication()); !errors.Is(err, context.Canceled) || called {
		t.Fatal("cancelled dial", err)
	}
	_, err = publisher.Publish(context.Background(), testPublication())
	if !errors.Is(err, ErrPublishUnavailable) || strings.Contains(err.Error(), "password") {
		t.Fatal("dial result", err)
	}
}

type relayBroker struct{ name, address, management, password string }

func newRelayBroker(t *testing.T) *relayBroker {
	t.Helper()
	b := &relayBroker{name: "justixauto-t012-rmq-" + uuid.NewString(), password: "synthetic-" + uuid.NewString()}
	raw, err := os.ReadFile("../../infra/local/rabbitmq/definitions.json")
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	salt := []byte{12, 0, 1, 2}
	h := sha256.Sum256(append(append([]byte{}, salt...), []byte(b.password)...))
	hash := base64.StdEncoding.EncodeToString(append(salt, h[:]...))
	for _, value := range d["users"].([]any) {
		u := value.(map[string]any)
		if u["name"] == "relay_inventory" {
			u["password_hash"] = hash
		}
	}
	d["users"] = append(d["users"].([]any), map[string]any{"name": "t012_admin", "password_hash": hash, "hashing_algorithm": "rabbit_password_hashing_sha256", "tags": []string{"administrator"}})
	d["permissions"] = append(d["permissions"].([]any), map[string]any{"user": "t012_admin", "vhost": "/justixauto", "configure": ".*", "write": ".*", "read": ".*"})
	// Explicit synthetic grants only. Checked-in activated business arrays and
	// definitions stay empty/unchanged. No caller gets wildcard owner routes.
	for _, value := range d["topic_permissions"].([]any) {
		p := value.(map[string]any)
		if p["user"] == "relay_inventory" && p["exchange"] == IntegrationExchange {
			p["write"] = `^(?:inventory\.retail\.inventory\.fixture\.changed\.v1|inventory\.documents\.inventory\.fixture\.changed\.v1)\z`
		}
	}
	dir := t.TempDir()
	raw, err = json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "definitions.json")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	config, err := filepath.Abs("../../infra/local/rabbitmq/rabbitmq.conf")
	if err != nil {
		t.Fatal(err)
	}
	relayDocker(t, "run", "-d", "--name", b.name, "-p", "127.0.0.1::5672", "-p", "127.0.0.1::15672", "--mount", "type=bind,src="+file+",dst=/etc/rabbitmq/definitions.json,readonly", "--mount", "type=bind,src="+config+",dst=/etc/rabbitmq/rabbitmq.conf,readonly", "docker.io/library/rabbitmq@sha256:b3b8b7f95f5382a19f9ea33540e604f30aad081d37ad9aba72255135765373a1")
	t.Cleanup(func() { relayDocker(t, "rm", "-f", "-v", b.name) })
	b.address = relayDocker(t, "port", b.name, "5672/tcp")
	b.management = "http://" + relayDocker(t, "port", b.name, "15672/tcp")
	if !strings.HasPrefix(b.address, "127.0.0.1:") || strings.Contains(b.address, "\n") || !strings.HasPrefix(b.management, "http://127.0.0.1:") || strings.Contains(b.management, "\n") {
		t.Fatal("non-loopback broker")
	}
	b.ready(t)
	return b
}

func (b *relayBroker) api(method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, b.management+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("t012_admin", b.password)
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 3 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("fixture API status %d", res.StatusCode)
	}
	return raw, nil
}
func (b *relayBroker) ready(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		raw, err := b.api("GET", "/api/overview", nil)
		if err == nil {
			var v struct {
				Version string `json:"rabbitmq_version"`
			}
			if json.Unmarshal(raw, &v) != nil || v.Version != "4.3.5" {
				t.Fatal("broker version")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("owned broker readiness", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (b *relayBroker) dial(address, user string) AMQPDialer {
	return func(ctx context.Context) (*amqp.Connection, error) {
		u := url.URL{Scheme: "amqp", Host: address, User: url.UserPassword(user, b.password), Path: "//justixauto", RawPath: "/%2Fjustixauto"}
		return amqp.DialConfig(u.String(), amqp.Config{Heartbeat: time.Second, Dial: func(network, address string) (net.Conn, error) {
			conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			deadline, ok := ctx.Deadline()
			if !ok {
				deadline = time.Now().Add(5 * time.Second)
			}
			if err := conn.SetDeadline(deadline); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return conn, nil
		}})
	}
}
func (b *relayBroker) publisher(t *testing.T) *AMQPPublisher {
	t.Helper()
	p, err := NewAMQPPublisher(b.dial(b.address, "relay_inventory"), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func (b *relayBroker) adminChannel(t *testing.T) (*amqp.Connection, *amqp.Channel) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := b.dial(b.address, "t012_admin")(ctx)
	if err != nil {
		t.Fatal("fixture admin connection", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.CloseDeadline(time.Now())
		t.Fatal(err)
	}
	return conn, ch
}
func (b *relayBroker) get(t *testing.T, target string) amqp.Delivery {
	t.Helper()
	conn, ch := b.adminChannel(t)
	defer conn.CloseDeadline(time.Now())
	deadline := time.Now().Add(3 * time.Second)
	for {
		d, ok, err := ch.Get("justix."+target+".inbox.v1", true)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatal("missing fixture delivery", target)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func (b *relayBroker) bind(t *testing.T, target string, bind bool) {
	t.Helper()
	conn, ch := b.adminChannel(t)
	defer conn.CloseDeadline(time.Now())
	var err error
	if bind {
		err = ch.QueueBind("justix."+target+".inbox.v1", "*."+target+".#", IntegrationExchange, false, nil)
	} else {
		err = ch.QueueUnbind("justix."+target+".inbox.v1", "*."+target+".#", IntegrationExchange, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
}

// Frame-level fault injection drops an actual broker basic.ack after the broker
// persisted the message, then breaks the connection. It never fabricates an ACK.
func relayLostConfirmProxy(t *testing.T, upstream string) (string, *atomic.Int64) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dropped := &atomic.Int64{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		client, err := listener.Accept()
		if err != nil {
			return
		}
		defer client.Close()
		server, err := net.DialTimeout("tcp", upstream, 3*time.Second)
		if err != nil {
			return
		}
		defer server.Close()
		_ = client.SetDeadline(time.Now().Add(10 * time.Second))
		_ = server.SetDeadline(time.Now().Add(10 * time.Second))
		copied := make(chan struct{})
		go func() { defer close(copied); _, _ = io.Copy(server, client); _ = server.Close() }()
		defer func() { _ = client.Close(); _ = server.Close(); <-copied }()
		for {
			header := make([]byte, 7)
			if _, err := io.ReadFull(server, header); err != nil {
				return
			}
			size := binary.BigEndian.Uint32(header[3:])
			if size > 2<<20 {
				return
			}
			body := make([]byte, int(size)+1)
			if _, err := io.ReadFull(server, body); err != nil {
				return
			}
			if header[0] == 1 && len(body) >= 5 && binary.BigEndian.Uint16(body[:2]) == 60 && binary.BigEndian.Uint16(body[2:4]) == 80 {
				dropped.Add(1)
				return
			}
			if _, err := client.Write(append(header, body...)); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(12 * time.Second):
			t.Error("proxy cleanup timed out")
		}
	})
	return listener.Addr().String(), dropped
}

func TestRelayBrokerPostgres(t *testing.T) {
	if os.Getenv("JUSTIXAUTO_TEST_OUTBOX_RELAY_BROKER") != "1" {
		t.Skip("set JUSTIXAUTO_TEST_OUTBOX_RELAY_BROKER=1 for owned pinned PostgreSQL and RabbitMQ")
	}
	b := newRelayBroker(t)
	f := newRelayPG(t, RelayCustody, true)
	s := relayStore(t, f, 10*time.Second)
	p := b.publisher(t)
	ctx := context.Background()
	isolate := func() {
		relayMustSQL(t, f, "UPDATE eventstore.outbox_deliveries SET next_attempt_at=clock_timestamp()+interval '1 day' WHERE sent_at IS NULL")
	}
	assertDelivery := func(t *testing.T, d amqp.Delivery, l RelayLease) {
		t.Helper()
		if d.MessageId != l.publication.eventID || !bytes.Equal(d.Body, l.publication.body) || d.DeliveryMode != amqp.Persistent || d.RoutingKey != l.publication.routingKey || d.ContentType != "application/json" {
			t.Fatal("broker changed original publication identity/bytes/persistence")
		}
	}
	t.Run("mandatory return plus positive confirm is unsent per recipient", func(t *testing.T) {
		isolate()
		b.bind(t, "documents", false)
		f.seed(t, events.OwnerDocuments, events.OwnerRetail)
		l := relayClaim(t, s)
		if l.destination != "documents" {
			t.Fatal("fixture claim order")
		}
		c, err := p.Publish(ctx, l.Publication())
		if !errors.Is(err, ErrPublishReturned) || c.routed {
			t.Fatal("unroutable accepted", err)
		}
		relayState(t, s, l, DeliveryLeased)
		retail := relayClaim(t, s)
		c, err = p.Publish(ctx, retail.Publication())
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MarkSent(ctx, retail, c); err != nil {
			t.Fatal(err)
		}
		assertDelivery(t, b.get(t, "retail"), retail)
		relayState(t, s, retail, DeliverySent)
		relayState(t, s, l, DeliveryLeased)
		b.bind(t, "documents", true)
		if err := s.Retry(ctx, l); err != nil {
			t.Fatal(err)
		}
		relayMustSQL(t, f, "UPDATE eventstore.outbox_deliveries SET next_attempt_at=clock_timestamp() WHERE event_id='"+l.publication.eventID+"' AND destination='documents'")
		retry := relayClaim(t, s)
		c, err = p.Publish(ctx, retry.Publication())
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MarkSent(ctx, retry, c); err != nil {
			t.Fatal(err)
		}
		assertDelivery(t, b.get(t, "documents"), retry)
	})
	t.Run("lost actual confirm restart and same-byte retry", func(t *testing.T) {
		isolate()
		f.seed(t, events.OwnerRetail)
		proxy, dropped := relayLostConfirmProxy(t, b.address)
		lossy, _ := NewAMQPPublisher(b.dial(proxy, "relay_inventory"), 3*time.Second)
		r, _ := NewRelay(s, lossy)
		l, err := r.Once(ctx)
		if !errors.Is(err, ErrPublishUnavailable) || dropped.Load() != 1 {
			t.Fatal("lost confirm injection", err, dropped.Load())
		}
		relayState(t, s, l, DeliveryPending)
		relayDocker(t, "restart", b.name)
		// Docker Desktop may allocate fresh ephemeral host ports on restart.
		b.address = relayDocker(t, "port", b.name, "5672/tcp")
		b.management = "http://" + relayDocker(t, "port", b.name, "15672/tcp")
		if !strings.HasPrefix(b.address, "127.0.0.1:") || strings.Contains(b.address, "\n") || !strings.HasPrefix(b.management, "http://127.0.0.1:") || strings.Contains(b.management, "\n") {
			t.Fatal("non-loopback restarted fixture")
		}
		b.ready(t)
		p = b.publisher(t)
		assertDelivery(t, b.get(t, "retail"), l)
		r, _ = NewRelay(s, p)
		retry, err := r.Once(ctx)
		if err != nil {
			t.Fatal("reconnect retry", err)
		}
		if retry.publication.eventID != l.publication.eventID || !bytes.Equal(retry.publication.body, l.publication.body) {
			t.Fatal("retry changed bytes")
		}
		relayState(t, s, retry, DeliverySent)
		assertDelivery(t, b.get(t, "retail"), retry)
	})
	t.Run("permission close cannot mark sent and no foreign activation", func(t *testing.T) {
		isolate()
		f.seed(t, events.OwnerInsurance)
		r, _ := NewRelay(s, p)
		l, err := r.Once(ctx)
		if !errors.Is(err, ErrPublishUnavailable) {
			t.Fatal("foreign route accepted", err)
		}
		relayState(t, s, l, DeliveryPending)
	})
	t.Run("real quorum overflow NACK is not sent", func(t *testing.T) {
		isolate()
		if _, err := b.api("PUT", "/api/policies/%2Fjustixauto/t012-overflow", []byte(`{"pattern":"^justix\\.retail\\.inbox\\.v1$","definition":{"max-length":1,"overflow":"reject-publish"},"priority":100,"apply-to":"queues"}`)); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := b.api("DELETE", "/api/policies/%2Fjustixauto/t012-overflow", nil); err != nil {
				t.Error(err)
			}
		}()
		// Quorum reject-publish permits a small overshoot; max-length is not an
		// exact count guarantee. Test the actual NACK rather than assuming which
		// first publication crosses that asynchronous notification boundary.
		// https://www.rabbitmq.com/docs/quorum-queues#length-limit
		confirmed := []RelayLease{}
		nacked := false
		for range 16 {
			f.seed(t, events.OwnerRetail)
			l := relayClaim(t, s)
			c, err := p.Publish(ctx, l.Publication())
			if errors.Is(err, ErrPublishNack) {
				if c.routed {
					t.Fatal("NACK receipt")
				}
				relayState(t, s, l, DeliveryLeased)
				nacked = true
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := s.MarkSent(ctx, l, c); err != nil {
				t.Fatal(err)
			}
			confirmed = append(confirmed, l)
		}
		for _, l := range confirmed {
			assertDelivery(t, b.get(t, "retail"), l)
		}
		if !nacked {
			t.Fatal("fixture did not reach quorum rejection")
		}
	})
	t.Run("hold committed while real publication in flight fences completion", func(t *testing.T) {
		isolate()
		f.seed(t, events.OwnerRetail)
		pub := relayPublisherFunc(func(ctx context.Context, pub Publication) (RoutedConfirmation, error) {
			c, err := p.Publish(ctx, pub)
			if err != nil {
				return c, err
			}
			relayMustSQL(t, f, "UPDATE eventstore.outbox_deliveries SET hold_ref='synthetic-inflight-hold' WHERE event_id='"+pub.eventID+"'")
			return c, nil
		})
		r, _ := NewRelay(s, pub)
		l, err := r.Once(ctx)
		if !errors.Is(err, ErrRelayLease) {
			t.Fatal("held in-flight completed", err)
		}
		relayState(t, s, l, DeliveryHeld)
		assertDelivery(t, b.get(t, "retail"), l)
	})
}

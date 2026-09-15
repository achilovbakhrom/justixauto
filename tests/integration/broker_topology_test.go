package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
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

func TestBrokerDefinitionsAreDurableAndRestricted(t *testing.T) {
	d := readDefinitions(t)
	if d.RabbitVersion != "4.3.5" {
		t.Fatalf("rabbit_version = %q, want pinned 4.3.5", d.RabbitVersion)
	}
	if len(d.Vhosts) != 1 || d.Vhosts[0].Name != vhost {
		t.Fatalf("vhosts = %#v, want only %q", d.Vhosts, vhost)
	}

	wantOwners := set(owners)
	seenUsers := make(map[string]bool, len(owners))
	for _, user := range d.Users {
		if user.Name == "guest" {
			t.Fatal("guest account must not be imported")
		}
		owner := strings.TrimPrefix(user.Name, "svc_")
		if _, ok := wantOwners[owner]; !ok {
			t.Fatalf("unexpected imported user %q", user.Name)
		}
		if user.PasswordHash != "" {
			t.Fatalf("%s embeds a credential; service principals must be disabled until secret injection", user.Name)
		}
		if len(user.Tags) != 0 {
			t.Fatalf("%s has privileged tags %v", user.Name, user.Tags)
		}
		seenUsers[owner] = true
	}
	assertExactSet(t, "service users", seenUsers, wantOwners)

	if len(d.Permissions) != len(owners) {
		t.Fatalf("permissions = %d, want %d", len(d.Permissions), len(owners))
	}
	for _, p := range d.Permissions {
		owner := strings.TrimPrefix(p.User, "svc_")
		if _, ok := wantOwners[owner]; !ok || p.Vhost != vhost {
			t.Fatalf("unexpected permission: %#v", p)
		}
		if p.Configure != "^$" {
			t.Fatalf("%s can configure topology with %q", p.User, p.Configure)
		}
		assertRegexMatchesOnly(t, p.Write, []string{integrationExchange, quarantineExchange}, []string{"", "amq.topic", "justix.identity.inbox.v1"})
		assertRegexMatchesOnly(t, p.Read,
			[]string{"justix." + owner + ".inbox.v1", "justix." + owner + ".quarantine.v1"},
			[]string{"justix." + otherOwner(owner) + ".inbox.v1", integrationExchange})
	}

	if len(d.Topic) != 2*len(owners) {
		t.Fatalf("topic permissions = %d, want %d", len(d.Topic), 2*len(owners))
	}
	for _, p := range d.Topic {
		owner := strings.TrimPrefix(p.User, "svc_")
		if _, ok := wantOwners[owner]; !ok || p.Vhost != vhost {
			t.Fatalf("unexpected topic permission: %#v", p)
		}
		switch p.Exchange {
		case integrationExchange:
			assertRegexMatchesOnly(t, p.Write,
				[]string{owner + "." + otherOwner(owner) + ".event.v1"},
				[]string{otherOwner(owner) + "." + owner + ".event.v1", owner + ".event.v1"})
			assertRegexMatchesOnly(t, p.Read,
				[]string{otherOwner(owner) + "." + owner + ".event.v1"},
				[]string{owner + "." + otherOwner(owner) + ".event.v1"})
		case quarantineExchange:
			assertRegexMatchesOnly(t, p.Write,
				[]string{owner + ".source.invalid.v1"},
				[]string{otherOwner(owner) + ".source.invalid.v1"})
		default:
			t.Fatalf("%s has topic grant on unexpected exchange %q", p.User, p.Exchange)
		}
	}

	assertTopologyObjects(t, d)
}

func TestPinnedBrokerReturnsMandatoryUnroutableMessage(t *testing.T) {
	url := os.Getenv("JUSTIX_RABBITMQ_URL")
	if url == "" {
		t.Skip("set JUSTIX_RABBITMQ_URL to run against the isolated pinned broker")
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatalf("dial isolated broker: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if got := fmt.Sprint(conn.Properties["version"]); got != "4.3.5" {
		t.Fatalf("broker version = %q, want 4.3.5", got)
	}

	declareTopologyPassive(t, conn)
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open publisher channel: %v", err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	if err := ch.Confirm(false); err != nil {
		t.Fatalf("enable publisher confirms: %v", err)
	}
	returns := ch.NotifyReturn(make(chan amqp.Return, 1))
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const body = `{"eventId":"00000000-0000-0000-0000-000000000006","eventType":"probe.unroutable.v1"}`
	err = ch.PublishWithContext(ctx, integrationExchange, "identity.no-such-owner.probe.unroutable.v1", true, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    "00000000-0000-0000-0000-000000000006",
		Body:         []byte(body),
	})
	if err != nil {
		t.Fatalf("publish mandatory probe: %v", err)
	}

	select {
	case returned := <-returns:
		if returned.ReplyCode != 312 || returned.ReplyText != "NO_ROUTE" || string(returned.Body) != body {
			t.Fatalf("unexpected basic.return: code=%d text=%q body=%q", returned.ReplyCode, returned.ReplyText, returned.Body)
		}
	case <-ctx.Done():
		t.Fatal("mandatory unroutable message was not returned")
	}
	select {
	case confirmation := <-confirms:
		if !confirmation.Ack {
			t.Fatal("broker negatively acknowledged unroutable probe")
		}
	case <-ctx.Done():
		t.Fatal("publisher confirmation did not arrive")
	}
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
		if queue.Vhost != vhost || !queue.Durable || queue.AutoDelete || queue.Arguments["x-queue-type"] != "quorum" {
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

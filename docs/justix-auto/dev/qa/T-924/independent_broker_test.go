package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// QA overlay: the task's source stays byte-identical to the reviewed commit.
func TestT924IndependentBrokerAdversarial(t *testing.T) {
	f := newBrokerFixture(t)
	t.Logf("owned fixture: %s", f.container)
	d := readDefinitions(t)
	f.install(t, d)
	f.install(t, d)
	const route = "inventory.retail.inventory.qa.changed.v1"
	const queue = "justix.retail.inbox.v1"
	t.Run("repeated empty import denies every role and odd key", func(t *testing.T) {
		for _, o := range owners {
			for _, role := range []string{"svc", "relay", "intake"} {
				for _, exchange := range []string{integrationExchange, quarantineExchange} {
					for _, key := range []string{"", "\n", "a", "a^", "\x00", route} {
						f.denied(t, role+"_"+o, func(ch *amqp.Channel) error {
							return brokerPublish(ch, exchange, key, "denied", "synthetic")
						})
					}
				}
			}
		}
	})
	c := routeContracts{1, 1, []integrationRoute{
		{"inventory", "retail", "inventory.qa.changed.v1", 1, "QA-synthetic-only"},
		{"inventory", "retail", "inventory.qa.changed.v2", 2, "QA-synthetic-only"},
	}, []quarantineRoute{{"retail", "inventory", "qa.malformed.v1", "QA-synthetic-only"}}}
	var err error
	d.Permissions, d.Topic, err = compileRoutePermissions(c)
	if err != nil { t.Fatal(err) }
	f.install(t, d)
	t.Run("resource names and quarantine absolute endings", func(t *testing.T) {
		admin := f.channel(t, "t924_admin")
		if err := admin.ExchangeDeclare(integrationExchange+"\n", "topic", true, false, false, false, nil); err != nil { t.Fatal(err) }
		if _, err := admin.QueueDeclare(queue+"\n", true, false, false, false, amqp.Table{"x-queue-type":"quorum"}); err != nil { t.Fatal(err) }
		f.denied(t, "relay_inventory", func(ch *amqp.Channel) error { return brokerPublish(ch, integrationExchange+"\n", route, "bad-resource", "synthetic") })
		f.denied(t, "intake_retail", func(ch *amqp.Channel) error { _, _, err := ch.Get(queue+"\n", false); return err })
		for _, suffix := range []string{"\n", "\r\n", "\x00", ".extra", " "} {
			f.denied(t, "intake_retail", func(ch *amqp.Channel) error { return brokerPublish(ch, quarantineExchange, "retail.inventory.qa.malformed.v1"+suffix, "bad-quarantine", "synthetic") })
			f.denied(t, "relay_inventory", func(ch *amqp.Channel) error { return brokerPublish(ch, integrationExchange, route+suffix, "bad-route", "synthetic") })
		}
		f.confirmed(t, "relay_inventory", integrationExchange, strings.TrimSuffix(route, "v1")+"v2", "version-two", "synthetic", false)
		if got := f.get(t, "intake_retail", queue); got.MessageId != "version-two" { t.Fatal("explicit second schema route failed") }
		// Do not individually delete whitespace aliases: broker resource
		// canonicalization can resolve those deletion names to retained objects.
		// The entire disposable fixture and its volume are removed by cleanup.
		declareTopologyPassive(t, f.connect(t,"t924_admin"))
	})
	t.Run("runtime management and foreign vhost cannot widen grants", func(t *testing.T) {
		for _, user := range []string{"svc_inventory", "relay_inventory", "intake_retail"} {
			req, err := http.NewRequest(http.MethodPut, f.managementAddress+"/api/permissions/%2Fjustixauto/"+user, strings.NewReader(`{"configure":".*","write":".*","read":".*"}`))
			if err != nil { t.Fatal(err) }
			req.SetBasicAuth(user, f.password)
			req.Header.Set("Content-Type", "application/json")
			resp, err := f.client.Do(req)
			if err != nil { t.Fatal(err) }
			_ = resp.Body.Close()
			if resp.StatusCode != 401 && resp.StatusCode != 403 { t.Fatalf("runtime ACL escalation status %d", resp.StatusCode) }
		}
		if _, err := f.api("PUT", "/api/vhosts/qa-foreign", []byte(`{}`)); err != nil { t.Fatal(err) }
		for _, user := range []string{"svc_inventory", "relay_inventory", "intake_retail"} {
			u := url.URL{Scheme:"amqp", Host:f.amqpAddress, User:url.UserPassword(user,f.password), Path:"/qa-foreign"}
			conn, err := amqp.DialConfig(u.String(), amqp.Config{Dial:amqp.DefaultDial(5*time.Second)})
			if err == nil { _ = conn.Close(); t.Fatal("foreign vhost opened") }
			if e, ok := err.(*amqp.Error); !ok || (e.Code != 403 && e.Code != 530) || !strings.Contains(e.Reason, "vhost") { t.Fatalf("foreign vhost unexpected error: %v", err) }
		}
	})
	t.Run("connection loss requeues original bytes and duplicate publication stays visible", func(t *testing.T) {
		body := string([]byte{0, 1, 255, '\n', '{', '}'})
		for i:=0; i<2; i++ { qaT924Confirmed(t, f, route, "duplicate-id", body) }
		ch := f.channel(t, "intake_retail")
		first, ok, err := ch.Get(queue, false)
		if err != nil || !ok { t.Fatalf("initial delivery: %v %v",ok,err) }
		if !bytes.Equal(first.Body, []byte(body)) || first.MessageId != "duplicate-id" { t.Fatal("initial bytes changed") }
		if err := ch.Close(); err != nil { t.Fatal(err) }
		recovered := f.get(t, "intake_retail", queue)
		if !recovered.Redelivered || !bytes.Equal(recovered.Body, []byte(body)) || recovered.MessageId != first.MessageId || recovered.RoutingKey != route || recovered.DeliveryMode != amqp.Persistent { t.Fatal("unacknowledged original did not recover") }
		duplicate := f.get(t, "intake_retail", queue)
		if !bytes.Equal(duplicate.Body, []byte(body)) || duplicate.MessageId != first.MessageId { t.Fatal("broker unexpectedly deduplicated identity") }
	})
	t.Run("broker restart retains backlog and final restrictive bootstrap", func(t *testing.T) {
		const body = "synthetic-backlog-before-restart\x00\xff"
		qaT924Confirmed(t, f, route, "restart-backlog", body)
		// The fixture originally bootstraps a synthetic legacy policy. Replace
		// only its own temporary mount with final deny-all definitions before
		// restart, modeling a correctly fenced deployment's retained config.
		final := readDefinitions(t)
		f.install(t, final)
		out, err := exec.Command("docker", "inspect", "--format", "{{json .Mounts}}", f.container).Output()
		if err != nil { t.Fatal(err) }
		var mounts []struct { Source, Destination, Type, Name string }
		if err := json.Unmarshal(out, &mounts); err != nil { t.Fatal(err) }
		written := false
		for _, m := range mounts {
			if m.Type == "volume" { t.Logf("owned anonymous volume: %s",m.Name) }
			if m.Destination == "/etc/rabbitmq/definitions.json" && m.Type == "bind" {
				if err := os.WriteFile(m.Source, f.importBytes(t, final), 0600); err != nil { t.Fatal(err) }
				written = true
			}
		}
		if !written { t.Fatal("owned bootstrap mount not found") }
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "restart", f.container).CombinedOutput(); err != nil { t.Fatalf("restart own fixture: %v %s",err,out) }
		// Docker can reassign HostPort=0 mappings across a restart.
		for _, port := range []string{"5672/tcp","15672/tcp"} {
			out, err := exec.CommandContext(ctx,"docker","port",f.container,port).Output()
			if err != nil { t.Fatal(err) }
			address := strings.TrimSpace(string(out))
			if !strings.HasPrefix(address,"127.0.0.1:") || strings.Contains(address,"\n") { t.Fatalf("unexpected restart mapping %q",address) }
			if port == "5672/tcp" { f.amqpAddress = address } else { f.managementAddress = "http://"+address }
		}
		deadline := time.Now().Add(60*time.Second)
		for {
			if _, err := f.api("GET", "/api/overview", nil); err == nil { break }
			if time.Now().After(deadline) { t.Fatal("restart unavailable") }
			time.Sleep(100*time.Millisecond)
		}
		got := f.get(t, "intake_retail", queue)
		if got.MessageId != "restart-backlog" || !bytes.Equal(got.Body, []byte(body)) || got.RoutingKey != route || got.DeliveryMode != amqp.Persistent { t.Fatal("restart mutated/lost retained message") }
		for _, user := range []string{"svc_inventory", "relay_inventory", "intake_inventory"} {
			f.denied(t, user, func(ch *amqp.Channel) error { return brokerPublish(ch, integrationExchange, route, "restart-denied", "synthetic") })
		}
		if err := f.auditBindings(final); err != nil { t.Fatal(err) }
		t.Log(fmt.Sprintf("retained restart and deny-all verified on %s",f.container))
	})
}

func qaT924Confirmed(t *testing.T, f *brokerFixture, route, id, body string) {
	t.Helper()
	ch := f.channel(t,"relay_inventory")
	closed := ch.NotifyClose(make(chan *amqp.Error,1))
	if err := ch.Confirm(false); err != nil { t.Fatal(err) }
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation,1))
	returns := ch.NotifyReturn(make(chan amqp.Return,1))
	if err := brokerPublish(ch,integrationExchange,route,id,body); err != nil { t.Fatal(err) }
	select {
	case c,ok := <-confirms:
		if !ok { t.Fatalf("confirm channel closed: %v", <-closed) }
		if !c.Ack || c.DeliveryTag != 1 { t.Fatalf("unexpected confirm: %#v",c) }
	case err := <-closed: t.Fatalf("publication closed: %v",err)
	case <-time.After(5*time.Second): t.Fatal("no confirm")
	}
	select { case r := <-returns: t.Fatalf("unexpected return: %#v",r); default: }
}

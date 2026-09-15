package outbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	ErrPublishUnavailable = errors.New("outbox AMQP attempt unavailable or outcome unknown")
	ErrPublishReturned    = errors.New("outbox mandatory publication was returned")
	ErrPublishNack        = errors.New("outbox publication was negatively confirmed")
	ErrPublishCorrelation = errors.New("outbox AMQP confirmation correlation failed")
)

// RoutedConfirmation is constructed only after a mandatory persistent publish
// receives its correlated positive confirm without a return. It is not evidence
// of consumer intake, business effects or intended binding topology.
type RoutedConfirmation struct {
	publication Publication
	routed      bool
}

func (c RoutedConfirmation) matches(p Publication) bool {
	return c.routed && p.committed && c.publication.committed && c.publication.eventID == p.eventID && c.publication.exchange == p.exchange &&
		c.publication.routingKey == p.routingKey && c.publication.attempt == p.attempt && c.publication.hash == p.hash && bytes.Equal(c.publication.body, p.body)
}

// AMQPDialer must honor context during TCP/TLS/AMQP negotiation and return a NEW
// dedicated connection with automatic recovery disabled. Inject relay_<owner>
// credentials at the composition boundary; never give it a provisioning role.
// The publisher closes every returned connection. It never declares/binds/reads
// topology and does not keep a URL or credentials in results or log messages.
type AMQPDialer func(context.Context) (*amqp.Connection, error)

type AMQPPublisher struct {
	dial    AMQPDialer
	timeout time.Duration
}

func NewAMQPPublisher(dial AMQPDialer, timeout time.Duration) (*AMQPPublisher, error) {
	if dial == nil || timeout < time.Millisecond || timeout > time.Minute {
		return nil, ErrRelayConfiguration
	}
	return &AMQPPublisher{dial, timeout}, nil
}

func (p *AMQPPublisher) Publish(ctx context.Context, publication Publication) (RoutedConfirmation, error) {
	if p == nil || p.dial == nil || !publication.committed || !admissionUUID(publication.attempt) || !admissionUUID(publication.eventID) || publication.exchange != IntegrationExchange ||
		len(publication.routingKey) == 0 || len(publication.routingKey) > 255 || len(publication.body) == 0 || sha256.Sum256(publication.body) != publication.hash {
		return RoutedConfirmation{}, ErrInvalidRecord
	}
	if publication.until.IsZero() || !time.Now().Before(publication.until) {
		return RoutedConfirmation{}, ErrRelayLease
	}
	ctx, leaseCancel := context.WithDeadline(ctx, publication.until)
	defer leaseCancel()
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return RoutedConfirmation{}, err
	}
	conn, err := p.dial(ctx)
	if err != nil {
		if conn != nil {
			_ = conn.CloseDeadline(time.Now())
		}
		return RoutedConfirmation{}, errors.Join(ErrPublishUnavailable, ctx.Err())
	}
	if conn == nil {
		return RoutedConfirmation{}, ErrPublishUnavailable
	}
	defer conn.CloseDeadline(time.Now())
	// Client automatic recovery could change channel sequence/correlation under
	// this attempt. Reconnect is a new explicitly scheduled database delivery.
	if conn.IsRecoveryEnabled() {
		return RoutedConfirmation{}, ErrRelayConfiguration
	}
	done := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		select {
		case <-ctx.Done():
			_ = conn.CloseDeadline(time.Now())
		case <-done:
		}
	}()
	defer func() { close(done); <-closed }()
	ch, err := conn.Channel()
	if err != nil {
		return RoutedConfirmation{}, errors.Join(ErrPublishUnavailable, ctx.Err())
	}
	returns := ch.NotifyReturn(make(chan amqp.Return, 1))
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	closes := ch.NotifyClose(make(chan *amqp.Error, 1))
	if err := ch.Confirm(false); err != nil {
		return RoutedConfirmation{}, errors.Join(ErrPublishUnavailable, ctx.Err())
	}
	seq := ch.GetNextPublishSeqNo()
	if err := ch.PublishWithContext(ctx, publication.exchange, publication.routingKey, true, false, amqp.Publishing{
		ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: publication.eventID, Body: bytes.Clone(publication.body),
	}); err != nil {
		return RoutedConfirmation{}, errors.Join(ErrPublishUnavailable, ctx.Err())
	}
	return awaitRouted(ctx, publication, seq, returns, confirms, closes)
}

// There is exactly one outstanding publication on a dedicated channel. RabbitMQ
// sends basic.return before basic.ack for an unroutable mandatory message;
// amqp091-go 1.14 dispatches NotifyReturn before NotifyPublish. On selecting ACK
// we MUST drain the already-buffered return: Go select order is not wire order.
// See https://www.rabbitmq.com/docs/confirms and the pinned Channel.Confirm docs.
// Any lost channel, NACK, return, cancellation or wrong sequence yields no receipt.
func awaitRouted(ctx context.Context, p Publication, seq uint64, returns <-chan amqp.Return, confirms <-chan amqp.Confirmation, closes <-chan *amqp.Error) (RoutedConfirmation, error) {
	returned := false
	checkReturn := func(r amqp.Return) error {
		if r.MessageId != p.eventID || r.Exchange != p.exchange || r.RoutingKey != p.routingKey ||
			!bytes.Equal(r.Body, p.body) {
			return ErrPublishCorrelation
		}
		returned = true
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return RoutedConfirmation{}, errors.Join(ErrPublishUnavailable, ctx.Err())
		case <-closes:
			return RoutedConfirmation{}, ErrPublishUnavailable
		case r, ok := <-returns:
			if !ok {
				return RoutedConfirmation{}, ErrPublishUnavailable
			}
			if err := checkReturn(r); err != nil {
				return RoutedConfirmation{}, err
			}
		case c, ok := <-confirms:
			if !ok {
				return RoutedConfirmation{}, ErrPublishUnavailable
			}
			if c.DeliveryTag != seq || seq == 0 {
				return RoutedConfirmation{}, ErrPublishCorrelation
			}
			select {
			case r, ok := <-returns:
				if !ok {
					return RoutedConfirmation{}, ErrPublishUnavailable
				}
				if err := checkReturn(r); err != nil {
					return RoutedConfirmation{}, err
				}
			default:
			}
			if err := ctx.Err(); err != nil {
				return RoutedConfirmation{}, errors.Join(ErrPublishUnavailable, err)
			}
			if returned {
				return RoutedConfirmation{}, ErrPublishReturned
			}
			if !c.Ack {
				return RoutedConfirmation{}, ErrPublishNack
			}
			return RoutedConfirmation{p, true}, nil
		}
	}
}

package events

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Owner string

const (
	OwnerIdentity  Owner = "identity"
	OwnerInventory Owner = "inventory"
	OwnerCommerce  Owner = "commerce"
	OwnerRetail    Owner = "retail"
	OwnerFinancing Owner = "financing"
	OwnerInsurance Owner = "insurance"
	OwnerDocuments Owner = "documents"
)

type ActorKind string

const (
	ActorUser    ActorKind = "user"
	ActorService ActorKind = "service"
	ActorSystem  ActorKind = "system"
)

type Actor struct {
	Kind ActorKind `json:"kind"`
	ID   string    `json:"id"`
}

type CompanyScope string

const (
	CompanyScopeTenantRequired CompanyScope = "tenant-required"
	CompanyScopeGlobalAllowed  CompanyScope = "global-allowed"
)

// EnvelopeInput contains the metadata required to construct an integration
// event envelope. CompanyID may be nil only for global identity events.
type EnvelopeInput struct {
	EventID             string
	AggregateID         string
	AggregateVersion    Revision
	IntegrationSequence Revision
	CompanyID           *string
	OccurredAt          time.Time
	Actor               Actor
	CorrelationID       string
	CausationID         string
	OperationID         string
}

// EventSchema is an owner-declared binding between an event name, its schema
// version, its aggregate stream and its company scope. Shared mechanics retain
// no registry of business event or aggregate types.
type EventSchema[T any] struct {
	eventType     string
	schemaVersion uint32
	aggregateType string
	companyScope  CompanyScope
	validate      func(T) error
}

// Envelope is immutable after construction. Its payload is retained as copied
// JSON bytes and is only exposed through copying accessors or typed decoding.
type Envelope struct {
	eventID             string
	eventType           string
	owner               Owner
	schemaVersion       uint32
	aggregateType       string
	aggregateID         string
	aggregateVersion    Revision
	integrationSequence Revision
	companyID           *string
	occurredAt          time.Time
	actor               Actor
	correlationID       string
	causationID         string
	operationID         string
	data                []byte
}

var (
	ErrInvalidEnvelope       = errors.New("invalid event envelope")
	ErrSensitivePayload      = errors.New("event payload contains secret or raw PII")
	ErrPayloadSchemaMismatch = errors.New("event payload schema mismatch")
	ErrInvalidPayload        = errors.New("invalid event payload")
	eventTypePattern         = regexp.MustCompile(`^([a-z][a-z0-9-]*)(?:\.[a-z][a-z0-9-]*)+\.v([1-9][0-9]*)$`)
	aggregateTypePattern     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

func NewEventSchema[T any](eventType string, schemaVersion uint32, aggregateType string, companyScope CompanyScope, validate func(T) error) (EventSchema[T], error) {
	matches := eventTypePattern.FindStringSubmatch(eventType)
	if matches == nil {
		return EventSchema[T]{}, fmt.Errorf("%w: schema eventType must be owner-qualified and versioned", ErrInvalidEnvelope)
	}
	owner := Owner(matches[1])
	version, err := strconv.ParseUint(matches[2], 10, 32)
	if !owner.Valid() || err != nil || version != uint64(schemaVersion) || schemaVersion == 0 {
		return EventSchema[T]{}, fmt.Errorf("%w: schema eventType and schemaVersion must match", ErrInvalidEnvelope)
	}
	if !aggregateTypePattern.MatchString(aggregateType) {
		return EventSchema[T]{}, fmt.Errorf("%w: schema aggregateType must be a lower-case slug", ErrInvalidEnvelope)
	}
	switch companyScope {
	case CompanyScopeTenantRequired:
	case CompanyScopeGlobalAllowed:
		if owner != OwnerIdentity {
			return EventSchema[T]{}, fmt.Errorf("%w: only identity schemas may allow global events", ErrInvalidEnvelope)
		}
	default:
		return EventSchema[T]{}, fmt.Errorf("%w: unsupported company scope %q", ErrInvalidEnvelope, companyScope)
	}
	if validate == nil {
		return EventSchema[T]{}, fmt.Errorf("%w: schema requires payload validation", ErrInvalidEnvelope)
	}
	return EventSchema[T]{
		eventType: eventType, schemaVersion: schemaVersion, aggregateType: aggregateType,
		companyScope: companyScope, validate: validate,
	}, nil
}

func NewEnvelope[T any](input EnvelopeInput, schema EventSchema[T], data T) (Envelope, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return Envelope{}, fmt.Errorf("%w: encode data: %v", ErrInvalidEnvelope, err)
	}
	return NewEnvelopeJSON(input, schema, payload)
}

func NewEnvelopeJSON[T any](input EnvelopeInput, schema EventSchema[T], data json.RawMessage) (Envelope, error) {
	envelope, err := newEnvelopeFromJSON(input, schema.eventType, schema.schemaVersion, schema.aggregateType, data)
	if err != nil {
		return Envelope{}, err
	}
	if _, err := DecodeData(envelope, schema); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func newEnvelopeFromJSON(input EnvelopeInput, eventType string, schemaVersion uint32, aggregateType string, data []byte) (Envelope, error) {
	owner, err := validateMetadata(input, eventType, schemaVersion, aggregateType)
	if err != nil {
		return Envelope{}, err
	}
	payload, err := validateAndCopyPayload(data)
	if err != nil {
		return Envelope{}, err
	}

	var companyID *string
	if input.CompanyID != nil {
		copied := *input.CompanyID
		companyID = &copied
	}
	return Envelope{
		eventID: input.EventID, eventType: eventType, owner: owner,
		schemaVersion: schemaVersion, aggregateType: aggregateType,
		aggregateID: input.AggregateID, aggregateVersion: input.AggregateVersion,
		integrationSequence: input.IntegrationSequence, companyID: companyID,
		occurredAt: input.OccurredAt.UTC(), actor: input.Actor,
		correlationID: input.CorrelationID, causationID: input.CausationID,
		operationID: input.OperationID, data: payload,
	}, nil
}

func validateMetadata(input EnvelopeInput, eventType string, schemaVersion uint32, aggregateType string) (Owner, error) {
	for name, id := range map[string]string{
		"eventId": input.EventID, "aggregateId": input.AggregateID,
		"actor.id": input.Actor.ID, "correlationId": input.CorrelationID,
		"causationId": input.CausationID, "operationId": input.OperationID,
	} {
		if !isCanonicalUUID(id) {
			return "", fmt.Errorf("%w: %s must be a canonical UUID", ErrInvalidEnvelope, name)
		}
	}

	matches := eventTypePattern.FindStringSubmatch(eventType)
	if matches == nil {
		return "", fmt.Errorf("%w: eventType must be owner-qualified and end in .v<schemaVersion>", ErrInvalidEnvelope)
	}
	owner := Owner(matches[1])
	if !owner.Valid() {
		return "", fmt.Errorf("%w: unsupported event owner %q", ErrInvalidEnvelope, owner)
	}
	version, err := strconv.ParseUint(matches[2], 10, 32)
	if err != nil || version != uint64(schemaVersion) || schemaVersion == 0 {
		return "", fmt.Errorf("%w: eventType version and schemaVersion must match", ErrInvalidEnvelope)
	}
	if !aggregateTypePattern.MatchString(aggregateType) {
		return "", fmt.Errorf("%w: aggregateType must be a lower-case slug", ErrInvalidEnvelope)
	}
	if input.AggregateVersion.IsZero() {
		return "", fmt.Errorf("%w: aggregateVersion must be positive", ErrInvalidEnvelope)
	}
	if input.IntegrationSequence.IsZero() {
		return "", fmt.Errorf("%w: integrationSequence must be positive", ErrInvalidEnvelope)
	}
	if input.CompanyID == nil && owner != OwnerIdentity {
		return "", fmt.Errorf("%w: companyId may be null only for identity events", ErrInvalidEnvelope)
	}
	if input.CompanyID != nil && !isCanonicalUUID(*input.CompanyID) {
		return "", fmt.Errorf("%w: companyId must be a canonical UUID", ErrInvalidEnvelope)
	}
	if !input.Actor.Kind.Valid() {
		return "", fmt.Errorf("%w: unsupported actor kind %q", ErrInvalidEnvelope, input.Actor.Kind)
	}
	if input.OccurredAt.IsZero() {
		return "", fmt.Errorf("%w: occurredAt is required", ErrInvalidEnvelope)
	}
	_, offset := input.OccurredAt.Zone()
	if offset != 0 {
		return "", fmt.Errorf("%w: occurredAt must be UTC", ErrInvalidEnvelope)
	}
	return owner, nil
}

func validateAndCopyPayload(data []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%w: data must be valid JSON: %v", ErrInvalidEnvelope, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	object, ok := decoded.(map[string]any)
	if !ok || object == nil {
		return nil, fmt.Errorf("%w: data must be a JSON object", ErrInvalidEnvelope)
	}
	if err := inspectPayload(object, "data"); err != nil {
		return nil, err
	}
	compact := bytes.NewBuffer(make([]byte, 0, len(data)))
	if err := json.Compact(compact, data); err != nil {
		return nil, fmt.Errorf("%w: compact data: %v", ErrInvalidEnvelope, err)
	}
	return bytes.Clone(compact.Bytes()), nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: data must contain one JSON value", ErrInvalidEnvelope)
	}
	return nil
}

func inspectPayload(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isSensitivePayloadKey(key) {
				return fmt.Errorf("%w: %s.%s", ErrSensitivePayload, path, key)
			}
			if err := inspectPayload(child, path+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range typed {
			if err := inspectPayload(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func isSensitivePayloadKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
	switch normalized {
	case "nationalid", "passportnumber", "taxid", "personalid", "dateofbirth", "birthdate", "surname", "mobile":
		return true
	}
	if strings.HasSuffix(normalized, "ref") || strings.HasSuffix(normalized, "refs") ||
		strings.HasSuffix(normalized, "id") || strings.HasSuffix(normalized, "ids") {
		return false
	}
	for _, fragment := range []string{"password", "secret", "accesstoken", "refreshtoken", "sessionhandle", "sessiontoken", "cookie", "credential", "recoverycode", "totp", "apikey", "privatekey", "authorization", "bearer"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	switch normalized {
	case "email", "phone", "address", "firstname", "lastname", "fullname", "displayname", "legalname", "pii", "note":
		return true
	default:
		return false
	}
}

func isCanonicalUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

func (o Owner) Valid() bool {
	switch o {
	case OwnerIdentity, OwnerInventory, OwnerCommerce, OwnerRetail, OwnerFinancing, OwnerInsurance, OwnerDocuments:
		return true
	default:
		return false
	}
}

func (k ActorKind) Valid() bool {
	switch k {
	case ActorUser, ActorService, ActorSystem:
		return true
	default:
		return false
	}
}

func (e Envelope) EventID() string               { return e.eventID }
func (e Envelope) EventType() string             { return e.eventType }
func (e Envelope) Owner() Owner                  { return e.owner }
func (e Envelope) SchemaVersion() uint32         { return e.schemaVersion }
func (e Envelope) AggregateType() string         { return e.aggregateType }
func (e Envelope) AggregateID() string           { return e.aggregateID }
func (e Envelope) AggregateVersion() Revision    { return e.aggregateVersion }
func (e Envelope) IntegrationSequence() Revision { return e.integrationSequence }
func (e Envelope) OccurredAt() time.Time         { return e.occurredAt }
func (e Envelope) Actor() Actor                  { return e.actor }
func (e Envelope) CorrelationID() string         { return e.correlationID }
func (e Envelope) CausationID() string           { return e.causationID }
func (e Envelope) OperationID() string           { return e.operationID }

func (e Envelope) CompanyID() (string, bool) {
	if e.companyID == nil {
		return "", false
	}
	return *e.companyID, true
}

func (e Envelope) Data() json.RawMessage { return bytes.Clone(e.data) }

// DecodeData decodes the immutable payload through an explicitly bound schema.
func DecodeData[T any](e Envelope, schema EventSchema[T]) (T, error) {
	var target T
	if schema.eventType == "" || schema.schemaVersion == 0 || schema.aggregateType == "" || schema.validate == nil {
		return target, fmt.Errorf("%w: uninitialized schema", ErrPayloadSchemaMismatch)
	}
	if e.eventType != schema.eventType || e.schemaVersion != schema.schemaVersion || e.aggregateType != schema.aggregateType {
		return target, fmt.Errorf("%w: envelope is %s/%s schema v%d, decoder is %s/%s schema v%d",
			ErrPayloadSchemaMismatch, e.eventType, e.aggregateType, e.schemaVersion,
			schema.eventType, schema.aggregateType, schema.schemaVersion)
	}
	if schema.companyScope == CompanyScopeTenantRequired && e.companyID == nil {
		return target, fmt.Errorf("%w: schema requires companyId", ErrPayloadSchemaMismatch)
	}
	if schema.companyScope == CompanyScopeGlobalAllowed && e.owner != OwnerIdentity {
		return target, fmt.Errorf("%w: global scope is limited to identity", ErrPayloadSchemaMismatch)
	}
	jsonDecoder := json.NewDecoder(bytes.NewReader(e.data))
	jsonDecoder.DisallowUnknownFields()
	if err := jsonDecoder.Decode(&target); err != nil {
		return target, fmt.Errorf("decode %s schema v%d: %w", e.eventType, e.schemaVersion, err)
	}
	if err := requireJSONEOF(jsonDecoder); err != nil {
		return target, err
	}
	if err := schema.validate(target); err != nil {
		return target, fmt.Errorf("%w: %s schema v%d: %v", ErrInvalidPayload, e.eventType, e.schemaVersion, err)
	}
	return target, nil
}

type envelopeJSON struct {
	EventID             string          `json:"eventId"`
	EventType           string          `json:"eventType"`
	SchemaVersion       uint32          `json:"schemaVersion"`
	AggregateType       string          `json:"aggregateType"`
	AggregateID         string          `json:"aggregateId"`
	AggregateVersion    Revision        `json:"aggregateVersion"`
	IntegrationSequence Revision        `json:"integrationSequence"`
	CompanyID           *string         `json:"companyId"`
	OccurredAt          string          `json:"occurredAt"`
	Actor               Actor           `json:"actor"`
	CorrelationID       string          `json:"correlationId"`
	CausationID         string          `json:"causationId"`
	OperationID         string          `json:"operationId"`
	Data                json.RawMessage `json:"data"`
}

func (e Envelope) MarshalJSON() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(envelopeJSON{
		EventID: e.eventID, EventType: e.eventType, SchemaVersion: e.schemaVersion,
		AggregateType: e.aggregateType, AggregateID: e.aggregateID,
		AggregateVersion: e.aggregateVersion, IntegrationSequence: e.integrationSequence,
		CompanyID: e.companyID, OccurredAt: e.occurredAt.Format(time.RFC3339Nano), Actor: e.actor,
		CorrelationID: e.correlationID, CausationID: e.causationID,
		OperationID: e.operationID, Data: bytes.Clone(e.data),
	})
}

// Validate rechecks structural envelope invariants. An envelope received across
// an untrusted or persistence boundary still requires DecodeData with an
// owner-declared EventSchema before its type, aggregate and scope are accepted.
func (e Envelope) Validate() error {
	var companyID *string
	if e.companyID != nil {
		copied := *e.companyID
		companyID = &copied
	}
	owner, err := validateMetadata(EnvelopeInput{
		EventID: e.eventID, AggregateID: e.aggregateID,
		AggregateVersion: e.aggregateVersion, IntegrationSequence: e.integrationSequence,
		CompanyID: companyID, OccurredAt: e.occurredAt, Actor: e.actor,
		CorrelationID: e.correlationID, CausationID: e.causationID,
		OperationID: e.operationID,
	}, e.eventType, e.schemaVersion, e.aggregateType)
	if err != nil {
		return err
	}
	if owner != e.owner {
		return fmt.Errorf("%w: event owner does not match eventType", ErrInvalidEnvelope)
	}
	_, err = validateAndCopyPayload(e.data)
	return err
}

func (e *Envelope) UnmarshalJSON(data []byte) error {
	if e == nil {
		return fmt.Errorf("%w: nil destination", ErrInvalidEnvelope)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire envelopeJSON
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %v", ErrInvalidEnvelope, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	if !strings.HasSuffix(wire.OccurredAt, "Z") {
		return fmt.Errorf("%w: occurredAt must use UTC Z notation", ErrInvalidEnvelope)
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, wire.OccurredAt)
	if err != nil {
		return fmt.Errorf("%w: occurredAt must be RFC3339: %v", ErrInvalidEnvelope, err)
	}
	constructed, err := newEnvelopeFromJSON(EnvelopeInput{
		EventID: wire.EventID, AggregateID: wire.AggregateID,
		AggregateVersion: wire.AggregateVersion, IntegrationSequence: wire.IntegrationSequence,
		CompanyID: wire.CompanyID, OccurredAt: occurredAt, Actor: wire.Actor,
		CorrelationID: wire.CorrelationID, CausationID: wire.CausationID,
		OperationID: wire.OperationID,
	}, wire.EventType, wire.SchemaVersion, wire.AggregateType, wire.Data)
	if err != nil {
		return err
	}
	*e = constructed
	return nil
}

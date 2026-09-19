# Inbox invariants

Deduplication and business effect share the approved owner transaction. Exercise
duplicate delivery, failure/rollback and retry. ACK requires the approved durable
outcome; poison payloads require durable quarantine. Respect recipient membership
and tenant boundaries. A successful broker connection proves no delivery guarantee.

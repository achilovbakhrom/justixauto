# Shared Go mechanics

Use `bash tools/go.sh`; shared packages cannot own business tables or import
service domains. Follow approved contracts rather than inventing delivery or
transaction guarantees. Changes to a shared interface require explicit downstream
compatibility checks in the packet plan. Pin evidence to a commit. Run relevant
package tests with race detection when concurrency is affected.

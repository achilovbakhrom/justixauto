# Architecture intake review — planning-r1

Status: one open synthesis correction; one source correction resolved during review. Read-only architecture review, not implementation QA or approval. Reviewed current target `business-logic.md`, `open-decisions.md`, `mock-map.md`, coordinator conventions/review notes, and all three available analysis slices. Known review-note issues are not repeated below.

## Findings

1. **P1 — scanned file identity is not pinned to immutable storage bytes.** `analysis/retail-finance-insurance.md:119–127` proposes a private `objectKey`, a presigned upload, a scan verdict, and later download/binding of a document version, but does not prohibit reusing the still-valid upload capability to replace that key after scanning. A metadata-level “immutable DocumentVersion” does not itself ensure the downloaded object is the bytes the scanner examined. Synthesis should require a write-once finalized object or exact immutable storage-generation/version identifier, tie scan verdict and content digest to that identity, and use the same identity for bindings/downloads. Include a test where upload is replayed/overwritten after clean scan: altered bytes must never inherit the clean verdict. This is a missing security contract, not a claim that an implementation already has this defect.

2. **P2, resolved during review — incorrect customer-scope correction.** The initially read `review-notes.md` called “natural persons only” unconfirmed and directed a client-kind model; however, current target `business-logic.md:56` explicitly defines `CRM | Lead, Customer-физлицо`. The retail slice's natural-person Customer is consistent with that source. The coordinator was notified and has corrected `review-notes.md:19–21` to retain that scope. No additional change required; corporate-customer support is not automatically confirmed.

## Boundary

No additional concrete blocker found beyond the coordinator's existing known items and these two corrections. This does not validate an eventual synthesized reservation/branch/submission protocol: those must be reviewed once their definitive contracts exist. No network/browser/code execution QA, target edits, or application changes performed.

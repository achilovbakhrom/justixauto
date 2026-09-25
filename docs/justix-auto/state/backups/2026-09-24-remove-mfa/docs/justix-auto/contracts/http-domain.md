# HTTP/domain contracts v1 — subordinate architecture proposal

Status: ARCHITECTURE BASELINE APPROVED 2026-09-13; part of `../architecture.md`. Typed contract baseline for owner tasks, not implemented APIs or an exhaustive approved release backlog. OD/security/visual-state gates remain.
Common transport, domain fields, authorization and OD gates in the parent apply. Tables use owner-relative paths: prefix every public row with `/api/v1/{owner}`.
Existing aggregate writes use If-Match, never body expectedVersion; additional referenced revisions have explicit field names. Creates and actions are idempotent by their data (unique business keys, state checks, If-Match), never by a request key: a repeat gets 409/412, not a replayed response.
Authentication handshakes are exceptions to the business command ledger: login,
MFA verification/enrollment and recovery use rate limits, session-bound single-use
challenges/tokens and replay guards; logout is idempotent session revocation.
Do not persist credentials, MFA codes or authentication responses in a generic
command receipt. Authenticated provider provisioning is a business command whose
repeat is rejected by the company and user unique keys (see below).
Request bodies below do not include trusted actor/company/permissions; use authenticated context. Explicit counterparty IDs select an object and must be authorized.
Response: create201/command200 `{data:<resource receipt>,revision:<string>,operationId}`; cross-owner202 `{operationId,status:"pending"}`. Logout204 is the explicit no-body exception.
All receipts carry exact affected IDs/state; multi-object receipts have `data.related:[{type,id,revision}]`. GET detail `{data:<typed resource>,revision,asOf}`; list `{items,nextCursor,asOf}`.
Typed resources are the parent data model plus allowedActions:string[]; no private auth records/PII refs/object keys in ordinary public DTOs. Public PII fields require owner field-level authorization.
Errors use `{error:{code,message,fields:{},traceId},operationId?}`; status400 syntax,401 session,403 permission/MFA/policy,404 hidden/missing,409 invariant,412 stale,422 fields,428 missing precondition,503 authoritative dependency unavailable.

## 1. Common exact value contracts

```text
ID := UUID string
Revision := canonical nonnegative decimal string, <= signed bigint maximum
Quantity := positive Revision; SignedDelta := canonical signed bigint decimal string
Money := {amountMinor: canonical integer string <=38 digits, currency: uppercase code}
Country := {key?:string,label:string}; Region := {key?:string,label:string}
CompanyInput := {name,legalName,country:Country,region?:Region,registration,email,address?,phone?}
WarehouseInput := {name,country:Country,region?:Region,city,address,capacity:Quantity}
VehicleSpecification := {make:string,model:string,variant:string,year:integer,
 bodyType:string,exteriorColor:string,interiorColor:string,powertrain:string,drivetrain:string}
VehiclePhotos := {exterior:[{versionId:ID,bindingId:ID,position:integer}],
 interior:[{versionId:ID,bindingId:ID,position:integer}]}
Scope := {mode:"ALL"|"SELECTED",branchIds:ID[]}
CalculationInput := {price:Money,downPayment:Money,termMonths:Quantity,firstDueDate:date,
 businessTimezone:string,rateParameters:record<string,decimal-string>}
CalculationSnapshot := {id,kind:"own-installment"|"partner-program",policyId,policyVersion,
 programVersion?,input:CalculationInput,output:{total:Money,financedAmount:Money,
 schedule:[{number:Quantity,dueDate:date,principal?:Money,charge?:Money,total:Money,balance:Money}]},
 inputDigest,resultDigest,calculatedAt}
CommercialLine := {lineId,modelId,quantity:Quantity,unitPrice:Money}
CommercialTerms := {lines:CommercialLine[],route:"factory"|"foreign-direct"|"in-transit"|"local",
 deliveryTerms:string,paymentSchedule:[{id,amount:Money,dueDate:date}],
 warrantyTerms:string,serviceTerms:string}
```

Company country supports free input, normalization determines uniqueness; changing country requires cleared/reselected region. Demo list is not a national enum.
Vehicle field mapping to current `vehicle-fields.js`: make←brand, model←modelName, variant←trim, then year/bodyType/exteriorColor/interiorColor/powertrain/drivetrain. These are nine separate inputs, never one combined description.
Make/model/variant autocomplete permits free entry; model is disabled without make, variant disabled without make+model. Changing make clears model+variant and refreshes suggestions; changing model clears variant. Server rejects stale dependent references; typed new labels resolve/create versioned catalog references through an authorized catalog command, never by hidden VIN mutation.
Exterior/interior colors autocomplete independently. Year/bodyType/powertrain/drivetrain use server catalogue options; current mock enum/year bounds are evidence for visual parity, not permanent production country/model limits. Vehicle detail carries exact specification version + separately ordered exterior/interior immutable photo bindings; a gallery photo is not automatically mandatory milestone evidence.
No undefined floating monetary fields. Output currency must agree with inputs/program, approved schedule components close exactly, and each policy supplies its own required rates/date/rounding schema.
Missing production calculation policy returns POLICY_UNRESOLVED; fixture demo IDs are nonproduction-only. Program versions never rewrite old submitted calculations.
Input length/upload limits are named versioned server configuration delivered with applicable forms; absence of required production configuration is a deployment validation error, never a copied demo default.

## 2. Identity/session

| Method/path | Exact request / result specifics |
|---|---|
| GET `/session` | `{data:{user:{id,displayName,status},roles:[{id,name}],permissions:string[],mfa:{enrolled,authenticatedAt?},context:{revision,companyId:null\|ID,branchScope:Scope},accessibleCompanies:[{id,name,kind,access}],setup:{next:company\|branch\|none,partnershipRequiredForB2B:true}},revision,asOf}`; revision is session context revision |
| POST `/session/login` | `{login,password}` → session cookie + session view, or `{data:{challengeId,required:"mfa"},revision:"0",operationId}` with restricted challenge cookie only |
| POST `/session/mfa/verify` | `{challengeId,code}` → rotated cookie + session view; challenge single-use |
| POST `/session/logout` | `{}` + CSRF → revoke cookie/session,204 |
| PUT `/session/context` | `{companyId}` + If-Match session context revision → context with revision incremented and branchScope ALL/[] |
| PUT `/session/branch-scope` | Scope + If-Match session context revision → context; ALL requires[], SELECTED nonempty permitted branches |
| POST `/session/revoke-all` | `{reason}` + reauth/MFA → revoke other sessions, rotate current cookie |
| POST `/session/mfa/enrollment` | `{}` in restricted authenticated enrollment flow → `{enrollmentId,secretOrChallenge}` once, never logged |
| POST `/session/mfa/enrollment/{id}/confirm` | `{code}` → enrolled, rotate security/session version |
| POST `/session/recovery/request` | `{login}` → uniform202, no account-existence response; delivery/proof security gate |
| POST `/session/recovery/complete` | `{token,newPassword,confirmation}` →204 + revoke sessions; no role/membership changes |
| POST `/admin/provider-companies` | `{kind:"bank"\|"mfo"\|"insurance",company:CompanyInput,firstAdmin:{displayName,login,email,password,passwordConfirmation}}` → organization/user/membership refs, company draft |
| POST `/companies` | `{company:CompanyInput}` → seller organization + creator membership, no new privilege/context switch |
| PATCH `/companies/{id}` | CompanyInput → stable ID/current profile revision, old snapshots preserved |
| POST `/admin/companies/{id}/activate`, `/suspend`, `/restore` | `{reason}` → access draft→active / active→suspended / suspended→active; last two exposure gated OD-05 |
| POST `/companies/{id}/branches` | `{name,address?,warehouse:{mode:"none"}\|{mode:"link",warehouseId,warehouseRevision}\|{mode:"create",input:WarehouseInput}}` → branch201 or linked-setup202 |
| PATCH `/companies/{id}/branches/{branchId}` | `{name,address?}` → details only; reject lifecycle fence |
| POST `/companies/{id}/branches/{branchId}/warehouse-attachments` | `{warehouse:{mode:"link",warehouseId,warehouseRevision}\|{mode:"create",input:WarehouseInput}}` →202; branch If-Match |
| POST `/companies/{id}/branches/{branchId}/warehouse-detachments` | `{warehouseId,warehouseRevision,reason}` →202; exact attachment protocol |
| POST `/admin/users` | `{displayName,email,roleIds:ID[]}` → pending user; no implicit password/enrollment |
| PATCH `/admin/users/{id}` | `{displayName,roleIds:ID[]}` → global roles, no membership-role field |
| POST `/admin/users/{id}/suspend`, `/restore`, `/revoke-sessions` | `{reason}` → protected versioned state/session action |
| POST `/admin/users/{id}/memberships` | `{companyId,branchAccess:{mode:"ALL_BRANCHES"\|"SELECTED_BRANCHES",branchIds:ID[]}}` → active membership; duplicate active409, regrant explicit existing revoked revision |
| PATCH `/admin/memberships/{id}/branch-access` | `{branchAccess:{mode,branchIds}}` → membership revision/context invalidation |
| POST `/admin/memberships/{id}/revoke` | `{reason}` → revoked; other memberships preserved |
| POST `/admin/roles`; PATCH `/admin/roles/{id}` | `{name,permissionKeys:string[]}` → custom role; unknown/system/unassignable keys denied |
| POST `/admin/companies/{id}/invitations` | `{email}` → `{id,status,expiresAt}`, no bearer token; proof/delivery approval required |
| POST `/invitations/accept` | `{token}` + verified matching identity → membership only; no overwrite/linking an unverified existing user |
| POST `/admin/invitations/{id}/revoke` | `{reason}` → pending invitation revoked |

Identity GET collections/details: `/companies/{id}`, `/admin/companies?kind&access`, `/admin/users`, `/admin/users/{id}`, `/admin/roles`, `/admin/permissions`, `/admin/audit?resourceType&resourceId&actorId`, company branches.
Permission catalog `{key,scope:platform|company,requiresMfa,assignable}` is deployment-versioned. Platform powers do not imply company-financial or insurer permissions.
Default business action permission is `<owner>.<resource>.<action>` (e.g. `financing.applications.terms`, `insurance.applications.approve`); identity special grants are `platform.companies.create|access`, `platform.users.manage`, `platform.memberships.manage`, `platform.roles.manage`, `platform.directory.read`, `platform.audit.read`, `company.create|edit`, `branches.create|edit`.
Catalog explicit allowlist is generated from approved action schemas; unknown action denied. Sensitive document download additionally requires `documents.sensitive.download` and owner-side read grant/MFA.
`requiresMfa=true` for platform access/roles/credentials, membership/user administration, provider provisioning, finance/insurance decisions and terms agreement, payment acceptance, fulfillment and sensitive downloads. Routine CRM contacts and model/profile edits use their permission without blanket step-up; catalog changes are versioned security decisions.
New company initial capabilities are empty, not inferred from kind or activation; assignment/live capability-compliance policy awaits OD-11. Provider sign-in can show the empty admitted workspace without publishing a program or granting product authority.
Secret-bearing commands (provider provisioning, user creation with an initial password) keep no request ledger: a repeat hits the company registration / user email unique keys and gets a generic 409; it never resets the password. The outcome is recovered by an authorized lookup of the created company or user.
Private `POST /internal/v1/identity/authorize {sessionHandle,operationId,action,companyId?,branchId?,resource:{type,id}?,contextRevision}` → `{decisionId,allowed,denyCode?,actorId,companyId?,effectiveBranchIds,policyRevision,securityRevision,mfaAuthenticatedAt?,decidedAt}`. Session handle never enters events/logs.

## 3. Inventory and commerce

| Owner / method/path | Exact body and authoritative effect |
|---|---|
| inventory POST `/vehicle-models`; POST `/vehicle-models/{id}/specification-versions` | `{specification:VehicleSpecification}` → ID/immutable specificationVersion, authorized catalogue editor; no existing VIN reassignment |
| inventory POST `/vehicle-units/{id}/photo-versions` | `{photos:VehiclePhotos}` → ordered photo-set version of exact clean bindings; no receipt/customs approval implied |
| inventory POST `/warehouses` | WarehouseInput → warehouse with branchId null; attachment only through identity process |
| inventory PATCH `/warehouses/{id}` | `{name,country,region?,city,address}` → profile only, no bypass of attachment fence |
| inventory POST `/warehouses/{id}/capacity-changes` | `{capacity:Quantity,reason}` → reject below occupied |
| inventory POST `/warehouses/{id}/receipt-batches` | `{modelId,modelSpecificationVersion,stock:{mode:"unidentified",quantity}\|{mode:"identified",vins:string[]},receivedAt,evidenceBindingIds,sourceOrderId?,sourceShipmentId?}` → batch+occupied atomic; identified mode also creates VIN units+placements; warehouse If-Match |
| inventory POST `/receipt-batches/{id}/identifications` | `{items:[{vin,modelId}],atomic:true}` → VIN/unit/placement/batch atomically; no per-item partial success |
| inventory POST `/receipt-batches/{id}/quantity-adjustments` | `{delta:SignedDelta,reason,evidenceBindingIds}` → reasoned delta/capacity guard; evidence/permission policy gated |
| inventory POST `/vehicle-units/{id}/warehouse-moves` | `{fromWarehouseId,toWarehouseId,occurredAt,evidenceBindingIds}` → same-company move, sorted locks, no title change |
| commerce POST `/partnerships` | `{counterpartyCompanyId}` → requested, reject self/duplicate pair |
| commerce POST `/partnerships/{id}/accept` | `{}` → requested→active, recipient only |
| commerce POST `/partnerships/{id}/decline` | `{reason}` → requested→declined, recipient only |
| commerce POST `/partnerships/{id}/withdraw` | `{reason}` → requested→withdrawn, requester only; outgoing Cancel mock action |
| commerce POST `/partnerships/{id}/end` | `{reason}` → active→ended, participant; preserve prior records |
| commerce POST `/offers`; POST `/offers/{id}/versions` | `{terms:CommercialTerms,audience:{mode:"all-active"\|"selected",partnerCompanyIds:ID[]}}` → draft/version |
| commerce POST `/offers/{id}/publish`; `/withdraw` | `{offerVersionId}` / `{reason}` → publish immutable exact version / withdraw; no reserve |
| commerce POST `/rfqs`; POST `/rfqs/{id}/send` | `{supplierCompanyId,offerVersionId?,lines:[{lineId,modelId,quantity}]}` / `{}` → draft/sent; active partnership |
| commerce POST `/rfqs/{id}/quotation-versions` | `{terms:CommercialTerms}` → supplier's immutable numbered quotation, RFQ negotiating |
| commerce POST `/rfqs/{id}/accept` | `{quotationVersionId,quotationDigest}` → buyer accepts exact current version + creates order atomically |
| commerce POST `/orders` | `{offerVersionId,lines:[{offerLineId,quantity}]}` → direct-order awaiting supplier with captured terms |
| commerce POST `/orders/{id}/supplier-confirmations` | `{}` → supplier accepts; no VIN allocation implied |
| commerce POST `/orders/{id}/allocations` | `{items:[{orderLineId,vehicleId}]}` → durable inventory reservation202 |
| commerce POST `/orders/{id}/addenda` | `{terms:CommercialTerms,reason}` → immutable proposed addendum, no old-snapshot overwrite |
| commerce POST `/orders/{id}/addenda/{addendumId}/accept` | `{addendumRevision}` → other-party acceptance; inventory-impacting changes separate OD-04 process |
| commerce POST `/orders/{id}/cancellations` | `{reason}` → policy-gated cancellation202, terminal only after exact inventory release |
| commerce POST `/orders/{id}/shipments` | `{vehicleIds,route}` → assigned concrete VINs only, no duplicate active allocation |
| commerce POST `/shipments/{id}/milestones` | `{milestoneType,occurredAt,location,responsibleActorId,evidenceBindingIds,policyVersion}` → allowed route fact only, OD-06 |
| commerce POST `/shipments/{id}/receipt-decisions` | `{decision:"accept"\|"reject",vehicleIds,warehouseId?,reason?,evidenceBindingIds,policyVersion}` → reject needs reason; accept202 waits inventory placement/capacity receipt |
| commerce POST `/invoices/{id}/payment-evidence` | `{claimedAmount:Money,paidOn,externalReference,attachmentBindingIds}` → buyer submitted external assertion |
| commerce POST `/payment-evidence/{id}/accept`; `/reject` | `{confirmation:true}` / `{reason}` → seller financial permission; no money transfer |

VIN syntax proposal: trim then uppercase,17 ASCII VIN characters excluding I/O/Q; no inferred national check-digit/WMI policy. Global duplicate returns VIN_UNAVAILABLE without foreign-owner details.
Manual receipt supports both known VINs and quantity awaiting VIN entry. In
identified mode nonempty unique `vins` fixes quantity; all VIN registrations,
units, placements, identified batch count and capacity commit together or all
roll back. Unidentified mode creates no fake units. Do not implement known-VIN
receipt as two browser commands whose first half can succeed alone. An existing
canonical VIN is never recreated: use its authorized move or shipment-receipt
flow with its stable vehicle ID; a duplicate lookup must not expose other tenants.
Inventory GET `/warehouses`, `/warehouses/{id}/inventory`, `/vehicle-units/{id}`, `/vehicle-units?placement=warehouse|outside|any&warehouseId`; separate unidentifiedBatches array, no fake VIN. OD-02 selects React default, not data ownership.
Commerce GET `/partnerships`, `/offers`, `/offers/{id}`, `/rfqs`, `/rfqs/{id}`, `/orders`, `/orders/{id}`, `/shipments/{id}`, `/invoices/{id}`; history/filter scope independently authorized.
Order detail `data={order,termsSnapshot,allocation:{state,operationId?,reservationSetId?},shipmentSummaries,paymentSummary,sourceRevisions,allowedActions}`. No combined arbitrary order/shipment/payment status setter.
Shipment receipt uses private `POST /internal/v1/inventory/receipt-operations/{op}/accept {shipmentId,sourceRevision,companyId,vehicleIds,warehouseId,evidenceBindingIds,policyVersion,admissionRef}` → `{operationId,state:"accepted"|"rejected",inventoryFactIds?,errorCode?}`; same-key status GET at same path; local inventory capacity transaction then commerce completion, no guessed success.

## 4. Retail, financing and insurance

| Owner / method/path | Body → state/action |
|---|---|
| retail POST `/customers` | `{profile:{displayName,phone?}}` → company-local natural person/PII record |
| retail POST `/leads` | `{customerId,branchId,source:"website"\|"telegram"\|"phone"\|"manual",assignedUserId?}` → new |
| retail POST `/leads/{id}/assign`; `/contacts`; `/stage` | `{assignedUserId}` / `{channel,note}` / `{stage,reason?}`; lost requires reason; won only from delivery |
| retail POST `/tasks`; `/tasks/{id}/complete` | `{customerId,leadId?,dealId?,ownerUserId,dueAt,title}` / `{}` → open/completed |
| retail POST `/listings`; PATCH `/listings/{id}` | `{vehicleId,text,askingPrice:Money}` / `{text,askingPrice}`; existing inventory reference |
| retail POST `/listings/{id}/publish`; `/withdraw` | `{}` → publication only, authoritative eligibility check |
| retail POST `/deals` | `{customerId,leadId?,vehicleId,branchId,paymentScheme,price:Money}` →202 pending reservation; optional lead must be eligible |
| retail POST `/deals/{id}/contract-records` | `{bindingIds,signedOn,reference}` → external contract fact; partner-finance OD-01 |
| retail POST `/deals/{id}/invoices` | `{purpose,amount:Money,recipientSnapshot,dueDate?}` → permitted invoice purpose; registration OD-07 |
| retail POST `/invoices/{id}/evidence` | `{claimedAmount:Money,paidOn,externalReference,attachmentBindingIds}` → submitted |
| retail POST `/evidence/{id}/accept`; `/reject` | `{confirmation:true}` / `{reason}` → factual review, no schedule allocation implied |
| retail POST `/deals/{id}/deliveries` | `{policyId,policyVersion,occurredAt,evidenceBindingIds}` →202 fulfillment intent/inventory finalize, scoped policy gates |
| financing POST `/programs`; `/programs/{id}/versions` | `{name,currency,terms,eligibility,calculationPolicyId,calculationPolicyVersion}` → draft/version; terms/eligibility validated by named program schema |
| financing POST `/programs/{id}/publish`; `/withdraw` | `{programVersion}` / `{reason}` → own-provider publication state, old snapshots unchanged |
| financing POST `/applications`; PATCH `/applications/{id}` | `{retailDealId,providerCompanyId,programId?,programVersion?,calculationInputs?}` / draft fields; provider change clears old program/calculation |
| financing POST `/applications/{id}/submit` | `{confirmation:true,dealRevision,calculationDigest,consentRecordRef?}` →202 submission protocol |
| financing POST `/applications/{id}/take` | `{}`; addressed provider submitted→review |
| financing POST `/applications/{id}/information-requests` | `{note}`; provider review→needs-info |
| financing POST `/applications/{id}/responses` | `{requestId,note,attachmentBindingIds}`; seller needs-info→review |
| financing POST `/applications/{id}/terms` | `{note,calculationInputs,calculationPolicyVersion}`; provider review→terms, authoritative recalculation+immutable TermsVersion |
| financing POST `/applications/{id}/counter` | `{termsVersion,note}`; seller terms→review |
| financing POST `/applications/{id}/agree` | `{termsVersion,confirmation:true}`; seller current terms→agreed, not an authenticated buyer signature |
| financing POST `/applications/{id}/decline` | `{reason}`; provider review→declined terminal in current slice |
| financing POST `/applications/{id}/document-requests` | `{title,requirements}`; provider agreed only →requested |
| financing POST `/document-requests/{id}/submissions` | `{attachmentBindingId,note?}`; seller requested/changes→review with immutable numbered submission |
| financing POST `/document-requests/{id}/accept`; `/return`; `/cancel` | `{submissionVersion,confirmation:true}` / `{submissionVersion,note}` / `{reason}`; provider review→accepted/changes, requested/changes→cancelled |
| insurance POST `/applications`; PATCH `/applications/{id}` | `{retailDealId,insurerCompanyId,note?}` / `{insurerCompanyId,note?}`; own eligible sale, draft-only edit |
| insurance POST `/applications/{id}/submit` | `{confirmation:true,dealRevision,consentRecordRef?}` →202 submission protocol |
| insurance POST `/applications/{id}/take` | `{}`; addressed insurer submitted→review |
| insurance POST `/applications/{id}/information-requests`; `/responses` | `{note}` / `{requestId,note,attachmentBindingIds}`; insurer review→needs-info / seller→review |
| insurance POST `/applications/{id}/approve`; `/decline` | `{note}`; insurer review→terminal decision, no coverage/payment/handover event |

Lead forward stages follow `new→contacted→qualified→test-drive→negotiation`; any nonterminal→lost requires reason. Won derives delivery; backward/reopen/skip-stage permissions are a scoped user rule, not inferred by the API.
Retail GET `/customers`, `/customers/{id}`, `/leads`, `/leads/{id}`, `/tasks`, `/deals`, `/deals/{id}`, `/listings`, `/invoices`, `/invoices/{id}`, `/deals/{id}/history`.
Finance GET `/programs`, `/programs/{id}`, `/applications`, `/applications/{id}`, `/applications/{id}/history`, `/applications/{id}/document-requests`; insurance analogous applications/detail/history.
Draft data is seller-only on list/detail/count/history/download, not just hidden in navigation. Provider sees only addressed applications after submission admission completes.
Invoice/evidence correction, overpayment allocation, installment servicing and bank funding require separately approved contracts, not a generic editable paid/status field.

## 5. Documents and contract generation

| Method/path under documents | Body / result |
|---|---|
| POST `/upload-intents` | `{ownerDomain,ownerObjectId,purpose,fileName,declaredMime,declaredBytes,sha256}` → `{uploadId,documentId,versionId,uploadUrl,expiresAt,requiredHeaders,limitsVersion}` plus standard receipt |
| POST `/upload-intents/{id}/complete` | `{}` + upload If-Match →202 byte finalization/scan operation; actual stored bytes checked |
| GET `/versions/{id}` | `{data:{id,documentId,number,fileName,detectedMime,byteLength,sha256,scanState,createdAt},revision,asOf}` with owner authorization, no storage key |
| POST `/versions/{id}/download-grants` | `{purpose}` → `{grantId,href,expiresAt}`; sensitive MFA/owner read grant |
| GET `/downloads/{grantId}` | authenticated same-session byte stream, owner reauthorization and immutable generation pin |

UploadUrl is an opaque staging-only capability; production provider/generation semantics, scan callback and binding are fixed by `cross-owner.md`. File clean is not domain accepted.
Published integration payloads for reservation/attachment/submission use the exact cross-owner document. Other facts follow `<owner>.<aggregate>.<past-tense-action>.v1` with ID/version references and changed safe fields; owner contract tasks must author concrete schemas/fixtures before consumers start.
Each owner owns OpenAPI3.1 and JSON Schema integration contracts in its sole contract directory; generated web/domain clients derive from them, never from slice prose or mock globals.
Contract task acceptance: required/unknown fields, integer-string precision, empty/null distinction, status/action guards, scope filtering, secret exclusions, replay-compatible versioning and typed success/error fixtures. No interface-consuming task becomes ready against an unspecified schema.
Changes to this approved baseline require coordinator ADR/contract revision and dependent fixture/client regeneration before parallel implementation; worker suggestions are not unilateral interface changes.

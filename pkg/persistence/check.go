package persistence

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"gorm.io/gorm"
)

const sharedBaseHash = "86ce41c5ec194a8bc255ae417b4506b518877fabb4d3e4fb6306858a0ab15d53"

var supportedShared = []struct{ filename, hash string }{
	{"pkg/eventstore/schema.sql", sharedBaseHash},
	{"pkg/eventstore/migrations/000002_messaging_delivery.up.sql", "1cb4db0d5a19490cfe7886fabfb8a681573b2a1ead411963a619f41fca127bc2"},
	{"pkg/eventstore/migrations/000003_messaging_route_compatibility.up.sql", "c198af498e39056a7a251b5906d6db050ad994e04588bf6cecf8316221ea85ec"},
	{"pkg/eventstore/migrations/000004_projection_checkpoint.up.sql", "41536429fb95d4b60a11866843a2ccbd6a4cf723cd919884933b6bc1d8202c4c"},
}

// Check performs SELECTs only on the supplied handle. It opens no transaction,
// retries nothing and never runs DDL or callback SQL. A UOW must pass its actual
// ReadCommitted transaction before binding repositories on EVERY Run. A startup
// check is not cached authority and cannot fence uncooperative administrative DDL.
// Custody and shared revisions beyond 4 deliberately await the separate checker.
func Check(ctx context.Context, db *gorm.DB, p Profile) error {
	if ctx == nil || db == nil || db.Config == nil || db.Error != nil || db.Statement == nil || p.data == nil {
		return ErrIncompatible
	}
	s := p.data.spec
	if s.Shared.Mode != "legacy" || len(s.Shared.Artifacts) > len(supportedShared) {
		return ErrUnsupported
	}
	for i, a := range s.Shared.Artifacts {
		if a.Filename != supportedShared[i].filename || digestHex(a.SHA256) != supportedShared[i].hash {
			return fmt.Errorf("%w: shared artifact", ErrIncompatible)
		}
	}
	db = db.WithContext(ctx)
	if err := require(db, `SELECT current_database()=? AND current_user=? AND session_user=?`, s.Database, s.RuntimeRole, s.RuntimeRole); err != nil {
		return err
	}
	if err := checkCatalog(db, s); err != nil {
		return err
	}
	if err := require(db, `SELECT count(*)=1 AND coalesce(bool_and(version=? AND NOT dirty),false) FROM public.schema_migrations`, s.Head); err != nil {
		return err
	}
	if err := require(db, fmt.Sprintf(`SELECT count(*)=1 AND coalesce(bool_and(singleton AND owner_service=? AND mechanics_version=1),false) FROM %s_mechanics.compatibility`, s.Owner), s.Owner); err != nil {
		return err
	}
	if err := require(db, `SELECT EXISTS(SELECT FROM pg_attrdef d JOIN pg_attribute a ON a.attrelid=d.adrelid AND a.attnum=d.adnum WHERE d.adrelid='eventstore.events'::regclass AND a.attname='owner_service' AND pg_get_expr(d.adbin,d.adrelid)=quote_literal(?)||'::text')`, s.Owner); err != nil {
		return err
	}
	if err := checkHistory(db, s); err != nil {
		return err
	}
	return checkLegacyShared(db, s)
}

func digestHex(d Digest) string { return hex.EncodeToString(d[:]) }
func require(db *gorm.DB, sql string, args ...any) error {
	var valid bool
	if err := db.Raw(sql, args...).Scan(&valid).Error; err != nil {
		return fmt.Errorf("%w: %w", ErrIncompatible, err)
	}
	if !valid {
		return ErrIncompatible
	}
	return nil
}

func checkHistory(db *gorm.DB, s Specification) error {
	b := s.Baseline
	if err := require(db, `SELECT count(*)=count(DISTINCT installation_request_id) FROM owner_migrations.artifacts`); err != nil {
		return err
	}
	if err := require(db, `SELECT count(*)=1 AND coalesce(bool_and(singleton AND owner_service=? AND database_name::text=? AND runtime_role::text=? AND format_revision=? AND template_sha256=decode(?,'hex') AND base_schema_sha256=decode(?,'hex') AND base_version=1 AND base_artifact_sha256=decode(?,'hex') AND baseline_kind=? AND evidence_ref=? AND approval_ref=? AND backup_ref=? AND stopped_runtimes_ref=? AND installation_request_id=?::uuid AND isfinite(installed_at)),false) FROM owner_migrations.compatibility`, s.Owner, s.Database, s.RuntimeRole, s.HistoryRevision, digestHex(s.HistorySHA256), sharedBaseHash, digestHex(s.Artifacts[0].Identity.SHA256), b.Kind, b.EvidenceRef, b.ApprovalRef, b.BackupRef, b.StoppedRuntimesRef, b.RequestID.String()); err != nil {
		return err
	}
	type receipt struct {
		Version         int64
		Filename        string
		SQLHash         string
		Predecessor     *int64
		PredecessorHash *string
		FeatureID       *string
		FeatureRevision *int64
		FeatureHash     *string
		ManifestHash    string
		ValidEvidence   bool
	}
	var rows []receipt
	err := db.Raw(`SELECT version,filename,encode(sql_sha256,'hex') AS sql_hash,predecessor_version AS predecessor,encode(predecessor_sha256,'hex') AS predecessor_hash,feature_contract_id AS feature_id,feature_contract_revision AS feature_revision,encode(feature_contract_sha256,'hex') AS feature_hash,encode(artifact_manifest_sha256,'hex') AS manifest_hash,
 installation_request_id<>'00000000-0000-0000-0000-000000000000'::uuid AND octet_length(installing_profile_sha256)=32 AND installing_profile_sha256<>decode(repeat('00',32),'hex') AND isfinite(completed_at) AND
 CASE WHEN version=1 THEN installation_request_id=?::uuid AND provenance_kind=? AND provenance_ref=? ELSE provenance_kind='verified-installation' AND length(btrim(provenance_ref)) BETWEEN 1 AND 2048 AND provenance_ref !~ '[[:cntrl:]]' END AS valid_evidence
 FROM owner_migrations.artifacts ORDER BY version`, b.RequestID.String(), b.Kind, b.EvidenceRef).Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("%w: %w", ErrIncompatible, err)
	}
	if len(rows) != len(s.Artifacts) {
		return ErrIncompatible
	}
	for i, row := range rows {
		a := s.Artifacts[i]
		if row.Version != a.Identity.Version || row.Filename != a.Identity.Filename || row.SQLHash != digestHex(a.Identity.SHA256) || row.ManifestHash != digestHex(a.ManifestSHA256) || !row.ValidEvidence {
			return ErrIncompatible
		}
		if i == 0 {
			if row.Predecessor != nil || row.PredecessorHash != nil || row.FeatureID != nil || row.FeatureRevision != nil || row.FeatureHash != nil {
				return ErrIncompatible
			}
		} else {
			previous := s.Artifacts[i-1].Identity
			if row.Predecessor == nil || *row.Predecessor != previous.Version || row.PredecessorHash == nil || *row.PredecessorHash != digestHex(previous.SHA256) || row.FeatureID == nil || *row.FeatureID != a.Feature.ID || row.FeatureRevision == nil || *row.FeatureRevision != int64(a.Feature.Revision) || row.FeatureHash == nil || *row.FeatureHash != digestHex(a.Feature.SHA256) {
				return ErrIncompatible
			}
		}
	}
	type marker struct {
		ID       string
		Revision int64
		Version  int64
		Hash     string
	}
	var got []marker
	if err := db.Raw(`SELECT contract_id AS id,contract_revision AS revision,migration_version AS version,encode(contract_sha256,'hex') AS hash FROM owner_migrations.feature_contracts ORDER BY migration_version`).Scan(&got).Error; err != nil {
		return fmt.Errorf("%w: %w", ErrIncompatible, err)
	}
	expected := make([]marker, 0, len(s.Artifacts)-1)
	for _, a := range s.Artifacts[1:] {
		expected = append(expected, marker{a.Feature.ID, int64(a.Feature.Revision), a.Identity.Version, digestHex(a.Feature.SHA256)})
	}
	if len(got) != len(expected) || !slices.Equal(got, expected) {
		return ErrIncompatible
	}
	return nil
}

func checkLegacyShared(db *gorm.DB, s Specification) error {
	// Exact presence, not maximum revision inference; undeclared known lineage
	// markers fail even if an older subset would otherwise be compatible.
	names := []string{"messaging_mode", "messaging_route_compatibility", "projection_checkpoint_compatibility", "quarantine_compatibility", "projection_generation_compatibility"}
	for i, name := range names {
		if err := require(db, `SELECT (to_regclass(?) IS NOT NULL)=?`, "eventstore."+name, i+2 <= len(s.Shared.Artifacts)); err != nil {
			return err
		}
	}
	if len(s.Shared.Artifacts) >= 2 {
		if err := require(db, `SELECT count(*)=1 AND coalesce(bool_and(singleton AND owner_service=? AND runtime_role::text=? AND schema_version=2 AND mode='legacy'),false) FROM eventstore.messaging_mode`, s.Owner, s.RuntimeRole); err != nil {
			return err
		}
	}
	if len(s.Shared.Artifacts) >= 3 {
		if err := require(db, `SELECT count(*)=1 AND coalesce(bool_and(singleton AND owner_service=? AND runtime_role::text=? AND base_schema_version=2 AND migration_revision=3 AND route_format_version=1 AND prior_migration_sha256=decode(?,'hex') AND correction_migration_sha256=decode(?,'hex') AND isfinite(installed_at)),false) FROM eventstore.messaging_route_compatibility`, s.Owner, s.RuntimeRole, supportedShared[1].hash, supportedShared[2].hash); err != nil {
			return err
		}
	}
	if len(s.Shared.Artifacts) >= 4 {
		if err := require(db, `SELECT count(*)=1 AND coalesce(bool_and(singleton AND owner_service=? AND runtime_role::text=? AND base_schema_version=2 AND migration_revision=4 AND feature_format_version=1 AND base_schema_sha256=decode(?,'hex') AND prior_migration_sha256=decode(?,'hex') AND correction_migration_sha256=decode(?,'hex') AND checkpoint_migration_sha256=decode(?,'hex') AND isfinite(installed_at)),false) FROM eventstore.projection_checkpoint_compatibility`, s.Owner, s.RuntimeRole, sharedBaseHash, supportedShared[1].hash, supportedShared[2].hash, supportedShared[3].hash); err != nil {
			return err
		}
	}
	return nil
}

type grantRecord struct {
	Schema        string   `json:"schema"`
	Table         string   `json:"tbl"`
	Select        bool     `json:"sel"`
	Insert        bool     `json:"ins"`
	Update        bool     `json:"upd"`
	Delete        bool     `json:"del"`
	SelectColumns []string `json:"sc"`
	InsertColumns []string `json:"ic"`
	UpdateColumns []string `json:"uc"`
}

func grants(s Specification) []grantRecord {
	var out []grantRecord
	add := func(schema, table string, insert bool, updates ...string) {
		out = append(out, grantRecord{schema, table, true, insert, false, false, []string{}, []string{}, updates})
	}
	add("public", "schema_migrations", false)
	add(s.Owner+"_mechanics", "compatibility", false)
	for _, name := range []string{"compatibility", "artifacts", "feature_contracts"} {
		add("owner_migrations", name, false)
	}
	for _, name := range []string{"events", "command_receipts", "inbox", "operation_steps"} {
		add("eventstore", name, true)
	}
	add("eventstore", "outbox", true, "attempts", "next_attempt_at", "lease_owner", "lease_until", "sent_at")
	add("eventstore", "operations", true, "phase", "decision", "attention_required", "result", "error", "attempts", "next_attempt_at", "lease_owner", "lease_until", "revision", "updated_at")
	if len(s.Shared.Artifacts) >= 2 {
		for _, name := range []string{"messaging_mode", "messaging_legacy_authorizations", "messaging_cutovers", "messaging_legacy_evidence"} {
			add("eventstore", name, false)
		}
		for _, name := range []string{"messaging_admissions", "outbox_messages", "dispatch_messages", "dispatch_enrollments"} {
			add("eventstore", name, true)
		}
		add("eventstore", "outbox_deliveries", true, "attempts", "next_attempt_at", "lease_owner", "lease_until", "sent_at", "hold_ref")
		add("eventstore", "dispatch_jobs", true, "attempts", "next_attempt_at", "lease_owner", "lease_until", "completed_at", "inbox_consumer_name", "inbox_event_id", "hold_ref", "quarantine_ref")
	}
	if len(s.Shared.Artifacts) >= 3 {
		add("eventstore", "messaging_route_compatibility", false)
	}
	if len(s.Shared.Artifacts) >= 4 {
		add("eventstore", "projection_checkpoint_compatibility", false)
		for _, name := range []string{"consumer_bootstraps", "consumer_gaps", "consumer_gap_attempts"} {
			add("eventstore", name, true)
		}
		add("eventstore", "consumer_checkpoints", true, "position", "last_event_id", "last_event_hash", "revision", "updated_at")
	}
	for _, f := range s.Features {
		for _, g := range f.Tables {
			out = append(out, grantRecord{g.Schema, g.Table, g.Select, g.Insert, g.Update, g.Delete, g.SelectColumns, g.InsertColumns, g.UpdateColumns})
		}
	}
	// JSON arrays, including empty sets, must never become SQL NULL permissions.
	for i := range out {
		for _, p := range []*[]string{&out[i].SelectColumns, &out[i].InsertColumns, &out[i].UpdateColumns} {
			if *p == nil {
				*p = []string{}
			}
		}
	}
	return out
}

func checkCatalog(db *gorm.DB, s Specification) error {
	g := grants(s)
	encoded, err := json.Marshal(g)
	if err != nil {
		return ErrIncompatible
	}
	if err := require(db, catalogSQL, string(encoded), s.Database, s.RuntimeRole); err != nil {
		return err
	}
	// The ledger's complete rowset must have its exact original two-column shape.
	if err := require(db, `SELECT count(*)=2 AND count(*) FILTER(WHERE attname='version' AND atttypid='bigint'::regtype AND attnotnull)=1 AND count(*) FILTER(WHERE attname='dirty' AND atttypid='boolean'::regtype AND attnotnull)=1 FROM pg_attribute WHERE attrelid='public.schema_migrations'::regclass AND attnum>0 AND NOT attisdropped`); err != nil {
		return err
	}
	// Named objects and ACLs above are readiness, not a full constraint dump.
	// Also require the actual history immutability/insertion guards remain enabled
	// with their expected owner-local functions. Feature constraints belong to the
	// feature contract/schema tests, not arbitrary profile-provided SQL predicates.
	type guard struct {
		Table    string
		Name     string
		Function string
		Kind     int16
	}
	expected := []guard{{"artifacts", "artifacts_immutable", "immutable", 58}, {"artifacts", "artifacts_insert", "artifact_guard", 7}, {"compatibility", "compatibility_immutable", "immutable", 58}, {"feature_contracts", "feature_contracts_immutable", "immutable", 58}, {"feature_contracts", "feature_contracts_insert", "feature_guard", 7}}
	var got []guard
	err = db.Raw(`SELECT c.relname AS table,t.tgname AS name,p.proname AS function,t.tgtype AS kind FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_proc p ON p.oid=t.tgfoid JOIN pg_namespace n ON n.oid=p.pronamespace WHERE c.relnamespace='owner_migrations'::regnamespace AND NOT t.tgisinternal AND t.tgenabled IN ('O','A') AND n.nspname='owner_migrations' AND NOT p.prosecdef AND p.proowner=(SELECT datdba FROM pg_database WHERE datname=current_database()) ORDER BY c.relname,t.tgname`).Scan(&got).Error
	if err != nil {
		return fmt.Errorf("%w: %w", ErrIncompatible, err)
	}
	if !reflect.DeepEqual(got, expected) {
		return ErrIncompatible
	}
	return nil
}

// Parameterized typed policies, never caller SQL. Inspect every SET ROLE
// reachable identity (including NOINHERIT), effective permissions, raw PUBLIC
// and grant-option ACLs, and defaults. Unknown user objects are not silently
// granted access simply because their schema contains an approved feature.
const catalogSQL = `WITH expected AS (
 SELECT * FROM jsonb_to_recordset(?::jsonb) AS e(schema text,tbl text,sel boolean,ins boolean,upd boolean,del boolean,sc text[],ic text[],uc text[])
), owner AS (SELECT oid FROM pg_roles WHERE rolname=?), runtime AS (SELECT oid FROM pg_roles WHERE rolname=?),
 roles AS (SELECT r.* FROM pg_roles r,runtime u WHERE pg_has_role(u.oid,r.oid,'MEMBER')),
 namespaces AS (SELECT * FROM pg_namespace WHERE nspname NOT LIKE 'pg\_%' ESCAPE '\' AND nspname<>'information_schema'),
 relations AS (SELECT c.*,n.nspname FROM pg_class c JOIN namespaces n ON n.oid=c.relnamespace WHERE c.relkind IN ('r','p','v','m','f','S'))
SELECT (SELECT count(*)=1 FROM owner) AND (SELECT count(*)=1 FROM runtime)
 AND (SELECT datdba=(SELECT oid FROM owner) FROM pg_database WHERE datname=current_database())
 AND has_database_privilege(current_user,current_database(),'CONNECT')
 AND NOT EXISTS(SELECT FROM roles r WHERE r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolreplication OR r.rolbypassrls OR r.oid=(SELECT oid FROM owner) OR r.rolname LIKE 'pg\_%' ESCAPE '\' OR has_database_privilege(r.oid,current_database(),'CREATE,TEMPORARY'))
 AND NOT EXISTS(SELECT FROM pg_database d CROSS JOIN LATERAL aclexplode(d.datacl) a WHERE d.datname=current_database() AND (a.grantee=0 OR (a.grantee IN (SELECT oid FROM roles) AND a.is_grantable)))
 AND NOT EXISTS(SELECT FROM pg_database d CROSS JOIN roles r WHERE d.datname LIKE 'justix\_%' ESCAPE '\' AND d.datname<>current_database() AND has_database_privilege(r.oid,d.oid,'CONNECT,CREATE,TEMPORARY'))
 AND NOT EXISTS(SELECT FROM expected e LEFT JOIN namespaces n ON n.nspname=e.schema WHERE n.oid IS NULL OR (n.nspname<>'public' AND n.nspowner<>(SELECT oid FROM owner)) OR NOT has_schema_privilege(current_user,n.oid,'USAGE'))
 AND NOT EXISTS(SELECT FROM namespaces n CROSS JOIN roles r WHERE has_schema_privilege(r.oid,n.oid,'CREATE') OR (has_schema_privilege(r.oid,n.oid,'USAGE') AND NOT EXISTS(SELECT FROM expected e WHERE e.schema=n.nspname)))
 AND NOT EXISTS(SELECT FROM namespaces n CROSS JOIN LATERAL aclexplode(n.nspacl) a WHERE (a.grantee=0 AND n.nspname<>'public') OR (a.grantee IN (SELECT oid FROM roles) AND a.is_grantable))
 AND NOT EXISTS(SELECT FROM expected e LEFT JOIN relations c ON c.nspname=e.schema AND c.relname=e.tbl WHERE c.oid IS NULL OR c.relkind<>'r' OR c.relowner<>(SELECT oid FROM owner) OR c.relrowsecurity OR c.relforcerowsecurity
   OR (e.sel AND NOT has_table_privilege(current_user,c.oid,'SELECT')) OR (e.ins AND NOT has_table_privilege(current_user,c.oid,'INSERT')) OR (e.upd AND NOT has_table_privilege(current_user,c.oid,'UPDATE')) OR (e.del AND NOT has_table_privilege(current_user,c.oid,'DELETE'))
   OR EXISTS(SELECT FROM unnest(e.sc) col WHERE NOT EXISTS(SELECT FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped AND a.attname=col AND has_column_privilege(current_user,c.oid,a.attnum,'SELECT')))
   OR EXISTS(SELECT FROM unnest(e.ic) col WHERE NOT EXISTS(SELECT FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped AND a.attname=col AND has_column_privilege(current_user,c.oid,a.attnum,'INSERT')))
   OR EXISTS(SELECT FROM unnest(e.uc) col WHERE NOT EXISTS(SELECT FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped AND a.attname=col AND has_column_privilege(current_user,c.oid,a.attnum,'UPDATE'))))
 AND NOT EXISTS(SELECT FROM relations c CROSS JOIN roles r LEFT JOIN expected e ON e.schema=c.nspname AND e.tbl=c.relname WHERE c.relkind<>'S' AND (
   has_table_privilege(r.oid,c.oid,'TRUNCATE,TRIGGER,REFERENCES,MAINTAIN') OR (has_table_privilege(r.oid,c.oid,'SELECT') AND NOT coalesce(e.sel,false)) OR (has_table_privilege(r.oid,c.oid,'INSERT') AND NOT coalesce(e.ins,false)) OR (has_table_privilege(r.oid,c.oid,'UPDATE') AND NOT coalesce(e.upd,false)) OR (has_table_privilege(r.oid,c.oid,'DELETE') AND NOT coalesce(e.del,false))))
 AND NOT EXISTS(SELECT FROM relations c CROSS JOIN roles r WHERE c.relkind='S' AND has_sequence_privilege(r.oid,c.oid,'SELECT,UPDATE,USAGE'))
 AND NOT EXISTS(SELECT FROM relations c CROSS JOIN roles r JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped LEFT JOIN expected e ON e.schema=c.nspname AND e.tbl=c.relname WHERE c.relkind<>'S' AND (
   has_column_privilege(r.oid,c.oid,a.attnum,'REFERENCES') OR (has_column_privilege(r.oid,c.oid,a.attnum,'SELECT') AND NOT coalesce(e.sel OR a.attname=ANY(e.sc),false)) OR (has_column_privilege(r.oid,c.oid,a.attnum,'INSERT') AND NOT coalesce(e.ins OR a.attname=ANY(e.ic),false)) OR (has_column_privilege(r.oid,c.oid,a.attnum,'UPDATE') AND NOT coalesce(e.upd OR a.attname=ANY(e.uc),false))))
 AND NOT EXISTS(SELECT FROM relations c CROSS JOIN LATERAL aclexplode(c.relacl) a WHERE a.grantee=0 OR (a.grantee IN (SELECT oid FROM roles) AND a.is_grantable))
 AND NOT EXISTS(SELECT FROM relations c JOIN pg_attribute col ON col.attrelid=c.oid CROSS JOIN LATERAL aclexplode(col.attacl) a WHERE a.grantee=0 OR (a.grantee IN (SELECT oid FROM roles) AND a.is_grantable))
 AND NOT EXISTS(SELECT FROM pg_proc p JOIN namespaces n ON n.oid=p.pronamespace CROSS JOIN roles r WHERE has_function_privilege(r.oid,p.oid,'EXECUTE'))
 AND NOT EXISTS(SELECT FROM pg_default_acl d CROSS JOIN LATERAL aclexplode(d.defaclacl) a WHERE d.defaclrole=(SELECT oid FROM owner) AND d.defaclobjtype IN ('r','f','S','n') AND a.grantee<>(SELECT oid FROM owner))`

package model

import "justixauto/internal/pkg/auth"

const (
	PermRead              = "documents.read"
	PermUpload            = "documents.upload"
	PermSensitiveDownload = "documents.sensitive.download" // needs fresh MFA
)

// Permissions are registered in the identity catalog at startup.
var Permissions = []auth.PermissionInfo{
	{Key: PermRead, Scope: "company", Assignable: true},
	{Key: PermUpload, Scope: "company", Assignable: true},
	{Key: PermSensitiveDownload, Scope: "company", RequiresMFA: true, Assignable: true},
}

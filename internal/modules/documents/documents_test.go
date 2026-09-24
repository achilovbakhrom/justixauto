package documents_test

import (
	"bytes"
	"net/http"
	"testing"

	"justixauto/internal/e2e"
	"justixauto/internal/modules/documents"
)

func TestUploadAccessAndSensitivity(t *testing.T) {
	e := e2e.New(t)
	admin := e.Admin()
	a := e.CompanyUser(admin, "Alpha", documents.PermRead, documents.PermUpload, documents.PermSensitiveDownload)
	b := e.CompanyUser(admin, "Beta", documents.PermRead)

	e2e.Expect(t, a.Upload("other", "notes.txt", []byte("plain text")), http.StatusUnprocessableEntity)
	e2e.Expect(t, a.Upload("guess", "x.pdf", e2e.PDF), http.StatusUnprocessableEntity)
	e2e.Expect(t, b.Upload("other", "x.pdf", e2e.PDF), http.StatusForbidden)
	up := a.Upload("other", "../../etc/contract.pdf", e2e.PDF)
	e2e.Expect(t, up, http.StatusCreated)
	id := up.Data()["id"].(string)
	if up.Data()["fileName"] != "contract.pdf" || up.Data()["mime"] != "application/pdf" || up.Data()["storageKey"] != nil {
		t.Fatalf("metadata: %v", up.Data())
	}

	status, body := a.Raw("/documents/files/" + id + "/content")
	if status != http.StatusOK || !bytes.Equal(body, e2e.PDF) {
		t.Fatalf("download: %d %q", status, body)
	}
	// Other companies see nothing unless a module shares the file.
	e2e.Expect(t, b.Do(http.MethodGet, "/documents/files/"+id, nil), http.StatusNotFound)
	if status, _ := b.Raw("/documents/files/" + id + "/content"); status != http.StatusNotFound {
		t.Fatalf("foreign download: %d", status)
	}

	// Sensitive files need a fresh second factor to download.
	sens := a.Upload("finance-document", "passport.pdf", e2e.PDF)
	e2e.Expect(t, sens, http.StatusCreated)
	if status, _ := a.Raw("/documents/files/" + sens.Data()["id"].(string) + "/content"); status != http.StatusForbidden {
		t.Fatalf("sensitive download without MFA: %d", status)
	}
	a.EnrollMFA()
	if status, _ := a.Raw("/documents/files/" + sens.Data()["id"].(string) + "/content"); status != http.StatusOK {
		t.Fatalf("sensitive download with MFA: %d", status)
	}
}

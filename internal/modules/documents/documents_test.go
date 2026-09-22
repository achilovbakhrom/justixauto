package documents_test

import (
	"bytes"
	"net/http"
	"testing"

	"justixauto/internal/modules/documents"
	"justixauto/internal/testkit"
)

func TestUploadAccessAndSensitivity(t *testing.T) {
	e := testkit.New(t)
	admin := e.Admin()
	a := e.CompanyUser(admin, "Alpha", documents.PermRead, documents.PermUpload, documents.PermSensitiveDownload)
	b := e.CompanyUser(admin, "Beta", documents.PermRead)

	testkit.Expect(t, a.Upload("other", "notes.txt", []byte("plain text")), http.StatusUnprocessableEntity)
	testkit.Expect(t, a.Upload("guess", "x.pdf", testkit.PDF), http.StatusUnprocessableEntity)
	testkit.Expect(t, b.Upload("other", "x.pdf", testkit.PDF), http.StatusForbidden)
	up := a.Upload("other", "../../etc/contract.pdf", testkit.PDF)
	testkit.Expect(t, up, http.StatusCreated)
	id := up.Data()["id"].(string)
	if up.Data()["fileName"] != "contract.pdf" || up.Data()["mime"] != "application/pdf" || up.Data()["storageKey"] != nil {
		t.Fatalf("metadata: %v", up.Data())
	}

	status, body := a.Raw("/documents/files/" + id + "/content")
	if status != http.StatusOK || !bytes.Equal(body, testkit.PDF) {
		t.Fatalf("download: %d %q", status, body)
	}
	// Other companies see nothing unless a module shares the file.
	testkit.Expect(t, b.Do(http.MethodGet, "/documents/files/"+id, nil), http.StatusNotFound)
	if status, _ := b.Raw("/documents/files/" + id + "/content"); status != http.StatusNotFound {
		t.Fatalf("foreign download: %d", status)
	}

	// Sensitive files need a fresh second factor to download.
	sens := a.Upload("finance-document", "passport.pdf", testkit.PDF)
	testkit.Expect(t, sens, http.StatusCreated)
	if status, _ := a.Raw("/documents/files/" + sens.Data()["id"].(string) + "/content"); status != http.StatusForbidden {
		t.Fatalf("sensitive download without MFA: %d", status)
	}
	a.EnrollMFA()
	if status, _ := a.Raw("/documents/files/" + sens.Data()["id"].(string) + "/content"); status != http.StatusOK {
		t.Fatalf("sensitive download with MFA: %d", status)
	}
}

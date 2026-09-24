// Package idempotency makes retried POST requests safe. A client sends an
// Idempotency-Key (UUID) with every create/action; a retry with the same key
// and body replays the first successful response instead of acting twice.
package idempotency

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
)

// abandonAfter: an in-progress key older than this is assumed to belong to a
// crashed request and may be taken over by a retry.
const abandonAfter = 5 * time.Minute

type record struct {
	ActorID      string `gorm:"primaryKey;type:uuid"`
	Key          string `gorm:"primaryKey;type:uuid"`
	RequestHash  []byte
	Status       string
	ResponseCode *int
	ResponseEtag string
	ResponseBody []byte
	CreatedAt    time.Time
	CompletedAt  *time.Time
}

func (record) TableName() string { return "platform.idempotency_keys" }

// Middleware requires Idempotency-Key on authenticated POST requests, except
// paths containing one of the exempt fragments (authentication handshakes).
// Only 2xx responses are stored; errors release the key for a corrected retry.
func Middleware(db *gorm.DB, now func() time.Time, exempt ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := auth.Get(c)
			req := c.Request()
			if req.Method != http.MethodPost || p == nil || isExempt(req.URL.Path, exempt) {
				return next(c)
			}
			key, err := requireKey(req)
			if err != nil {
				return err
			}
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest)
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
			hash := requestHash(req.Method, req.URL.RequestURI(), body)

			ctx := req.Context()
			rec := record{ActorID: p.UserID, Key: key, RequestHash: hash, Status: "in_progress", CreatedAt: now().UTC()}
			res := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rec)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				replayed, err := resume(c, db, rec, now)
				if replayed || err != nil {
					return err
				}
			}
			return runAndRecord(c, db, now, next, p.UserID, key)
		}
	}
}

// requireKey validates the Idempotency-Key header on a request that needs one.
func requireKey(req *http.Request) (string, error) {
	key := req.Header.Get("Idempotency-Key")
	if key == "" {
		return "", apperr.New(apperr.ErrPreconditionRequired, "idempotency_key_required", "Idempotency-Key header is required")
	}
	if uuid.Validate(key) != nil {
		return "", apperr.FieldError("Idempotency-Key", "must be a UUID")
	}
	return key, nil
}

// runAndRecord calls the handler, capturing its response, then stores a
// completed record for a 2xx response or releases the key for any other
// outcome so a corrected retry can reuse it.
func runAndRecord(c echo.Context, db *gorm.DB, now func() time.Time, next echo.HandlerFunc, actorID, key string) error {
	capture := &captureWriter{ResponseWriter: c.Response().Writer}
	c.Response().Writer = capture
	err := next(c)
	status := c.Response().Status
	where := db.WithContext(c.Request().Context()).Model(&record{}).Where("actor_id = ? AND key = ?", actorID, key)
	if err == nil && status >= 200 && status < 300 {
		done := now().UTC()
		return where.Updates(map[string]any{
			"status": "completed", "response_code": status,
			"response_etag": c.Response().Header().Get("ETag"), "response_body": capture.body.Bytes(),
			"completed_at": done,
		}).Error
	}
	if delErr := where.Delete(&record{}).Error; delErr != nil {
		return errors.Join(err, delErr)
	}
	return err
}

// resume handles a key that already exists: replay a completed response, or
// take over an abandoned attempt. It reports whether a response was written.
func resume(c echo.Context, db *gorm.DB, rec record, now func() time.Time) (bool, error) {
	var existing record
	q := db.WithContext(c.Request().Context()).Where("actor_id = ? AND key = ?", rec.ActorID, rec.Key)
	if err := q.Take(&existing).Error; err != nil {
		return false, err
	}
	if !bytes.Equal(existing.RequestHash, rec.RequestHash) {
		return false, apperr.New(apperr.ErrConflict, "idempotency_key_reused", "this Idempotency-Key was used for a different request")
	}
	if existing.Status == "completed" {
		h := c.Response().Header()
		h.Set("Idempotent-Replayed", "true")
		if existing.ResponseEtag != "" {
			h.Set("ETag", existing.ResponseEtag)
		}
		return true, c.Blob(*existing.ResponseCode, echo.MIMEApplicationJSON, existing.ResponseBody)
	}
	if now().UTC().Sub(existing.CreatedAt) < abandonAfter {
		return false, apperr.New(apperr.ErrConflict, "request_in_progress", "the same request is still being processed")
	}
	res := q.Model(&record{}).Where("status = 'in_progress' AND created_at = ?", existing.CreatedAt).
		Update("created_at", now().UTC())
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, apperr.New(apperr.ErrConflict, "request_in_progress", "the same request is still being processed")
	}
	return false, nil
}

func isExempt(path string, exempt []string) bool {
	for _, e := range exempt {
		if strings.Contains(path, e) {
			return true
		}
	}
	return false
}

func requestHash(method, uri string, body []byte) []byte {
	h := sha256.New()
	h.Write([]byte(method + " " + uri + "\n"))
	h.Write(body)
	return h.Sum(nil)
}

type captureWriter struct {
	http.ResponseWriter
	body bytes.Buffer
}

func (w *captureWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

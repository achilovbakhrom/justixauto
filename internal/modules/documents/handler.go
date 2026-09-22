package documents

import (
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	"justixauto/internal/platform/apperr"
	"justixauto/internal/platform/auth"
	"justixauto/internal/platform/httpx"
)

type Handler struct{ s *Service }

func (h *Handler) Routes(g *echo.Group) {
	c := g.Group("", auth.RequireCompany())
	c.POST("/files", h.upload, auth.Require(PermUpload))
	c.GET("/files/:id", h.get, auth.Require(PermRead))
	c.GET("/files/:id/content", h.content, auth.Require(PermRead))
}

// upload takes multipart/form-data with fields "purpose" and "file".
func (h *Handler) upload(c echo.Context) error {
	req := c.Request()
	req.Body = http.MaxBytesReader(c.Response(), req.Body, MaxBytes+1<<20) // file plus form overhead
	fh, err := c.FormFile("file")
	if err != nil {
		return apperr.FieldError("file", "attach a file")
	}
	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	file, err := h.s.Upload(c.Request().Context(), auth.Get(c), c.FormValue("purpose"), fh.Filename, f)
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusCreated, FileInfo(file), 1)
}

func (h *Handler) get(c echo.Context) error {
	f, err := h.s.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, FileInfo(f), 1)
}

func (h *Handler) content(c echo.Context) error {
	f, r, err := h.s.Open(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	defer r.Close()
	hd := c.Response().Header()
	hd.Set("Content-Type", f.MIME)
	hd.Set("Content-Length", strconv.FormatInt(f.ByteLength, 10))
	hd.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.FileName}))
	hd.Set("X-Content-Type-Options", "nosniff")
	c.Response().WriteHeader(http.StatusOK)
	_, err = io.Copy(c.Response(), r)
	return err
}

type Module struct {
	handler *Handler
	Service *Service
}

func New(db *gorm.DB, now func() time.Time, storage Storage) *Module {
	if now == nil {
		now = time.Now
	}
	s := &Service{repo: &repository{db}, storage: storage, now: now}
	return &Module{handler: &Handler{s}, Service: s}
}

// Register mounts the document routes under /api/v1/documents.
func (m *Module) Register(api *echo.Group) { m.handler.Routes(api.Group("/documents")) }

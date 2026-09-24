package handler

import (
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"justixauto/internal/modules/documents/model"
	"justixauto/internal/modules/documents/service"
	"justixauto/internal/pkg/apperr"
	"justixauto/internal/pkg/auth"
	"justixauto/internal/pkg/httpx"
)

// Handler holds the service behind the documents HTTP routes.
type Handler struct{ s *service.Service }

// New builds the documents handler from its service.
func New(s *service.Service) *Handler { return &Handler{s} }

func (h *Handler) Routes(g *echo.Group) {
	c := g.Group("", auth.RequireCompany())
	c.POST("/files", h.upload, auth.Require(model.PermUpload))
	c.GET("/files/:id", h.get, auth.Require(model.PermRead))
	c.GET("/files/:id/content", h.content, auth.Require(model.PermRead))
}

// upload takes multipart/form-data with fields "purpose" and "file".
//
//	@Summary	Upload file
//	@Tags		documents
//	@Security	CSRF
//	@Accept		mpfd
//	@Param		Idempotency-Key	header		string	true	"retry key"
//	@Param		purpose			formData	string	true	"file purpose"
//	@Param		file			formData	file	true	"file contents"
//	@Success	201				{object}	httpx.DataEnvelope[handler.FileView]
//	@Failure	401,403,413,422	{object}	httpx.ErrorBody
//	@Router		/documents/files [post]
func (h *Handler) upload(c echo.Context) error {
	req := c.Request()
	req.Body = http.MaxBytesReader(c.Response(), req.Body, service.MaxBytes+1<<20) // file plus form overhead
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
	return httpx.Data(c, http.StatusCreated, toFileView(file), 1)
}

// get returns a file's metadata.
//
//	@Summary	Get file metadata
//	@Tags		documents
//	@Param		id			path		string	true	"file ID"
//	@Success	200			{object}	httpx.DataEnvelope[handler.FileView]
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/documents/files/{id} [get]
func (h *Handler) get(c echo.Context) error {
	f, err := h.s.Get(c.Request().Context(), auth.Get(c), c.Param("id"))
	if err != nil {
		return err
	}
	return httpx.Data(c, http.StatusOK, toFileView(f), 1)
}

// content streams the file's raw bytes.
//
//	@Summary	Download file content
//	@Tags		documents
//	@Produce	octet-stream
//	@Param		id			path		string	true	"file ID"
//	@Success	200			{file}		file
//	@Failure	401,403,404	{object}	httpx.ErrorBody
//	@Router		/documents/files/{id}/content [get]
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

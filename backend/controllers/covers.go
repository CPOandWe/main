package controllers

import (
	"backend/middleware"
	"backend/models"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

const maxCoverSize = 2 << 20 // 2 MB

var coverExts = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}

// ponytail: local disk; swap for S3 when file serving moves out of the backend.
func coversDir() string {
	if d := os.Getenv("COVERS_DIR"); d != "" {
		return d
	}
	return "uploads/covers"
}

func removeCover(name string) {
	if name != "" {
		os.Remove(filepath.Join(coversDir(), name))
	}
}

// authorizeEdit: owner while the book is unpublished, moderators always.
// Sends 403/404 itself and returns false when not allowed.
func (ctrl *BookController) authorizeEdit(c *gin.Context, b bookRow) bool {
	owner := b.uploadedBy == c.GetString(middleware.UserIDKey)
	if (owner && !b.isPublic) || ctrl.isModerator(c) {
		return true
	}
	if !owner && !b.isPublic {
		c.JSON(404, models.ErrorResponse{Message: "Book not found"})
	} else {
		c.JSON(403, models.ErrorResponse{Message: "Forbidden"})
	}
	return false
}

// @Summary Загрузка обложки
// @Description multipart/form-data, поле cover: JPEG, PNG или WebP до 2 МБ. Права как у редактирования книги. Заменяет прежнюю обложку
// @Tags Books
// @Accept mpfd
// @Param id path string true "ID книги"
// @Param cover formData file true "Файл обложки"
// @Success 204 "Обложка сохранена"
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 413 {object} models.ErrorResponse
// @Router /books/{id}/cover [put]
func (ctrl *BookController) UploadCover(c *gin.Context) {
	id, b, ok := ctrl.loadBook(c)
	if !ok || !ctrl.authorizeEdit(c, b) {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCoverSize+1<<20) // + multipart overhead
	fh, err := c.FormFile("cover")
	if err != nil {
		c.JSON(400, models.ErrorResponse{Message: "cover file is required"})
		return
	}
	if fh.Size > maxCoverSize {
		c.JSON(413, models.ErrorResponse{Message: "Cover is too large (max 2 MB)"})
		return
	}

	f, err := fh.Open()
	if err != nil {
		internalErr(c, err)
		return
	}
	defer f.Close()

	// trust the content, not the client's Content-Type / file name
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	ext, ok := coverExts[http.DetectContentType(head[:n])]
	if !ok {
		c.JSON(400, models.ErrorResponse{Message: "Cover must be JPEG, PNG or WebP"})
		return
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		internalErr(c, err)
		return
	}

	if err := os.MkdirAll(coversDir(), 0o755); err != nil {
		internalErr(c, err)
		return
	}
	name := id + ext
	dst, err := os.Create(filepath.Join(coversDir(), name))
	if err != nil {
		internalErr(c, err)
		return
	}
	_, err = io.Copy(dst, f)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		internalErr(c, err)
		return
	}

	if _, err := ctrl.pool.Exec(c.Request.Context(),
		`UPDATE books SET cover_path = $2 WHERE book_id = $1`, id, name); err != nil {
		internalErr(c, err)
		return
	}
	if b.coverPath != name { // previous cover had another extension
		removeCover(b.coverPath)
	}

	c.Status(204)
}

// @Summary Обложка книги
// @Description Видимость как у самой книги
// @Tags Books
// @Produce image/jpeg,image/png,image/webp
// @Param id path string true "ID книги"
// @Success 200 {file} binary
// @Failure 404 {object} models.ErrorResponse "Book or cover not found"
// @Router /books/{id}/cover [get]
func (ctrl *BookController) GetCover(c *gin.Context) {
	_, b, ok := ctrl.loadBook(c)
	if !ok {
		return
	}
	if !ctrl.canView(c, b) {
		c.JSON(404, models.ErrorResponse{Message: "Book not found"})
		return
	}
	if b.coverPath == "" {
		c.JSON(404, models.ErrorResponse{Message: "Cover not found"})
		return
	}

	c.Header("X-Content-Type-Options", "nosniff")
	c.File(filepath.Join(coversDir(), b.coverPath))
}

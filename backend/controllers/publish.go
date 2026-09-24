package controllers

import (
	"backend/middleware"
	"backend/models"
	"backend/utils"
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const maxPDFSize = 50 << 20 // 50 MB

// ponytail: local disk; swap for S3 when file serving moves out of the backend.
func booksDir() string {
	if d := os.Getenv("BOOKS_DIR"); d != "" {
		return d
	}
	return "uploads/books"
}

func removeBookFile(name string) {
	if name != "" {
		os.Remove(filepath.Join(booksDir(), name))
	}
}

// canView: public books for everyone, private ones for the owner and moderators.
func (ctrl *BookController) canView(c *gin.Context, b bookRow) bool {
	return b.isPublic || b.uploadedBy == c.GetString(middleware.UserIDKey) || ctrl.isModerator(c)
}

// @Summary Загрузка PDF книги
// @Description multipart/form-data, поле file: PDF до 50 МБ. Права как у редактирования книги. Заменяет прежний файл
// @Tags Books
// @Accept mpfd
// @Param id path string true "ID книги"
// @Param file formData file true "PDF-файл"
// @Success 204 "Файл сохранён"
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 413 {object} models.ErrorResponse
// @Router /books/{id}/file [put]
func (ctrl *BookController) UploadFile(c *gin.Context) {
	id, b, ok := ctrl.loadBook(c)
	if !ok || !ctrl.authorizeEdit(c, b) {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPDFSize+1<<20) // + multipart overhead
	fh, err := c.FormFile("file")
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || (err == nil && fh.Size > maxPDFSize) {
		c.JSON(413, models.ErrorResponse{Message: "File is too large (max 50 MB)"})
		return
	}
	if err != nil {
		c.JSON(400, models.ErrorResponse{Message: "file is required"})
		return
	}

	f, err := fh.Open()
	if err != nil {
		internalErr(c, err)
		return
	}
	defer f.Close()

	// trust the content, not the client's Content-Type / file name
	magic := make([]byte, 5)
	if _, err := io.ReadFull(f, magic); err != nil || !bytes.Equal(magic, []byte("%PDF-")) {
		c.JSON(400, models.ErrorResponse{Message: "File must be a PDF"})
		return
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		internalErr(c, err)
		return
	}

	if err := os.MkdirAll(booksDir(), 0o755); err != nil {
		internalErr(c, err)
		return
	}
	name := id + ".pdf"
	dst, err := os.Create(filepath.Join(booksDir(), name))
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
		`UPDATE books SET file_path = $2, file_size = $3 WHERE book_id = $1`, id, name, fh.Size); err != nil {
		internalErr(c, err)
		return
	}

	c.Status(204)
}

// @Summary Скачивание PDF
// @Description Только для авторизованных. Видимость как у самой книги
// @Tags Books
// @Produce application/pdf
// @Param id path string true "ID книги"
// @Success 200 {file} binary
// @Failure 401 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse "Book or file not found"
// @Router /books/{id}/download [get]
func (ctrl *BookController) Download(c *gin.Context) {
	id, b, ok := ctrl.loadBook(c)
	if !ok {
		return
	}
	if !ctrl.canView(c, b) {
		c.JSON(404, models.ErrorResponse{Message: "Book not found"})
		return
	}
	if b.filePath == "" {
		c.JSON(404, models.ErrorResponse{Message: "File not found"})
		return
	}

	c.Header("X-Content-Type-Options", "nosniff")
	c.FileAttachment(filepath.Join(booksDir(), b.filePath), id+".pdf")
}

// @Summary Статус книги
// @Description draft — нет заявки, pending/approved/rejected — по последней заявке (при rejected есть reason), published — книга в каталоге. Видимость как у книги
// @Tags Books
// @Produce json
// @Param id path string true "ID книги"
// @Success 200 {object} models.BookStatusResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /books/{id}/status [get]
func (ctrl *BookController) Status(c *gin.Context) {
	id, b, ok := ctrl.loadBook(c)
	if !ok {
		return
	}
	if !ctrl.canView(c, b) {
		c.JSON(404, models.ErrorResponse{Message: "Book not found"})
		return
	}
	if b.isPublic {
		c.JSON(200, models.BookStatusResponse{Status: "published"})
		return
	}

	res := models.BookStatusResponse{Status: "draft"}
	err := ctrl.pool.QueryRow(c.Request.Context(),
		`SELECT status, reason FROM book_add_requests WHERE book_id = $1 ORDER BY created_at DESC LIMIT 1`, id,
	).Scan(&res.Status, &res.Reason)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		internalErr(c, err)
		return
	}

	c.JSON(200, res)
}

// @Summary Заявка на публикацию
// @Description Владелец подаёт заявку на публикацию своей книги; PDF уже должен быть загружен
// @Tags Book requests
// @Produce json
// @Param id path string true "ID книги"
// @Success 201 {object} models.BookStatusResponse
// @Failure 400 {object} models.ErrorResponse "No PDF uploaded"
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse "Already published or request already pending"
// @Router /books/{id}/publish-request [post]
func (ctrl *BookController) RequestPublish(c *gin.Context) {
	id, b, ok := ctrl.loadBook(c)
	if !ok {
		return
	}
	me := c.GetString(middleware.UserIDKey)
	if b.uploadedBy != me {
		if ctrl.canView(c, b) {
			c.JSON(403, models.ErrorResponse{Message: "Forbidden"})
		} else {
			c.JSON(404, models.ErrorResponse{Message: "Book not found"})
		}
		return
	}
	if b.isPublic {
		c.JSON(409, models.ErrorResponse{Message: "Book is already published"})
		return
	}
	if b.filePath == "" {
		c.JSON(400, models.ErrorResponse{Message: "Upload the PDF first"})
		return
	}

	_, err := ctrl.pool.Exec(c.Request.Context(),
		`INSERT INTO book_add_requests (user_id, book_id, created_at) VALUES ($1, $2, now())`, me, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // partial unique index: one pending per book
			c.JSON(409, models.ErrorResponse{Message: "Request is already pending"})
			return
		}
		internalErr(c, err)
		return
	}

	c.JSON(201, models.BookStatusResponse{Status: "pending"})
}

// @Summary Прямая публикация
// @Description Модератор публикует книгу без заявки (закрывает открытую заявку как approved)
// @Tags Book requests
// @Param id path string true "ID книги"
// @Success 204 "Опубликовано"
// @Failure 400 {object} models.ErrorResponse "No PDF uploaded"
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /books/{id}/publish [post]
func (ctrl *BookController) Publish(c *gin.Context) {
	id, b, ok := ctrl.loadBook(c)
	if !ok {
		return
	}
	if b.filePath == "" {
		c.JSON(400, models.ErrorResponse{Message: "Upload the PDF first"})
		return
	}

	ctx := c.Request.Context()
	tx, err := ctrl.pool.Begin(ctx)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE books SET is_public = TRUE WHERE book_id = $1`, id); err != nil {
		internalErr(c, err)
		return
	}
	if _, err := tx.Exec(ctx,
		`UPDATE book_add_requests SET status = 'approved', reviewed_by = $2, reviewed_at = now()
		 WHERE book_id = $1 AND status = 'pending'`, id, c.GetString(middleware.UserIDKey)); err != nil {
		internalErr(c, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internalErr(c, err)
		return
	}

	c.Status(204)
}

// @Summary Заявки на публикацию
// @Description Открытые заявки, старые первыми (только Moderator+)
// @Tags Book requests
// @Produce json
// @Success 200 {array} models.ModerationRequest
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /moderation/requests [get]
func (ctrl *BookController) ModerationList(c *gin.Context) {
	rows, err := ctrl.pool.Query(c.Request.Context(),
		`SELECT r.request_id, b.book_id, b.title, b.description, b.is_public,
		        a.first_name || ' ' || a.last_name AS author
		 FROM book_add_requests r
		 JOIN books b USING (book_id)
		 JOIN authors a USING (author_id)
		 WHERE r.status = 'pending'
		 ORDER BY r.created_at`)
	if err != nil {
		internalErr(c, err)
		return
	}
	reqs, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.ModerationRequest])
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(200, reqs)
}

// @Summary Решение по заявке
// @Description approved публикует книгу, rejected требует reason (только Moderator+)
// @Tags Book requests
// @Accept json
// @Produce json
// @Param id path string true "ID заявки"
// @Param request body models.ReviewRequestBody true "Решение"
// @Success 200 {object} models.ReviewRequestResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse "Pending request not found"
// @Router /moderation/requests/{id} [patch]
func (ctrl *BookController) ModerationReview(c *gin.Context) {
	id := c.Param("id")
	if validate.Var(id, "uuid") != nil {
		c.JSON(404, models.ErrorResponse{Message: "Request not found"})
		return
	}

	var body models.ReviewRequestBody
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid request"})
		return
	}
	if err := validate.Struct(body); err != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid request", Errors: utils.FormatValidationError(err)})
		return
	}
	if body.Status == "rejected" && body.Reason == "" {
		c.JSON(400, models.ErrorResponse{Message: "reason is required when rejecting"})
		return
	}

	ctx := c.Request.Context()
	tx, err := ctrl.pool.Begin(ctx)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer tx.Rollback(ctx)

	var bookID string
	err = tx.QueryRow(ctx,
		`UPDATE book_add_requests
		 SET status = $2, reason = NULLIF($3, ''), reviewed_by = $4, reviewed_at = now()
		 WHERE request_id = $1 AND status = 'pending' RETURNING book_id`,
		id, body.Status, body.Reason, c.GetString(middleware.UserIDKey)).Scan(&bookID)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(404, models.ErrorResponse{Message: "Pending request not found"})
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}

	if body.Status == "approved" {
		if _, err := tx.Exec(ctx, `UPDATE books SET is_public = TRUE WHERE book_id = $1`, bookID); err != nil {
			internalErr(c, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(200, models.ReviewRequestResponse{Status: body.Status, Reason: body.Reason})
}

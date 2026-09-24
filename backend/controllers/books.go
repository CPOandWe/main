package controllers

import (
	"backend/middleware"
	"backend/models"
	"backend/utils"
	"errors"
	"log"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const booksPageSize = 20

type BookController struct {
	pool *pgxpool.Pool
}

func NewBookController(pool *pgxpool.Pool) *BookController {
	return &BookController{pool: pool}
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func internalErr(c *gin.Context, err error) {
	log.Println(err)
	c.JSON(500, models.ErrorResponse{Message: "Something went wrong, try again~"})
}

func uuidQuery(c *gin.Context, name string) (val *string, ok bool) {
	v := c.Query(name)
	if v == "" {
		return nil, true
	}
	if validate.Var(v, "uuid") != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid " + name})
		return nil, false
	}
	return &v, true
}

func (ctrl *BookController) isModerator(c *gin.Context) bool {
	var ok bool
	err := ctrl.pool.QueryRow(c.Request.Context(),
		`SELECT r.name IN ('moderator', 'admin') FROM users u JOIN roles r USING (role_id) WHERE u.user_id = $1`,
		c.GetString(middleware.UserIDKey),
	).Scan(&ok)
	return err == nil && ok
}

type bookRow struct {
	uploadedBy string
	isPublic   bool
}

func (ctrl *BookController) loadBook(c *gin.Context) (id string, b bookRow, ok bool) {
	id = c.Param("id")
	if validate.Var(id, "uuid") != nil {
		c.JSON(404, models.ErrorResponse{Message: "Book not found"})
		return
	}
	err := ctrl.pool.QueryRow(c.Request.Context(),
		`SELECT uploaded_by, is_public FROM books WHERE book_id = $1`, id,
	).Scan(&b.uploadedBy, &b.isPublic)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(404, models.ErrorResponse{Message: "Book not found"})
		return
	}
	if err != nil {
		internalErr(c, err)
		return
	}
	return id, b, true
}

// @Summary Список книг
// @Description Публичные книги с фильтрами и поиском по названию, по 20 на страницу
// @Tags Books
// @Produce json
// @Param q query string false "Поиск по названию"
// @Param topic_id query string false "ID жанра"
// @Param language_id query string false "ID языка"
// @Param author_id query string false "ID автора"
// @Param page query int false "Страница, с 1"
// @Success 200 {array} models.Book
// @Failure 400 {object} models.ErrorResponse
// @Router /books [get]
func (ctrl *BookController) List(c *gin.Context) {
	topic, ok := uuidQuery(c, "topic_id")
	if !ok {
		return
	}
	language, ok := uuidQuery(c, "language_id")
	if !ok {
		return
	}
	author, ok := uuidQuery(c, "author_id")
	if !ok {
		return
	}
	page := 1
	if p := c.Query("page"); p != "" {
		var err error
		if page, err = strconv.Atoi(p); err != nil || page < 1 {
			c.JSON(400, models.ErrorResponse{Message: "invalid page"})
			return
		}
	}

	rows, err := ctrl.pool.Query(c.Request.Context(),
		`SELECT b.book_id, b.title, b.description, b.is_public FROM books b
		 WHERE b.is_public
		   AND ($1::text = '' OR b.title ILIKE '%' || $1 || '%')
		   AND ($2::uuid IS NULL OR b.author_id = $2)
		   AND ($3::uuid IS NULL OR b.language_id = $3)
		   AND ($4::uuid IS NULL OR EXISTS (SELECT 1 FROM book_topics bt WHERE bt.book_id = b.book_id AND bt.topic_id = $4))
		 ORDER BY b.title, b.book_id LIMIT $5 OFFSET $6`,
		likeEscaper.Replace(c.Query("q")), author, language, topic, booksPageSize, (page-1)*booksPageSize)
	if err != nil {
		internalErr(c, err)
		return
	}
	books, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Book])
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(200, books)
}

// @Summary Книга
// @Description Публичная книга видна всем, непубличная — только владельцу и модераторам
// @Tags Books
// @Produce json
// @Param id path string true "ID книги"
// @Success 200 {object} models.Book
// @Failure 404 {object} models.ErrorResponse
// @Router /books/{id} [get]
func (ctrl *BookController) Get(c *gin.Context) {
	id, b, ok := ctrl.loadBook(c)
	if !ok {
		return
	}
	if !b.isPublic && b.uploadedBy != c.GetString(middleware.UserIDKey) && !ctrl.isModerator(c) {
		c.JSON(404, models.ErrorResponse{Message: "Book not found"})
		return
	}

	var book models.Book
	err := ctrl.pool.QueryRow(c.Request.Context(),
		`SELECT book_id, title, description, is_public FROM books WHERE book_id = $1`, id,
	).Scan(&book.BookID, &book.Title, &book.Description, &book.IsPublic)
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(200, book)
}

// @Summary Моя библиотека
// @Description Книги из библиотеки пользователя, опционально по статусу
// @Tags Books
// @Produce json
// @Param status query string false "Статус"
// @Success 200 {array} models.Book
// @Failure 401 {object} models.ErrorResponse
// @Router /users/me/books [get]
func (ctrl *BookController) MyBooks(c *gin.Context) {
	rows, err := ctrl.pool.Query(c.Request.Context(),
		`SELECT b.book_id, b.title, b.description, b.is_public
		 FROM user_library ul
		 JOIN books b USING (book_id)
		 LEFT JOIN book_status bs ON bs.book_id = b.book_id AND bs.user_id = ul.user_id
		 WHERE ul.user_id = $1 AND ($2::text = '' OR bs.status = $2)
		 ORDER BY ul.added_at DESC`,
		c.GetString(middleware.UserIDKey), c.Query("status"))
	if err != nil {
		internalErr(c, err)
		return
	}
	books, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Book])
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(200, books)
}

func bookWriteError(c *gin.Context, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		c.JSON(400, models.ErrorResponse{Message: "Unknown author or language"})
		return
	}
	internalErr(c, err)
}

// @Summary Создание книги
// @Description Создаёт непубличную книгу; публикация — через заявку на модерацию
// @Tags Books
// @Accept json
// @Produce json
// @Param request body models.CreateBookBody true "Данные книги"
// @Success 201 {object} models.BookIDResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Router /books [post]
func (ctrl *BookController) Create(c *gin.Context) {
	var body models.CreateBookBody
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid request"})
		return
	}
	if err := validate.Struct(body); err != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid request", Errors: utils.FormatValidationError(err)})
		return
	}

	var id string
	err := ctrl.pool.QueryRow(c.Request.Context(),
		`INSERT INTO books (title, description, author_id, language_id, published_at, file_path, uploaded_at, uploaded_by)
		 VALUES ($1, $2, $3, $4, $5, '', CURRENT_DATE, $6) RETURNING book_id`,
		body.Title, body.Description, body.AuthorID, body.LanguageID, body.PublishedAt, c.GetString(middleware.UserIDKey),
	).Scan(&id)
	if err != nil {
		bookWriteError(c, err)
		return
	}

	c.JSON(201, models.BookIDResponse{BookID: id})
}

// @Summary Редактирование книги
// @Description Владелец может править пока книга не опубликована, модератор — всегда. Меняются только переданные поля
// @Tags Books
// @Accept json
// @Produce json
// @Param id path string true "ID книги"
// @Param request body models.UpdateBookBody true "Изменяемые поля"
// @Success 200 {object} models.BookIDResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /books/{id} [patch]
func (ctrl *BookController) Update(c *gin.Context) {
	id, b, ok := ctrl.loadBook(c)
	if !ok {
		return
	}

	var body models.UpdateBookBody
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid request"})
		return
	}
	if err := validate.Struct(body); err != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid request", Errors: utils.FormatValidationError(err)})
		return
	}

	owner := b.uploadedBy == c.GetString(middleware.UserIDKey)
	if !(owner && !b.isPublic) && !ctrl.isModerator(c) {
		if !owner && !b.isPublic {
			c.JSON(404, models.ErrorResponse{Message: "Book not found"})
			return
		}
		c.JSON(403, models.ErrorResponse{Message: "Forbidden"})
		return
	}

	_, err := ctrl.pool.Exec(c.Request.Context(),
		`UPDATE books SET
		   title        = COALESCE($2, title),
		   description  = COALESCE($3, description),
		   author_id    = COALESCE($4::uuid, author_id),
		   language_id  = COALESCE($5::uuid, language_id),
		   published_at = COALESCE($6::date, published_at)
		 WHERE book_id = $1`,
		id, body.Title, body.Description, body.AuthorID, body.LanguageID, body.PublishedAt)
	if err != nil {
		bookWriteError(c, err)
		return
	}

	c.JSON(200, models.BookIDResponse{BookID: id})
}

// @Summary Удаление книги
// @Description Владелец или модератор
// @Tags Books
// @Param id path string true "ID книги"
// @Success 204 "Удалено"
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /books/{id} [delete]
func (ctrl *BookController) Delete(c *gin.Context) {
	id, b, ok := ctrl.loadBook(c)
	if !ok {
		return
	}

	if b.uploadedBy != c.GetString(middleware.UserIDKey) && !ctrl.isModerator(c) {
		if !b.isPublic {
			c.JSON(404, models.ErrorResponse{Message: "Book not found"})
			return
		}
		c.JSON(403, models.ErrorResponse{Message: "Forbidden"})
		return
	}

	ctx := c.Request.Context()
	tx, err := ctrl.pool.Begin(ctx)
	if err != nil {
		internalErr(c, err)
		return
	}
	defer tx.Rollback(ctx)

	for _, t := range []string{"book_topics", "reviews", "book_status", "book_add_requests", "user_library", "books"} {
		if _, err := tx.Exec(ctx, `DELETE FROM `+t+` WHERE book_id = $1`, id); err != nil {
			internalErr(c, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		internalErr(c, err)
		return
	}

	c.Status(204)
}

package controllers

import (
	"backend/models"
	"backend/utils"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type crud[B, T any] struct {
	pool     *pgxpool.Pool
	table    string
	idCol    string
	cols     []string
	order    string
	vals     func(B) []any
	notFound string
}

func (d crud[B, T]) sel() string { return strings.Join(append([]string{d.idCol}, d.cols...), ", ") }

func (d crud[B, T]) list(c *gin.Context) {
	rows, err := d.pool.Query(c.Request.Context(), fmt.Sprintf(`SELECT %s FROM %s ORDER BY %s`, d.sel(), d.table, d.order))
	if err != nil {
		internalErr(c, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		internalErr(c, err)
		return
	}

	c.JSON(200, items)
}

func (d crud[B, T]) id(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if validate.Var(id, "uuid") != nil {
		c.JSON(404, models.ErrorResponse{Message: d.notFound})
		return "", false
	}
	return id, true
}

func (d crud[B, T]) body(c *gin.Context) (b B, ok bool) {
	if err := c.BindJSON(&b); err != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid request"})
		return b, false
	}
	if err := validate.Struct(b); err != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid request", Errors: utils.FormatValidationError(err)})
		return b, false
	}
	return b, true
}

func (d crud[B, T]) one(c *gin.Context, status int, query string, args ...any) {
	rows, err := d.pool.Query(c.Request.Context(), query, args...)
	var item T
	if err == nil {
		item, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[T])
	}

	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		c.JSON(404, models.ErrorResponse{Message: d.notFound})
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		c.JSON(409, models.ErrorResponse{Message: "Already exists"})
	case err != nil:
		internalErr(c, err)
	default:
		c.JSON(status, item)
	}
}

func (d crud[B, T]) get(c *gin.Context) {
	if id, ok := d.id(c); ok {
		d.one(c, 200, fmt.Sprintf(`SELECT %s FROM %s WHERE %s = $1`, d.sel(), d.table, d.idCol), id)
	}
}

func (d crud[B, T]) create(c *gin.Context) {
	b, ok := d.body(c)
	if !ok {
		return
	}
	ph := make([]string, len(d.cols))
	for i := range ph {
		ph[i] = fmt.Sprintf("$%d", i+1)
	}
	d.one(c, 201, fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s) RETURNING %s`,
		d.table, strings.Join(d.cols, ", "), strings.Join(ph, ", "), d.sel()), d.vals(b)...)
}

func (d crud[B, T]) update(c *gin.Context) {
	id, ok := d.id(c)
	if !ok {
		return
	}
	b, ok := d.body(c)
	if !ok {
		return
	}
	sets := make([]string, len(d.cols))
	for i, col := range d.cols {
		sets[i] = fmt.Sprintf("%s = $%d", col, i+2)
	}
	d.one(c, 200, fmt.Sprintf(`UPDATE %s SET %s WHERE %s = $1 RETURNING %s`,
		d.table, strings.Join(sets, ", "), d.idCol, d.sel()), append([]any{id}, d.vals(b)...)...)
}

func (d crud[B, T]) delete(c *gin.Context) {
	id, ok := d.id(c)
	if !ok {
		return
	}
	tag, err := d.pool.Exec(c.Request.Context(), fmt.Sprintf(`DELETE FROM %s WHERE %s = $1`, d.table, d.idCol), id)
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23503":
		c.JSON(409, models.ErrorResponse{Message: "In use by books, can't delete"})
	case err != nil:
		internalErr(c, err)
	case tag.RowsAffected() == 0:
		c.JSON(404, models.ErrorResponse{Message: d.notFound})
	default:
		c.Status(204)
	}
}

type DictionaryController struct {
	topics    crud[models.TopicBody, models.Topic]
	authors   crud[models.AuthorBody, models.Author]
	languages crud[models.LanguageBody, models.Language]
}

func NewDictionaryController(pool *pgxpool.Pool) *DictionaryController {
	return &DictionaryController{
		topics: crud[models.TopicBody, models.Topic]{pool: pool, table: "topics", idCol: "topic_id",
			cols: []string{"name"}, order: "name", notFound: "Topic not found",
			vals: func(b models.TopicBody) []any { return []any{b.Name} }},
		authors: crud[models.AuthorBody, models.Author]{pool: pool, table: "authors", idCol: "author_id",
			cols: []string{"first_name", "last_name"}, order: "last_name, first_name", notFound: "Author not found",
			vals: func(b models.AuthorBody) []any { return []any{b.FirstName, b.LastName} }},
		languages: crud[models.LanguageBody, models.Language]{pool: pool, table: "languages", idCol: "language_id",
			cols: []string{"name"}, order: "name", notFound: "Language not found",
			vals: func(b models.LanguageBody) []any { return []any{b.Name} }},
	}
}

// @Summary Список жанров
// @Tags Topics
// @Produce json
// @Success 200 {array} models.Topic
// @Router /topics [get]
func (ctrl *DictionaryController) Topics(c *gin.Context) { ctrl.topics.list(c) }

// @Summary Жанр
// @Tags Topics
// @Produce json
// @Param id path string true "ID жанра"
// @Success 200 {object} models.Topic
// @Failure 404 {object} models.ErrorResponse
// @Router /topics/{id} [get]
func (ctrl *DictionaryController) Topic(c *gin.Context) { ctrl.topics.get(c) }

// @Summary Создание жанра (Moderator+)
// @Tags Topics
// @Accept json
// @Produce json
// @Param request body models.TopicBody true "Жанр"
// @Success 201 {object} models.Topic
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse "Already exists"
// @Router /topics [post]
func (ctrl *DictionaryController) CreateTopic(c *gin.Context) { ctrl.topics.create(c) }

// @Summary Изменение жанра (Moderator+)
// @Tags Topics
// @Accept json
// @Produce json
// @Param id path string true "ID жанра"
// @Param request body models.TopicBody true "Жанр"
// @Success 200 {object} models.Topic
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse "Already exists"
// @Router /topics/{id} [put]
func (ctrl *DictionaryController) UpdateTopic(c *gin.Context) { ctrl.topics.update(c) }

// @Summary Удаление жанра (Moderator+)
// @Tags Topics
// @Param id path string true "ID жанра"
// @Success 204 "Удалено"
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse "Used by books"
// @Router /topics/{id} [delete]
func (ctrl *DictionaryController) DeleteTopic(c *gin.Context) { ctrl.topics.delete(c) }

// @Summary Список авторов
// @Tags Authors
// @Produce json
// @Success 200 {array} models.Author
// @Router /authors [get]
func (ctrl *DictionaryController) Authors(c *gin.Context) { ctrl.authors.list(c) }

// @Summary Автор
// @Tags Authors
// @Produce json
// @Param id path string true "ID автора"
// @Success 200 {object} models.Author
// @Failure 404 {object} models.ErrorResponse
// @Router /authors/{id} [get]
func (ctrl *DictionaryController) Author(c *gin.Context) { ctrl.authors.get(c) }

// @Summary Создание автора (User+)
// @Description Доступно любому авторизованному пользователю: автор нужен при добавлении книги
// @Tags Authors
// @Accept json
// @Produce json
// @Param request body models.AuthorBody true "Автор"
// @Success 201 {object} models.Author
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Router /authors [post]
func (ctrl *DictionaryController) CreateAuthor(c *gin.Context) { ctrl.authors.create(c) }

// @Summary Изменение автора (Moderator+)
// @Tags Authors
// @Accept json
// @Produce json
// @Param id path string true "ID автора"
// @Param request body models.AuthorBody true "Автор"
// @Success 200 {object} models.Author
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /authors/{id} [put]
func (ctrl *DictionaryController) UpdateAuthor(c *gin.Context) { ctrl.authors.update(c) }

// @Summary Удаление автора (Moderator+)
// @Tags Authors
// @Param id path string true "ID автора"
// @Success 204 "Удалено"
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse "Used by books"
// @Router /authors/{id} [delete]
func (ctrl *DictionaryController) DeleteAuthor(c *gin.Context) { ctrl.authors.delete(c) }

// @Summary Список языков
// @Tags Languages
// @Produce json
// @Success 200 {array} models.Language
// @Router /languages [get]
func (ctrl *DictionaryController) Languages(c *gin.Context) { ctrl.languages.list(c) }

// @Summary Язык
// @Tags Languages
// @Produce json
// @Param id path string true "ID языка"
// @Success 200 {object} models.Language
// @Failure 404 {object} models.ErrorResponse
// @Router /languages/{id} [get]
func (ctrl *DictionaryController) Language(c *gin.Context) { ctrl.languages.get(c) }

// @Summary Создание языка (Moderator+)
// @Tags Languages
// @Accept json
// @Produce json
// @Param request body models.LanguageBody true "Язык"
// @Success 201 {object} models.Language
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse "Already exists"
// @Router /languages [post]
func (ctrl *DictionaryController) CreateLanguage(c *gin.Context) { ctrl.languages.create(c) }

// @Summary Изменение языка (Moderator+)
// @Tags Languages
// @Accept json
// @Produce json
// @Param id path string true "ID языка"
// @Param request body models.LanguageBody true "Язык"
// @Success 200 {object} models.Language
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse "Already exists"
// @Router /languages/{id} [put]
func (ctrl *DictionaryController) UpdateLanguage(c *gin.Context) { ctrl.languages.update(c) }

// @Summary Удаление языка (Moderator+)
// @Tags Languages
// @Param id path string true "ID языка"
// @Success 204 "Удалено"
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 409 {object} models.ErrorResponse "Used by books"
// @Router /languages/{id} [delete]
func (ctrl *DictionaryController) DeleteLanguage(c *gin.Context) { ctrl.languages.delete(c) }

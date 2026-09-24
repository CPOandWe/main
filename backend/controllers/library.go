package controllers

import (
	"backend/middleware"
	"backend/models"
	"backend/utils"

	"github.com/gin-gonic/gin"
)

// @Summary Добавить книгу в библиотеку
// @Description Можно добавить публичную книгу или свою. Повторное добавление ничего не меняет
// @Tags Books
// @Param id path string true "ID книги"
// @Success 204 "Книга в библиотеке"
// @Failure 401 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /users/me/books/{id} [post]
func (ctrl *BookController) AddToLibrary(c *gin.Context) {
	id, b, ok := ctrl.loadBook(c)
	if !ok {
		return
	}
	if !ctrl.canView(c, b) {
		c.JSON(404, models.ErrorResponse{Message: "Book not found"})
		return
	}

	if _, err := ctrl.pool.Exec(c.Request.Context(),
		`INSERT INTO user_library (user_id, book_id, added_at) VALUES ($1, $2, now())
		 ON CONFLICT (user_id, book_id) DO NOTHING`,
		c.GetString(middleware.UserIDKey), id); err != nil {
		internalErr(c, err)
		return
	}

	c.Status(204)
}

// @Summary Убрать книгу из библиотеки
// @Description Идемпотентно: если книги в библиотеке нет, тоже 204
// @Tags Books
// @Param id path string true "ID книги"
// @Success 204 "Книги нет в библиотеке"
// @Failure 401 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse "Invalid book id"
// @Router /users/me/books/{id} [delete]
func (ctrl *BookController) RemoveFromLibrary(c *gin.Context) {
	id := c.Param("id")
	if validate.Var(id, "uuid") != nil {
		c.JSON(404, models.ErrorResponse{Message: "Book not found"})
		return
	}

	// reading status is per (user, book): drop it too so a re-add starts clean
	me := c.GetString(middleware.UserIDKey)
	if _, err := ctrl.pool.Exec(c.Request.Context(),
		`WITH s AS (DELETE FROM book_status WHERE user_id = $1 AND book_id = $2)
		 DELETE FROM user_library WHERE user_id = $1 AND book_id = $2`, me, id); err != nil {
		internalErr(c, err)
		return
	}

	c.Status(204)
}

// @Summary Статус чтения
// @Description Отмечает книгу из библиотеки как reading или read
// @Tags Books
// @Accept json
// @Param id path string true "ID книги"
// @Param request body models.ReadingStatusBody true "Статус чтения"
// @Success 204 "Статус сохранён"
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse "Book is not in the library"
// @Router /users/me/books/{id}/status [put]
func (ctrl *BookController) SetReadingStatus(c *gin.Context) {
	id := c.Param("id")
	if validate.Var(id, "uuid") != nil {
		c.JSON(404, models.ErrorResponse{Message: "Book not found"})
		return
	}

	var body models.ReadingStatusBody
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid request"})
		return
	}
	if err := validate.Struct(body); err != nil {
		c.JSON(400, models.ErrorResponse{Message: "invalid request", Errors: utils.FormatValidationError(err)})
		return
	}

	tag, err := ctrl.pool.Exec(c.Request.Context(),
		`INSERT INTO book_status (book_id, user_id, status)
		 SELECT $2, $1, $3 WHERE EXISTS (SELECT 1 FROM user_library WHERE user_id = $1 AND book_id = $2)
		 ON CONFLICT (book_id, user_id) DO UPDATE SET status = EXCLUDED.status`,
		c.GetString(middleware.UserIDKey), id, body.Status)
	if err != nil {
		internalErr(c, err)
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(404, models.ErrorResponse{Message: "Book is not in your library"})
		return
	}

	c.Status(204)
}

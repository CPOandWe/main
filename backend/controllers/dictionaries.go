package controllers

import (
	"backend/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DictionaryController struct {
	pool *pgxpool.Pool
}

func NewDictionaryController(pool *pgxpool.Pool) *DictionaryController {
	return &DictionaryController{pool: pool}
}

func listAll[T any](c *gin.Context, pool *pgxpool.Pool, query string) {
	rows, err := pool.Query(c.Request.Context(), query)
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

// @Summary Список жанров
// @Tags Dictionaries
// @Produce json
// @Success 200 {array} models.Topic
// @Router /topics [get]
func (ctrl *DictionaryController) Topics(c *gin.Context) {
	listAll[models.Topic](c, ctrl.pool, `SELECT topic_id, name FROM topics ORDER BY name`)
}

// @Summary Список авторов
// @Tags Dictionaries
// @Produce json
// @Success 200 {array} models.Author
// @Router /authors [get]
func (ctrl *DictionaryController) Authors(c *gin.Context) {
	listAll[models.Author](c, ctrl.pool, `SELECT author_id, first_name, last_name FROM authors ORDER BY last_name, first_name`)
}

// @Summary Список языков
// @Tags Dictionaries
// @Produce json
// @Success 200 {array} models.Language
// @Router /languages [get]
func (ctrl *DictionaryController) Languages(c *gin.Context) {
	listAll[models.Language](c, ctrl.pool, `SELECT language_id, name FROM languages ORDER BY name`)
}

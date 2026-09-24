package models

type Book struct {
	BookID      string  `json:"book_id" db:"book_id"`
	Title       string  `json:"title" db:"title"`
	Description string  `json:"description" db:"description"`
	IsPublic    bool    `json:"is_public" db:"is_public"`
	CoverURL    *string `json:"cover_url" db:"cover_url"` // null when the book has no cover
}

type CreateBookBody struct {
	Title       string `json:"title" validate:"required"`
	Description string `json:"description" validate:"required"`
	AuthorID    string `json:"author_id" validate:"required,uuid"`
	LanguageID  string `json:"language_id" validate:"required,uuid"`
	PublishedAt string `json:"published_at" validate:"required,datetime=2006-01-02"`
}

// all fields optional: only the ones sent are changed
type UpdateBookBody struct {
	Title       *string `json:"title" validate:"omitempty,min=1"`
	Description *string `json:"description" validate:"omitempty,min=1"`
	AuthorID    *string `json:"author_id" validate:"omitempty,uuid"`
	LanguageID  *string `json:"language_id" validate:"omitempty,uuid"`
	PublishedAt *string `json:"published_at" validate:"omitempty,datetime=2006-01-02"`
}

type BookIDResponse struct {
	BookID string `json:"book_id"`
}

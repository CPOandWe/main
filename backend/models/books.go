package models

import "time"

type Book struct {
	BookID      string   `json:"book_id" db:"book_id"`
	Title       string   `json:"title" db:"title"`
	Description string   `json:"description" db:"description"`
	IsPublic    bool     `json:"is_public" db:"is_public"`
	CoverURL    *string  `json:"cover_url" db:"cover_url"` // null when the book has no cover
	Authors     []Author `json:"authors" db:"authors"`
	// library fields: null for guests and for books the user has not saved
	SavedAt       *time.Time `json:"saved_at" db:"saved_at"`
	ReadingStatus *string    `json:"reading_status" db:"reading_status"`
}

type CreateBookBody struct {
	Title       string   `json:"title" validate:"required"`
	Description string   `json:"description" validate:"required"`
	AuthorIDs   []string `json:"author_ids" validate:"required,min=1,dive,uuid"`
	LanguageID  string   `json:"language_id" validate:"required,uuid"`
	PublishedAt string   `json:"published_at" validate:"required,datetime=2006-01-02"`
}

// all fields optional: only the ones sent are changed
type UpdateBookBody struct {
	Title       *string  `json:"title" validate:"omitempty,min=1"`
	Description *string  `json:"description" validate:"omitempty,min=1"`
	AuthorIDs   []string `json:"author_ids" validate:"omitempty,min=1,dive,uuid"` // replaces the whole author list
	LanguageID  *string  `json:"language_id" validate:"omitempty,uuid"`
	PublishedAt *string  `json:"published_at" validate:"omitempty,datetime=2006-01-02"`
}

type BookIDResponse struct {
	BookID string `json:"book_id"`
}

type BookStatusResponse struct {
	Status string  `json:"status"` // draft, pending, approved, rejected, published
	Reason *string `json:"reason"` // set when rejected
}

type ModerationRequest struct {
	RequestID   string `json:"request_id" db:"request_id"`
	BookID      string `json:"book_id" db:"book_id"`
	Title       string `json:"title" db:"title"`
	Description string `json:"description" db:"description"`
	IsPublic    bool   `json:"is_public" db:"is_public"`
	Author      string `json:"author" db:"author"`
}

type ReviewRequestBody struct {
	Status string `json:"status" validate:"required,oneof=approved rejected"`
	Reason string `json:"reason"`
}

type ReviewRequestResponse struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// UploadedBook is a book uploaded by the user with its moderation status.
type UploadedBook struct {
	BookID        string     `json:"book_id" db:"book_id"`
	Title         string     `json:"title" db:"title"`
	Description   string     `json:"description" db:"description"`
	IsPublic      bool       `json:"is_public" db:"is_public"`
	CoverURL      *string    `json:"cover_url" db:"cover_url"`
	Authors       []Author   `json:"authors" db:"authors"`
	Status        string     `json:"status" db:"status"`
	SavedAt       *time.Time `json:"saved_at" db:"saved_at"`
	ReadingStatus *string    `json:"reading_status" db:"reading_status"`
}

// LibraryBook is a library entry; Status is the reading status (nil until set).
type LibraryBook struct {
	BookID      string    `json:"book_id" db:"book_id"`
	Title       string    `json:"title" db:"title"`
	Description string    `json:"description" db:"description"`
	IsPublic    bool      `json:"is_public" db:"is_public"`
	CoverURL    *string   `json:"cover_url" db:"cover_url"`
	Authors     []Author  `json:"authors" db:"authors"`
	Status      *string   `json:"status" db:"status"`
	SavedAt     time.Time `json:"saved_at" db:"saved_at"`
}

type ReadingStatusBody struct {
	Status string `json:"status" validate:"required,oneof=reading read"`
}

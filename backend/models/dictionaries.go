package models

type Topic struct {
	TopicID string `json:"topic_id" db:"topic_id"`
	Name    string `json:"name" db:"name"`
}

type Author struct {
	AuthorID  string `json:"author_id" db:"author_id"`
	FirstName string `json:"first_name" db:"first_name"`
	LastName  string `json:"last_name" db:"last_name"`
}

type Language struct {
	LanguageID string `json:"language_id" db:"language_id"`
	Name       string `json:"name" db:"name"`
}

package utils

import (
	"strings"

	"github.com/go-playground/validator/v10"
)

func FormatValidationError(err error) map[string]string {
	if errs, ok := err.(validator.ValidationErrors); ok {
		errors := make(map[string]string)
		for _, e := range errs {
			field := strings.ToLower(e.Field())
			switch e.Tag() {
			case "required":
				errors[field] = "обязательное поле"
			case "email":
				errors[field] = "некорректный email"
			case "min":
				errors[field] = "минимум " + e.Param() + " символов"
			default:
				errors[field] = "некорректное значение"
			}
		}
		return errors
	}

	return nil

}

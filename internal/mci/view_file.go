package mci

import (
	"os"
	"strings"
)

func LoadViewFile(path string) (View, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return View{}, os.ErrInvalid
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return View{}, err
	}
	return ParseHJSON(body)
}

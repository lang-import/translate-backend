package translator

import (
	"io"
)

type Translator interface {
	io.Closer
	Translate(lang string, word string) (*Translation, error)
}

type Translation struct {
	Word  string `json:"word"`
	Spell string `json:"spell"`
}

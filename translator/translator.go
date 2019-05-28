package translator

import (
	"io"
)

type Translator interface {
	io.Closer
	Translate(lang string, word string) (*Translation, error)
}

type Translation struct {
	Original string `json:"original"`
	Lang     string `json:"lang"`
	Word     string `json:"word"`
	Spell    string `json:"spell"`
}

func (tr *Translation) Valid() bool {
	return tr.Word != "" && tr.Lang != "" && tr.Original != ""
}

package translator

import (
	"github.com/pkg/errors"
	"math/rand"
)

type Strategy interface {
	Gen(pool []Translator) []Translator
}

type StraightForward struct{}

func (rb *StraightForward) Gen(pool []Translator) []Translator {
	return pool
}

type Random struct{}

func (*Random) Gen(pool []Translator) []Translator {
	p := pool
	cp := make([]Translator, len(p))
	copy(cp, p)
	rand.Shuffle(len(pool), func(i, j int) {
		cp[i], cp[j] = cp[j], cp[i]
	})
	return cp
}

func NewPool(strategy Strategy, pool ...Translator) Translator {
	return &poolTranslator{
		pool:     pool,
		strategy: strategy,
	}
}

type poolTranslator struct {
	pool     []Translator
	strategy Strategy
}

func (p *poolTranslator) Close() error {
	return nil
}

func (p *poolTranslator) Translate(lang string, word string) (*Translation, error) {
	backs := p.strategy.Gen(p.pool)
	for _, trans := range backs {
		tr, err := trans.Translate(lang, word)
		if err == nil {
			return tr, nil
		}
	}
	return nil, errors.New("no suitable translator available")
}

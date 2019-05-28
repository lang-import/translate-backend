package translator

import (
	"encoding/json"
	"github.com/reddec/storages"
	"log"
	"os"
)

func NewCached(wrap Translator, cache storages.Storage) Translator {
	return &cachedTranslator{
		logger:  log.New(os.Stderr, "[cache] ", log.LstdFlags),
		cache:   cache,
		wrapped: wrap,
	}
}

type cachedTranslator struct {
	cache   storages.Storage
	wrapped Translator
	logger  *log.Logger
}

func (ct *cachedTranslator) Close() error {
	_ = ct.cache.Close()
	return ct.wrapped.Close()
}

func (ct *cachedTranslator) Translate(lang string, word string) (*Translation, error) {
	key := []byte(lang + ":" + word)
	value, err := ct.cache.Get(key)
	if err == nil {
		var tr *Translation
		err = json.Unmarshal(value, &tr)
		if err == nil && tr.Original != "" && tr.Word != "" {
			return tr, nil
		}
		// bad cache
	}
	//cache miss or whatever
	tr, err := ct.wrapped.Translate(lang, word)
	if err != nil {
		return nil, err
	}
	value, err = json.Marshal(tr)
	if err != nil {
		// almost impossible
		ct.logger.Println("failed marshal translation:", err)
		return nil, err
	}

	if err = ct.cache.Put(key, value); err != nil {
		ct.logger.Println("failed save to cache translation:", err)
	}
	return tr, nil
}

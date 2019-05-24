package translator

import (
	"encoding/json"
	"github.com/pkg/errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
)

func NewRemote(baseURL string) Translator {
	return &remoteTranslator{
		client:  http.DefaultClient,
		baseURL: baseURL,
		logger:  log.New(os.Stderr, "["+baseURL+"] ", log.LstdFlags),
	}
}

type remoteTranslator struct {
	baseURL string
	client  *http.Client
	logger  *log.Logger
}

func (rt *remoteTranslator) Close() error {
	return nil
}

func (rt *remoteTranslator) Translate(lang string, word string) (*Translation, error) {
	targetURL := rt.baseURL + "/translate/" + url.PathEscape(word) + "/to/" + url.PathEscape(word) + "/full"
	rt.logger.Println(lang, "=>", word)
	res, err := rt.client.Get(targetURL)
	if err != nil {
		rt.logger.Println("failed:", err)
		return nil, err
	}
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		rt.logger.Println("failed:", err)
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		rt.logger.Println("failed:", res.Status+" "+string(data))
		return nil, errors.New(string(data))
	}
	var tr Translation
	return &tr, json.Unmarshal(data, &tr)
}

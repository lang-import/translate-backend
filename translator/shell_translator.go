package translator

import (
	"bytes"
	"github.com/pkg/errors"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

func NewShell(binary string) (Translator, error) {
	tr := &shellTranslator{
		binary: binary,
		logger: log.New(os.Stderr, "[translator] ", log.LstdFlags),
	}
	engines, err := tr.listEngines()
	if err != nil {
		return nil, err
	}
	tr.engines = engines
	return tr, nil
}

type shellTranslator struct {
	engines []string
	binary  string
	logger  *log.Logger
}

func (t *shellTranslator) Engines() []string {
	return t.engines
}

func (t *shellTranslator) Close() error { return nil }

func (t *shellTranslator) Translate(lang string, word string) (*Translation, error) {
	for _, engine := range t.engines {
		tr, err := t.translateWithEngine(engine, lang, word)
		if err == nil {
			return tr, nil
		}
		t.logger.Println("translate failed by", engine, "failed:", err)
	}
	return nil, errors.Errorf("failed to translate %v to language %v by any engine", word, lang)
}

func (t *shellTranslator) translateWithEngine(engine, lang, word string) (*Translation, error) {
	const (
		phonetic  = "-show-original-phonetics"
		s_lang    = "-show-languages"
		s_o_dct   = "-show-original-dictionary"
		s_promt   = "-show-original"
		s_alt     = "-show-alternatives"
		s_dct     = "-show-dictionary"
		no_colors = "-no-ansi"
		n         = "n"
	)
	out := &bytes.Buffer{}
	combined := &bytes.Buffer{}
	args := []string{"-e", engine, phonetic, n, no_colors, s_lang, n, s_o_dct, n, s_promt, n, s_alt, n, s_dct, n, ":" + lang, word}
	t.logger.Println(t.binary, strings.Join(args, " "))
	cmd := exec.Command(t.binary, args...)
	cmd.Stdout = io.MultiWriter(out, combined)
	cmd.Stderr = combined
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	translateResult := string(out.Bytes())
	lines := strings.Split(translateResult, "\n")
	if len(lines) < 2 {
		return nil, errors.New("no result")
	}

	result := &Translation{
		Original: word,
		Lang:     lang,
		Word:     lines[0],
	}

	for _, line := range lines {
		line = strings.TrimSpace(strings.Map(func(r rune) rune {
			if unicode.IsPrint(r) {
				return r
			}
			return -1
		}, line))
		if strings.HasPrefix(line, "(") {
			result.Spell = line[1 : len(line)-1]
			break
		}
	}
	return result, nil
}

func (t *shellTranslator) listEngines() ([]string, error) {
	cmd := exec.Command(t.binary, "-S")
	data, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	content := strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, string(data))
	var engines []string
	for _, line := range strings.Split(content, " ") {
		line = strings.Replace(strings.TrimSpace(line), "*", "", -1)
		if len(line) < 3 {
			continue
		}
		engines = append(engines, line)
		if line == "google" {
			n := len(engines) - 1
			engines[0], engines[n] = engines[n], engines[0] //move google to first
		} else if line == "aspell" {
			engines = engines[0 : len(engines)-1]
		}
	}
	return engines, nil
}

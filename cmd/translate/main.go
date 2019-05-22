package main

import (
	"bytes"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	"github.com/jessevdk/go-flags"
	"github.com/pkg/errors"
	"gopkg.in/telegram-bot-api.v4"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var config struct {
	Redis                string        `long:"redis-url" env:"REDIS_URL" description:"Redis database" default:"redis://redis"`
	Command              string        `long:"command" env:"COMMAND" description:"Command to run" default:"/usr/bin/trans"`
	Listen               string        `long:"listen" env:"LISTEN" description:"Address to listen" default:":8888"`
	BotToken             string        `long:"tg-token" env:"TG_TOKEN" description:"Telegram BOT API token for notifications"`
	BotChatID            int64         `long:"tg-chat-id" env:"TG_CHAT_ID" description:"Telegram chat ID"`
	ThrottleNotification time.Duration `long:"notification-interval" env:"NOTIFICATION_INTERVAL" description:"Merge notifications to one message during this time" default:"1m"`
}

var engines = []string{"google"} // default, will be overwritten
var notifyChannel chan string

func main() {
	_, err := flags.Parse(&config)
	if err != nil {
		os.Exit(1)
	}
	first := &redis.Options{Addr: config.Redis, DB: 1}
	second := &redis.Options{Addr: config.Redis, DB: 2}
	client := redis.NewClient(first)
	clientFull := redis.NewClient(second)
	router := gin.Default()
	router.GET("/translate/:word/to/:lang", func(gctx *gin.Context) {
		word := strings.ToLower(strings.TrimSpace(gctx.Param("word")))
		lang := strings.ToLower(strings.TrimSpace(gctx.Param("lang")))
		if word == "" {
			gctx.String(http.StatusOK, "")
			return
		}
		if lang == "" {
			gctx.String(http.StatusOK, "")
			return
		}
		cached := client.HGet(lang, word)
		ans := cached.Val()
		if cached.Err() != nil {
			ans = fetch(word, lang, client, true)
		}
		gctx.String(http.StatusOK, ans)
		return
	})

	router.GET("/translate_full/:word/to/:lang", func(gctx *gin.Context) {
		word := strings.ToLower(strings.TrimSpace(gctx.Param("word")))
		lang := strings.ToLower(strings.TrimSpace(gctx.Param("lang")))
		if word == "" {
			gctx.String(http.StatusOK, "")
			return
		}
		if lang == "" {
			gctx.String(http.StatusOK, "")
			return
		}
		cached := clientFull.HGet(lang, word)
		ans := cached.Val()
		if cached.Err() != nil {
			ans = fetch(word, lang, client, false)
		}
		gctx.String(http.StatusOK, ans)
		return
	})

	notifyChannel = make(chan string)
	go notificationLoop()
	go func() { notifyChannel <- "import-lang backend started" }()
	go cleanup(client)
	go func() {
		list, err := getEngines()
		if err != nil {
			errorNotification("failed get engines list: " + err.Error())
		} else {
			infoNotification("supported engines: " + strings.Join(list, ", "))
			engines = list
		}
	}()
	panic(router.Run(config.Listen))
}

func fetch(word, lang string, client *redis.Client, isBrief bool) string {
	for _, engine := range engines {
		ans, err := invokeTrans(word, lang, engine, isBrief)
		if err == nil {
			client.HSet(lang, word, ans)
			return ans
		}
	}
	e := errors.New("failed to translate in all engines")
	fmt.Println(e, word)
	onTranslationError(word, lang, e)
	return word
}

const (
	phonetic = "-show-original-phonetics"
	s_lang   = "-show-languages"
	s_o_dct  = "-show-original-dictionary"
	s_promt  = "-show-original"
	s_alt    = "-show-alternatives"
	s_dct    = "-show-dictionary"
	brief    = "-b"
	n        = "n"
)

func invokeTrans(word, lang, engine string, isBrief bool) (string, error) {
	out := &bytes.Buffer{}
	combined := &bytes.Buffer{}

	var cmd *exec.Cmd
	if isBrief {
		cmd = exec.Command(config.Command, "-e", engine, brief, ":"+lang, word)
	} else {
		cmd = exec.Command(config.Command, "-e", engine, phonetic, n, s_lang, n, s_o_dct, n, s_promt, n, s_alt, n, s_dct, n, ":"+lang, word)
	}
	cmd.Stdout = io.MultiWriter(out, combined)
	cmd.Stderr = combined
	err := cmd.Run()
	translateResult := string(out.Bytes())
	fmt.Println(engine, ":", translateResult)

	//if !isBrief {
	//TODO　clear translateResult from [1m [22m
	//}

	if err != nil {
		fmt.Println("failed to translate", word, ":", err)
		return "", err
	}
	ans := strings.ToLower(strings.TrimSpace(string(out.Bytes())))
	if ans == "" {
		return "", errors.New("empty reply from API")
	}
	return ans, nil
}

func onTranslationError(originalWord, targetLanguage string, err error) {
	fmt.Println("[error] ", originalWord, "(to", targetLanguage+")", err)
	notifyChannel <- fmt.Sprint("[error] ", originalWord, " (to ", targetLanguage+") ", err)
}

func notificationLoop() {
	var bot *tgbotapi.BotAPI
	fmt.Println("initializing telegram bot...")
	if bt, err := tgbotapi.NewBotAPI(config.BotToken); err != nil {
		fmt.Println("failed initialize telegram notifications:", err)
	} else {
		bot = bt
		fmt.Println("telegram bot initialized")
	}
	var batch []string
	ticker := time.NewTicker(config.ThrottleNotification)
	defer ticker.Stop()
	for {
		select {
		case msg := <-notifyChannel:
			fmt.Println(msg)
			batch = append(batch, msg)
		case <-ticker.C:
			if len(batch) == 0 {
				continue
			}
			msg := strings.Join(batch, "\n")

			if bot == nil {
				batch = nil
				continue // >> /dev/null
			}
			fmt.Println("sending notification batch")
			tmsg := tgbotapi.NewMessage(config.BotChatID, msg)
			tmsg.DisableWebPagePreview = true
			_, err := bot.Send(tmsg)
			if err != nil {
				fmt.Println("failed send notification over telegram:", err)
			} else {
				batch = nil
				fmt.Println("notification batch sent")
			}

		}
	}
}

func cleanup(client *redis.Client) {
	removeEmptyTranslations(client)
	removeNonPrintableTranslations(client)
}

func removeEmptyTranslations(client *redis.Client) {
	removed := 0
	var stats = make(map[string]int)
	for _, lang := range client.Keys("*").Val() {
		fmt.Println("cleaning for", lang)
		for word, value := range client.HGetAll(lang).Val() {
			if value == "" {
				go func() { notifyChannel <- fmt.Sprintf("[rm_empty_word] %s | %s |", word, value) }()
				client.HDel(lang, word)
				removed++
				stats[lang] = stats[lang] + 1
			}
		}
	}
	if removed > 0 {
		text := []string{fmt.Sprint("[info] removed ", removed, " trashed (empty) translations")}
		for lang, count := range stats {
			text = append(text, fmt.Sprint(lang, ": ", count, " removes"))
		}
		go func() {
			s := strings.Join(text, "\n")
			fmt.Println(s)
			notifyChannel <- s
		}()
	}
}

func removeNonPrintableTranslations(client *redis.Client) {
	removed := 0
	var stats = make(map[string]int)
	for _, lang := range client.Keys("*").Val() {
		fmt.Println("cleaning for", lang)
		for word, value := range client.HGetAll(lang).Val() {
			for _, char := range value {
				if !unicode.IsGraphic(char) {
					go func() { notifyChannel <- fmt.Sprintf("[rm_bad_word] %s | %s |", word, value) }()
					client.HDel(lang, word)
					removed++
					stats[lang] = stats[lang] + 1
					break
				}
			}

		}
	}
	if removed > 0 {
		text := []string{fmt.Sprint("[info] removed ", removed, " non-printable translations")}
		for lang, count := range stats {
			text = append(text, fmt.Sprint(lang, ": ", count, " removes"))
		}
		go func() {
			s := strings.Join(text, "\n")
			fmt.Println(s)
			notifyChannel <- s
		}()
	}
}

func infoNotification(message string) {
	go func() { notifyChannel <- fmt.Sprint("[info] ", message) }()
}

func errorNotification(message string) {
	go func() { notifyChannel <- fmt.Sprint("[error] ", message) }()
}

var textOnly = regexp.MustCompile("\\w+")

func getEngines() ([]string, error) {
	//ssh root@ams.dc.mesh0.com docker exec 73704452b411 trans -S
	cmd := exec.Command(config.Command, "-S")
	data, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	var engines []string
	for _, line := range strings.Split(string(data), "\n") {
		if l := textOnly.FindString(line); l != "" {
			engines = append(engines, l)
		}
	}
	for i, eng := range engines {
		if eng != "google" {
			continue
		}
		if i == 0 {
			break
		}
		engines[0], engines[i] = engines[i], engines[0] //move google to first
		break
	}

	engines = removeEng(engines, "aspell")

	go func() { notifyChannel <- fmt.Sprint("[info_engines] ", engines) }()
	return engines, nil
}

func removeEng(engines []string, s string) []string {
	rmIndex := -1
	for i, eng := range engines {
		if eng == s {
			rmIndex = i
			break
		}
	}

	if rmIndex > -1 {
		engines[rmIndex] = engines[len(engines)-1]
		engines[len(engines)-1] = ""
		engines = engines[:len(engines)-1]
	}
	return engines
}

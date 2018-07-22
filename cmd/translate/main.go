package main

import (
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	"strings"
	"net/http"
	"sync"
	"os/exec"
	"fmt"
	"github.com/jessevdk/go-flags"
	"os"
	"github.com/pkg/errors"
	"time"
	"gopkg.in/telegram-bot-api.v4"
	"unicode"
	"bytes"
	"io"
)

var config struct {
	Redis                string        `long:"redis-url" env:"REDIS_URL" description:"Redis database" default:"redis://redis/1"`
	Command              string        `long:"command" env:"COMMAND" description:"Command to run" default:"/usr/bin/trans"`
	Listen               string        `long:"listen" env:"LISTEN" description:"Address to listen" default:":8888"`
	BotToken             string        `long:"tg-token" env:"TG_TOKEN" description:"Telegram BOT API token for notifications"`
	BotChatID            int64         `long:"tg-chat-id" env:"TG_CHAT_ID" description:"Telegram chat ID"`
	ThrottleNotification time.Duration `long:"notification-interval" env:"NOTIFICATION_INTERVAL" description:"Merge notifications to one message during this time" default:"1m"`
}

var notifyChannel chan string

func main() {
	_, err := flags.Parse(&config)
	if err != nil {
		os.Exit(1)
	}
	clientConfig, err := redis.ParseURL(config.Redis)
	if err != nil {
		panic(err)
	}
	client := redis.NewClient(clientConfig)
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
		if cached.Err() == redis.Nil {
			ans = fetch(word, lang, client)
		}
		gctx.String(http.StatusOK, ans)
		return
	})

	notifyChannel = make(chan string)
	go notificationLoop()
	go func() { notifyChannel <- "import-lang backend started" }()
	go cleanup(client)
	panic(router.Run(config.Listen))
}

var fetchLock sync.Mutex

func fetch(word, lang string, client *redis.Client) (string) {
	fetchLock.Lock()
	cached := client.HGet(lang, word)
	if cached.Err() == nil {
		fetchLock.Unlock()
		return cached.Val()
	}
	defer fetchLock.Unlock()
	out := &bytes.Buffer{}
	combined := &bytes.Buffer{}
	cmd := exec.Command(config.Command, "-b", ":"+lang, word)
	cmd.Stdout = io.MultiWriter(out, combined)
	cmd.Stderr = combined
	err := cmd.Run()
	fmt.Println(string(combined.String()))
	if err != nil {
		fmt.Println("failed to translate", word, ":", err)
		onTranslationError(word, lang, errors.Wrapf(err, "API failed"))
		return word
	}
	ans := strings.ToLower(strings.TrimSpace(string(out.Bytes())))
	if ans == "" {
		onTranslationError(word, lang, errors.New("empty reply from API"))
	}
	client.HSet(lang, word, ans)
	return ans
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

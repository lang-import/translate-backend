package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jessevdk/go-flags"
	"translate-backend/translator"
	"github.com/reddec/storages/redistorage"
	"gopkg.in/telegram-bot-api.v4"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var config struct {
	Remote               []string      `long:"remote" env:"REMOTE" description:"Remote base URLS"`
	Restrict             []string      `long:"restrict" env:"RESTRICT" description:"Restrict access by IP"`
	Redis                string        `long:"redis-url" env:"REDIS_URL" description:"Redis database" default:"redis://localhost"`
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

	notifyChannel = make(chan string)
	go notificationLoop()
	go func() { notifyChannel <- "import-lang backend started" }()

	panic(run())
}

func run() error {

	localTranslator, err := translator.NewShell(config.Command)
	if err != nil {
		return err
	}
	defer localTranslator.Close()

	var backends = []translator.Translator{localTranslator}
	for _, baseURL := range config.Remote {
		log.Println("registering remote backend on", baseURL)
		backends = append(backends, translator.NewRemote(baseURL))
	}

	pool := translator.NewPool(&translator.Random{}, backends...)

	cache, err := redistorage.New("", config.Redis)
	if err != nil {
		return err
	}
	defer cache.Close()

	cachedTranslator := translator.NewCached(pool, cache)

	return setupRoutes(cachedTranslator).Run(config.Listen)
}

func setupRoutes(trans translator.Translator) *gin.Engine {
	gin.Default()
	router := gin.Default()
	if len(config.Restrict) > 0 {
		router.Use(func(gctx *gin.Context) {
			cip := gctx.ClientIP()
			for _, ip := range config.Restrict {
				if ip == cip {
					gctx.Next()
					return
				}
			}
			gctx.AbortWithStatus(http.StatusForbidden)
		})
	}
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
		infoNotification("translate " + word + " to " + lang)
		ans, err := trans.Translate(lang, word)
		if err != nil {
			errorNotification("translate " + word + " to " + lang + ": " + err.Error())
			gctx.String(http.StatusOK, word)
			return
		}
		gctx.String(http.StatusOK, ans.Word)
		return
	})

	router.GET("/translate/:word/to/:lang/full", func(gctx *gin.Context) {
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
		infoNotification("translate " + word + " to " + lang)
		ans, err := trans.Translate(lang, word)
		if err != nil {
			errorNotification("translate " + word + " to " + lang + ": " + err.Error())
			gctx.String(http.StatusOK, word)
			return
		}
		gctx.JSON(http.StatusOK, ans)
		return
	})

	return router
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

func infoNotification(message string) {
	go func() { notifyChannel <- fmt.Sprint("[info] ", message) }()
}

func errorNotification(message string) {
	go func() { notifyChannel <- fmt.Sprint("[error] ", message) }()
}

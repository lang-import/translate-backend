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
)

var config struct {
	Redis   string `long:"redis-url" env:"REDIS_URL" description:"Redis database" default:"redis://redis/1"`
	Command string `long:"command" env:"COMMAND" description:"Command to run" default:"/usr/bin/trans"`
	Listen  string `long:"listen" env:"LISTEN" description:"Address to listen" default:":8888"`
}

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

	panic(router.Run(config.Listen))
}

var fetchLock sync.Mutex

func fetch(word, lang string, client *redis.Client) string {
	fetchLock.Lock()
	cached := client.HGet(lang, word)
	if cached.Err() == nil {
		fetchLock.Unlock()
		return cached.Val()
	}
	defer fetchLock.Unlock()
	cmd := exec.Command(config.Command, "-b", ":"+lang, word)
	res, err := cmd.CombinedOutput()
	fmt.Println(string(res))
	if err != nil {
		fmt.Println("failed to translate", word, ":", err)
		return word
	}
	ans := strings.ToLower(strings.TrimSpace(string(res)))

	client.HSet(lang, word, ans)
	return ans
}

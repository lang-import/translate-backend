package main

import (
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	"strings"
	"net/http"
	"sync"
	"os/exec"
	"fmt"
)

var config struct {
	Redis   string `long:"redis-url" env:"REDIS_URL" description:"Redis database" default:"redis://redis/1"`
	Command string `long:"command" env:"COMMAND" description:"Command to run" default:"/usr/bin/trans"`
	Listen  string `long:"listen" env:"LISTEN" description:"Address to listen" default:":8888"`
}

func main() {
	clientConfig, err := redis.ParseURL(config.Redis)
	if err != nil {
		panic(err)
	}
	client := redis.NewClient(clientConfig)
	router := gin.Default()
	router.GET("/translate/:word", func(gctx *gin.Context) {
		word := strings.ToLower(strings.TrimSpace(gctx.Param("word")))
		if word == "" {
			gctx.String(http.StatusOK, "")
			return
		}
		cached := client.Get(word)
		ans := cached.String()
		if cached.Err() == redis.Nil {
			ans = fetch(word, client)
		}
		gctx.String(http.StatusOK, ans)
		return
	})

	panic(router.Run(config.Listen))
}

var fetchLock sync.Mutex

func fetch(word string, client *redis.Client) string {
	fetchLock.Lock()
	cached := client.Get(word)
	if cached.Err() == nil {
		fetchLock.Unlock()
		return cached.String()
	}
	defer fetchLock.Unlock()
	cmd := exec.Command(config.Command, word)
	res, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("failed to translate", word, ":", err)
		return word
	}

	ans := strings.TrimSpace(strings.ToLower(strings.SplitN(string(res), "\n", 2)[0]))

	client.Set(word, ans, 0)
	return ans
}

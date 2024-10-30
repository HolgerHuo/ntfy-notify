package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"

	"ntfy-notify/config"
	"ntfy-notify/notifier"

	"github.com/gorilla/websocket"
)

var (
	configFile = flag.String("c", "./config.yml", "Path to config file")
	verbose    = flag.Bool("v", false, "Enable verbose output")
	useJson    = flag.Bool("json", false, "Use JSON output")
)

var (
	interrupt = make(chan os.Signal, 1)
	done      = make(chan struct{})
	messages  = make(chan Message)
)

var (
	lastOnline string
	lastId     string
)

type Message struct {
	Id         string      `json:"id"`
	Time       json.Number `json:"time"`
	Event      string      `json:"event"`
	Topic      string      `json:"topic"`
	Message    string      `json:"message"`
	Title      string      `json:"title"`
	Priority   int         `json:"priority"`
	Tags       []string    `json:"tags"`
	Click      string      `json:"click"`
	Icon       string      `json:"icon"`
	Attachment Attachment  `json:"attachment"`
}

type Attachment struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int    `json:"size"`
	Url  string `json:"url"`
}

func main() {
	flag.Parse()

	logLevel := new(slog.LevelVar)
	if *verbose {
		logLevel.Set(slog.LevelDebug)
	}

	textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	if *useJson {
		slog.SetDefault(slog.New(jsonHandler))
	} else {
		slog.SetDefault(slog.New(textHandler))
	}

	if err := config.LoadConfig(*configFile); err != nil {
		slog.Error("failed to parse config", "configFile", *configFile)
		slog.Debug(fmt.Sprintf("%v", err), "configFile", *configFile)
		return
	}
	slog.Debug("loaded config", "configFile", *configFile, "config", fmt.Sprintf("%#v", config.Config))

	slog.Info("starting ntfy-notify")

	var (
		writeWait  = 3 * time.Second
		pongWait   = time.Duration(config.Config.KeepAlive) * time.Second
		pingPeriod = (pongWait * 9) / 10
	)

	lastOnline = readCache("lastOnline")
	lastId = readCache("lastId")

	if config.Config.FetchMissed {
		go fetchCachedMsg()
	}

	u := url.URL{Scheme: "wss", Host: config.Config.Endpoint, Path: fmt.Sprintf("/%s/ws", config.Config.Topics)}
	d := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 10 * time.Second,
	}
	slog.Debug("subscribing to ntfy endpoint", "endpoint", config.Config.Endpoint, "topics", config.Config.Topics, "protocol", "wss")
	c, _, err := d.Dial(u.String(), config.Config.Header)
	if err != nil {
		slog.Error("failed to subscribe to ntfy endpoint", "endpoint", config.Config.Endpoint)
		slog.Debug(fmt.Sprintf("%v", err), "endpoint", config.Config.Endpoint)
		return
	}
	slog.Info("subscribed to ntfy endpoint", "endpoint", config.Config.Endpoint)

	signal.Notify(interrupt, os.Interrupt)
	ticker := time.NewTicker(pingPeriod)

	defer func() {
		ticker.Stop()
		c.Close()
	}()

	c.SetReadDeadline(time.Now().Add(pongWait))
	c.SetPongHandler(func(string) error {
		c.SetReadDeadline(time.Now().Add(pongWait))
		slog.Debug("received pong from ntfy endpoint")
		return nil
	})

	go func() {
		for {
			<-ticker.C
			c.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}()

	go func() {
		defer func() {
			slog.Info("ntfy closed connection")
			close(done)
		}()
		for {
			var message Message
			err := c.ReadJSON(&message)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
					slog.Error("cannot read ntfy message")
					slog.Debug(fmt.Sprintf("%v", err), "message", fmt.Sprintf("%#v", message))
				}
				return
			}
			slog.Debug("received ws message", "message", fmt.Sprintf("%#v", message))
			messages <- message
		}
	}()

	defer func() {
		writeCache("lastOnline", &lastOnline)
		writeCache("lastId", &lastId)
	}()

	slog.Info("started ntfy-notify")

	for {
		select {
		case <-done:
			return
		case <-interrupt:
			slog.Info("closing ntfy-notify")

			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				slog.Error("failed to send close message to ntfy")
				slog.Debug(fmt.Sprintf("%v", err))
			}

			select {
			case <-done:
			case <-time.After(time.Second):
				slog.Debug("ntfy endpoint timed out")
			}
			slog.Info("bye")
			return
		case message := <-messages:
			if message.Event == "message" {
				slog.Info("received ntfy message", "messageId", message.Id)
				notify(message)
			}
		}
	}
}

func fetchCachedMsg() {
	slog.Info("fetching cached messages")

	var count = 0

	client := &http.Client{}
	req, _ := http.NewRequest("GET", fmt.Sprintf("https://%s/%s/json?since=%s&poll=1", config.Config.Endpoint, config.Config.Topics, lastOnline), nil)
	req.Header = config.Config.Header

	resp, err := client.Do(req)
	if err != nil {
		slog.Error("failed to fetch cached message")
		slog.Debug(fmt.Sprintf("%v", err))
	} else {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			var message Message
			err := json.Unmarshal(scanner.Bytes(), &message)
			if err != nil {
				slog.Error("failed to parse cached message")
				slog.Debug(fmt.Sprintf("%v", err))
			} else {
				if message.Id != lastId {
					notify(message)
					count++
				}
			}
		}
		if err := scanner.Err(); err != nil {
			slog.Error("failed to fetch cached message")
			slog.Debug(fmt.Sprintf("%v", err))
		}
	}
	defer resp.Body.Close()
	slog.Info("fetched cached messages", "count", count)
}

func readCache(key string) string {
	data, err := os.ReadFile(config.Config.CacheDir + "/" + key)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Error("failed to read cache "+key, "cacheDir", config.Config.CacheDir)
		slog.Debug(fmt.Sprintf("%v", err), "cacheDir", config.Config.CacheDir)
		return ""
	}
	return string(data)
}

func writeCache(key string, data *string) {
	err := os.WriteFile(config.Config.CacheDir+"/"+key, []byte(*data), 0644)
	if err != nil {
		slog.Error("failed to save cache "+key, "cacheDir", config.Config.CacheDir)
		slog.Debug(fmt.Sprintf("%v", err), "cacheDir", config.Config.CacheDir)
	}
}

func notify(message Message) {
	slog.Debug("notifying message", "message", fmt.Sprintf("%#v", message))
	if err := notifier.Notify(message.Title, message.Message); err != nil {
		slog.Error("failed to notify message", "message", fmt.Sprintf("%#v", message))
		slog.Debug(fmt.Sprintf("%v", err), "message", fmt.Sprintf("%#v", message))
	} else {
		lastOnline = string(message.Time)
		lastId = message.Id
		slog.Debug("notified message", "message", fmt.Sprintf("%#v", message))
	}
}

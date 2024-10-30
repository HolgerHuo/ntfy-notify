package config

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var Version = "dev"

type config struct {
	Endpoint    string `yaml:"endpoint"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	Token       string `yaml:"token"`
	Topics      string `yaml:"topics"`
	CacheDir    string `yaml:"cacheDir"`
	UserAgent   string `yaml:"userAgent"`
	KeepAlive   int32  `yaml:"keepAlive"`
	FetchMissed bool   `yaml:"fetchMissed"`
	Header      http.Header
}

var (
	Config = config{
		Endpoint:    "ntfy.sh:443",
		Topics:      "ntfy_notify_announcement,ntfy_notify_release",
		UserAgent:   "ntfy-notify/" + Version,
		KeepAlive:   300,
		FetchMissed: true,
	}
)

func LoadConfig(configFile string) error {
	file, err := os.Open(configFile)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&Config); err != nil {
		return err
	}

	if Config.CacheDir == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return err
		}

		Config.CacheDir = filepath.Join(cacheDir, "ntfy-notify")
	}

	if err := os.MkdirAll(Config.CacheDir, 0700); err != nil {
		return err
	}

	var authorizationHeader string

	if Config.Token != "" {
		authorizationHeader = "Bearer " + Config.Token
	} else if Config.Username != "" && Config.Password != "" {
		authorizationHeader = "Basic " + base64.StdEncoding.EncodeToString([]byte(Config.Username+":"+Config.Password))
	}

	if authorizationHeader != "" {
		Config.Header = http.Header{"Authorization": {authorizationHeader}, "User-Agent": {Config.UserAgent}}
	} else {
		Config.Header = http.Header{"User-Agent": {Config.UserAgent}}
	}

	return nil
}

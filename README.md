# ntfy-notify | Send ntfy messages to Linux desktop notifications

![GitHub last commit](https://img.shields.io/github/last-commit/holgerhuo/ntfy-notify)[![Release](https://img.shields.io/github/release/holgerhuo/ntfy-notify.svg?color=success&style=flat-square)](https://github.com/holgerhuo/ntfy-notify/releases/latest)[![Release](https://github.com/HolgerHuo/ntfy-notify/actions/workflows/release.yml/badge.svg)](https://github.com/HolgerHuo/ntfy-notify/actions/workflows/release.yml)

`ntfy-notify` is a simple Linux daemon to send subscribed ntfy messages to your Linux desktop via `notify-send`. 

## Features

- Fetch missed notifications when you were offline
- WebSocket connection

## To-Dos

- [] Click actions
- [] Support icon and attachment
- [] Detect Internet connectivity

## Usage

1. Download ntfy-notify from [releases page](https://github.com/HolgerHuo/ntfy-notify/releases/latest)

2. Create [`config.yml`](https://github.com/HolgerHuo/ntfy-notify/blob/main/config.yml.sample)

```yaml
endpoint: ntfy.sh:443
#username: ntfy_user
#password: ntfy_password
topics: ntfy_notify_announcement,ntfy_notify_release
```

3. Run ntfy-notify

```bash
ntfy-notify -c ./config.yml
```

## Tips

If your Internet connection is not so stable, you could use nm-dispatcher or similar tools to manage this application's lifecycle. Please DO feedback on how to reliably and efficiently detect network connection and stop/restart `ntfy-notify` process.

```ini
[Unit]
Description=ntfy-notify
After=network.target NetworkManager.service

[Service]
Type=simple
Restart=always
ExecStart=/usr/bin/ntfy-notify -c %h/.config/ntfy-notify/config.yml -json
KillSignal=SIGINT

[Install]
WantedBy=default.target
```

## License

GPLv3 Holger Huo
Для установки:

    useradd -r -s /sbin/nologin -d /var/lib/wifi_bot wifi_bot
    mkdir -p /var/lib/wifi_bot/configs /etc/wifi_bot
    cp wifi_bot /usr/local/bin/
    cp configs/config.yaml /var/lib/wifi_bot/configs/

    # .env с секретами кладётся в /var/lib/wifi_bot/.env

    cp deploy/wifi_bot.service /etc/systemd/system/
    systemctl daemon-reload && systemctl enable --now wifi_bot

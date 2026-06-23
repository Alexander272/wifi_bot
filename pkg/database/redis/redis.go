package redis

import (
	"fmt"

	"github.com/go-redis/redis/v8"
)

type Config struct {
	Host     string
	Port     string
	Password string
	DB       int
}

func NewRedisClient(conf *Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     conf.Host + ":" + conf.Port,
		Password: conf.Password,
		DB:       conf.DB,
	})

	if err := client.Ping(client.Context()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return client, nil
}

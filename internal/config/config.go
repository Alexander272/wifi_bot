package config

import (
	"fmt"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Environment string           `yaml:"environment" env:"APP_ENV" env-default:"dev"`
	TestMode    bool             `yaml:"test_mode" env:"TEST_MODE" env-default:"false"`
	LogLevel    string           `yaml:"log_level" env-default:"info"`
	LogSource   bool             `yaml:"log_source" env-default:"false"`
	Http        HttpConfig       `yaml:"http"`
	Redis       RedisConfig      `yaml:"redis"`
	Postgres    PostgresConfig   `yaml:"postgres"`
	Mikrotik    MikrotikConfig   `yaml:"mikrotik"`
	Mattermost  MattermostConfig `yaml:"mattermost"`
}

type HttpConfig struct {
	Host           string        `yaml:"host" env:"HOST" env-default:"0.0.0.0"`
	Port           string        `yaml:"port" env:"PORT" env-default:"8080"`
	ReadTimeout    time.Duration `yaml:"read_timeout" env-default:"10s"`
	WriteTimeout   time.Duration `yaml:"write_timeout" env-default:"15s"`
	MaxHeaderBytes int           `yaml:"max_header_bytes" env-default:"1"`
}

type RedisConfig struct {
	Host     string `yaml:"host" env:"REDIS_HOST" env-default:"127.0.0.1"`
	Port     string `yaml:"port" env:"REDIS_PORT" env-default:"6379"`
	DB       int    `yaml:"db" env:"REDIS_DB" env-default:"0"`
	Password string `env:"REDIS_PASSWORD"`
}

type PostgresConfig struct {
	Host     string `yaml:"host" env:"POSTGRES_HOST" env-default:"127.0.0.1"`
	Port     string `yaml:"port" env:"POSTGRES_PORT" env-default:"5432"`
	Username string `yaml:"username" env:"POSTGRES_NAME" env-default:"postgres"`
	Password string `env:"POSTGRES_PASSWORD"`
	DbName   string `yaml:"db_name" env:"POSTGRES_DB" env-default:"wifi_bot"`
	SSLMode  string `yaml:"ssl_mode" env:"POSTGRES_SSL" env-default:"disable"`
}

type MikrotikConfig struct {
	APIVersion      string        `yaml:"api_version" env:"MIKROTIK_API_VERSION" env-default:"v7"`
	Host            string        `yaml:"host" env:"MIKROTIK_HOST" env-default:"192.168.88.1"`
	APIPort         int           `yaml:"api_port" env:"MIKROTIK_API_PORT" env-default:"443"`
	APIUsername     string        `yaml:"api_username" env:"MIKROTIK_API_USER"`
	APIPassword     string        `env:"MIKROTIK_API_PASS"`
	UseSSL          bool          `yaml:"use_ssl" env:"MIKROTIK_USE_SSL" env-default:"false"`
	AuthMethod      string        `yaml:"auth_method" env:"MIKROTIK_AUTH_METHOD" env-default:"chap"`
	AuthTimeout     time.Duration `yaml:"auth_timeout" env:"MIKROTIK_AUTH_TIMEOUT" env-default:"10s"`
	CollectInterval time.Duration `yaml:"collect_interval" env:"COLLECT_INTERVAL" env-default:"30s"`
	AllowReuse      bool          `yaml:"allow_reuse" env:"MIKROTIK_ALLOW_REUSE" env-default:"false"`
	AddressList     string        `yaml:"address_list" env:"MIKROTIK_ADDRESS_LIST"`
}

type MattermostConfig struct {
	Token       string        `env:"MOST_TOKEN"`
	Server      string        `env:"MOST_SERVER"`
	CodeTTL     time.Duration `yaml:"code_ttl" env:"CODE_TTL" env-default:"24h"`
	BotUsername string        `yaml:"bot_username" env:"MOST_BOT_USERNAME" env-default:"wifi_bot"`
	TeamName    string        `yaml:"team_name" env:"MOST_TEAM_NAME"`
	AdminIDs    []string      `yaml:"admin_ids"`
}

func Init(path string) (*Config, error) {
	var conf Config
	if err := cleanenv.ReadConfig(path, &conf); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return &conf, nil
}

package config

import (
	"encoding/hex"
	"flag"
	"os"

	"github.com/4aleksei/gmart/internal/common/logger"
	"github.com/4aleksei/gmart/internal/common/utils"
)

type Config struct {
	Address              string
	DatabaseURI          string
	AccrualSystemAddress string
	Key                  string
	KeySignature         string
	LCfg                 logger.Config
	PollInterval         int64
	RateLimit            int64
}

const (
	addressDefault      string = ":8090"
	levelDefault        string = "debug"
	databaseURIDefault  string = ""
	accrualSAddDef      string = "localhost:8100"
	keyDefault          string = ""
	keySignatureDefault string = ""
	pollIntervalDefault int64  = 2
	rateLimitDefault    int64  = 2

	defaultKeyLen int = 16
)

func GetConfig() *Config {
	cfg := new(Config)

	flag.StringVar(&cfg.Address, "a", addressDefault, "address and port to run server gopthermart")
	flag.StringVar(&cfg.LCfg.Level, "v", levelDefault, "level of logging")
	flag.StringVar(&cfg.AccrualSystemAddress, "r", accrualSAddDef, "accrual client`s address and port")

	flag.StringVar(&cfg.DatabaseURI, "d", databaseURIDefault, "database postgres URI")
	flag.Int64Var(&cfg.PollInterval, "i", pollIntervalDefault, "interval bd  request for accrual")
	flag.Int64Var(&cfg.RateLimit, "l", rateLimitDefault, "workers count")
	flag.StringVar(&cfg.Key, "k", keyDefault, "key for jwt signature")
	flag.StringVar(&cfg.KeySignature, "s", keySignatureDefault, "key for signature")
	flag.Parse()

	if envKey := os.Getenv("KEY"); cfg.Key == keyDefault && envKey != "" {
		cfg.Key = envKey
	}

	if envSignatureKey := os.Getenv("KEY_SIGNATURE"); cfg.KeySignature == keySignatureDefault && envSignatureKey != "" {
		cfg.KeySignature = envSignatureKey
	}

	if envRunAddr := os.Getenv("RUN_ADDRESS"); cfg.Address == addressDefault && envRunAddr != "" {
		cfg.Address = envRunAddr
	}

	if envdatabaseURI := os.Getenv("DATABASE_URI"); cfg.DatabaseURI == databaseURIDefault && envdatabaseURI != "" {
		cfg.DatabaseURI = envdatabaseURI
	}

	if envaSysA := os.Getenv("ACCRUAL_SYSTEM_ADDRESS"); cfg.AccrualSystemAddress == accrualSAddDef && envaSysA != "" {
		cfg.AccrualSystemAddress = envaSysA
	}

	if cfg.Key == "" {
		b, err := utils.GenerateRandom(defaultKeyLen)
		if err != nil {
			panic("no key")
		}
		cfg.Key = hex.EncodeToString(b)
	}

	if cfg.KeySignature == "" {
		b, err := utils.GenerateRandom(defaultKeyLen)
		if err != nil {
			panic("no key")
		}
		cfg.KeySignature = hex.EncodeToString(b)
	}
	return cfg
}

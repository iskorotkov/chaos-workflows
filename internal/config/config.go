package config

import (
	"errors"
	"fmt"
	"github.com/caarlos0/env"
	"math/rand"
	"reflect"
)

var (
	ParseError = errors.New("couldn't parse config from env vars")
)

type Config struct {
	ArgoServer  string `env:"ARGO_SERVER"`
	Development bool   `env:"DEVELOPMENT"`
}

func (c Config) Generate(r *rand.Rand, _ int) reflect.Value {
	rs := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, r.Intn(100))
	}
	return reflect.ValueOf(Config{
		ArgoServer:  rs("argo-server"),
		Development: r.Int()%2 == 0,
	})
}

func FromEnvironment() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, ParseError
	}

	return cfg, nil
}

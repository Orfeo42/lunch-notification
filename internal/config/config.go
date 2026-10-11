package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/orfeo42/lunch-notification/internal/schedule"
)

type Config struct {
	RunAt                 schedule.Clock `env:"RUN_AT"                  envDefault:"20:00"`
	Timezone              time.Location  `env:"TIMEZONE"                envDefault:"Europe/Rome"`
	Source                string         `env:"SOURCE"                  envDefault:"gsheets"`
	Tasks                 string         `env:"TASKS"                   envDefault:"gtasks"`
	SheetID               string         `env:"SHEET_ID"`
	SheetRange            string         `env:"SHEET_RANGE"             envDefault:"Menù!A1:H4"`
	TasklistID            string         `env:"TASKLIST_ID"             envDefault:"@default"`
	GoogleCredentialsFile string         `env:"GOOGLE_CREDENTIALS_FILE" envDefault:"/data/credentials.json"`
	GoogleTokenFile       string         `env:"GOOGLE_TOKEN_FILE"       envDefault:"/data/token.json"`
}

func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("parse env config: %w", err)
	}
	return cfg, nil
}

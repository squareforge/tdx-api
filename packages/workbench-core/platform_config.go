package workbench

import (
	"database/sql"
	"errors"
	"strings"
)

const (
	PlatformConfigID = "default"
	DefaultRiseColor = "#c93d35"
	DefaultFallColor = "#16805b"
)

func DefaultPlatformConfig() PlatformConfig {
	return PlatformConfig{ID: PlatformConfigID, RiseColor: DefaultRiseColor, FallColor: DefaultFallColor}
}

func normalizePlatformColor(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 7 || value[0] != '#' {
		return fallback
	}
	for _, char := range value[1:] {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return fallback
		}
	}
	return value
}

func normalizePlatformConfig(config PlatformConfig) (PlatformConfig, error) {
	config.ID = PlatformConfigID
	config.RiseColor = normalizePlatformColor(config.RiseColor, "")
	config.FallColor = normalizePlatformColor(config.FallColor, "")
	if config.RiseColor == "" || config.FallColor == "" {
		return config, errors.New("涨跌颜色必须是6位十六进制颜色，例如 #c93d35")
	}
	return config, nil
}

func (s *AppStore) GetPlatformConfig() (PlatformConfig, error) {
	config := DefaultPlatformConfig()
	err := s.db.QueryRow(`SELECT id,rise_color,fall_color,updated_at FROM platform_config WHERE id=?`, PlatformConfigID).
		Scan(&config.ID, &config.RiseColor, &config.FallColor, &config.UpdatedAt)
	if err == nil {
		config.RiseColor = normalizePlatformColor(config.RiseColor, DefaultRiseColor)
		config.FallColor = normalizePlatformColor(config.FallColor, DefaultFallColor)
		return config, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return config, err
	}
	return config, nil
}

func (s *AppStore) UpsertPlatformConfig(config PlatformConfig) (PlatformConfig, error) {
	config, err := normalizePlatformConfig(config)
	if err != nil {
		return config, err
	}
	config.UpdatedAt = NowText()
	_, err = s.db.Exec(`INSERT INTO platform_config (id,rise_color,fall_color,updated_at) VALUES (?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET rise_color=excluded.rise_color,fall_color=excluded.fall_color,updated_at=excluded.updated_at`,
		config.ID, config.RiseColor, config.FallColor, config.UpdatedAt)
	return config, err
}

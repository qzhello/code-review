package store

import (
	"encoding/json"
	"fmt"
)

// ModelConfig represents a saved model configuration.
type ModelConfig struct {
	Name     string                 `json:"name"`
	Provider string                 `json:"provider"`
	Model    string                 `json:"model"`
	Config   map[string]interface{} `json:"config"`
}

// SaveModelConfig stores a named model configuration.
func (d *DB) SaveModelConfig(name, provider, model string, config map[string]interface{}) error {
	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	_, err = d.db.Exec(
		`INSERT OR REPLACE INTO model_configs (name, provider, model, config_json) VALUES (?, ?, ?, ?)`,
		name, provider, model, string(data),
	)
	return err
}

// GetModelConfig retrieves a named model configuration.
func (d *DB) GetModelConfig(name string) (*ModelConfig, error) {
	var mc ModelConfig
	var configJSON string

	err := d.db.QueryRow(
		`SELECT name, provider, model, config_json FROM model_configs WHERE name = ?`,
		name,
	).Scan(&mc.Name, &mc.Provider, &mc.Model, &configJSON)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(configJSON), &mc.Config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &mc, nil
}

// ListModelConfigs returns all saved model configurations.
func (d *DB) ListModelConfigs() ([]ModelConfig, error) {
	rows, err := d.db.Query(`SELECT name, provider, model, config_json FROM model_configs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []ModelConfig
	for rows.Next() {
		var mc ModelConfig
		var configJSON string
		if err := rows.Scan(&mc.Name, &mc.Provider, &mc.Model, &configJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(configJSON), &mc.Config); err != nil {
			continue
		}
		configs = append(configs, mc)
	}

	return configs, rows.Err()
}

// DeleteModelConfig removes a named model configuration.
func (d *DB) DeleteModelConfig(name string) error {
	_, err := d.db.Exec(`DELETE FROM model_configs WHERE name = ?`, name)
	return err
}

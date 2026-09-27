package provider

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Config struct {
	Spec   atom.ProviderSpec
	Prices map[string]atom.Prices
	Models []atom.ModelInfo
}

type fileEntry struct {
	Name          string               `json:"name"`
	APIURL        string               `json:"api_url"`
	ModelListURL  string               `json:"model_list_url"`
	PriceTableURL string               `json:"price_table_url"`
	RefreshHours  int                  `json:"refresh_hours"`
	Prices        map[string]filePrice `json:"prices"`
	Models        []fileModel          `json:"models"`
}

type filePrice struct {
	Currency   string  `json:"currency"`
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

type fileModel struct {
	ID         string   `json:"id"`
	Level      int      `json:"level"`
	Input      []string `json:"input"`
	Output     []string `json:"output"`
	Tools      bool     `json:"tools"`
	ContextMax int      `json:"context_max"`
}

type fileConfig struct {
	Providers []fileEntry `json:"providers"`
}

func LoadFile(path string) ([]Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file fileConfig
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	var configs []Config
	for _, entry := range file.Providers {
		if entry.Name == "" {
			return nil, errors.New("provider: a provider in the file has no name")
		}
		prices := map[string]atom.Prices{}
		for id, price := range entry.Prices {
			currency := price.Currency
			if currency == "" {
				currency = "USD"
			}
			prices[id] = atom.Prices{
				Currency:   currency,
				Input:      price.Input,
				Output:     price.Output,
				CacheRead:  price.CacheRead,
				CacheWrite: price.CacheWrite,
			}
		}
		var models []atom.ModelInfo
		for _, model := range entry.Models {
			info := atom.ModelInfo{
				ID:         model.ID,
				Level:      model.Level,
				Tools:      model.Tools,
				ContextMax: model.ContextMax,
			}
			for _, media := range model.Input {
				info.Input = append(info.Input, atom.MediaType(media))
			}
			for _, media := range model.Output {
				info.Output = append(info.Output, atom.MediaType(media))
			}
			if price, ok := prices[model.ID]; ok {
				info.Prices = &price
			}
			models = append(models, info)
		}
		configs = append(configs, Config{
			Spec: atom.ProviderSpec{
				Name:          entry.Name,
				APIURL:        entry.APIURL,
				ModelListURL:  entry.ModelListURL,
				PriceTableURL: entry.PriceTableURL,
				Interval:      time.Duration(entry.RefreshHours) * time.Hour,
			},
			Prices: prices,
			Models: models,
		})
	}
	return configs, nil
}

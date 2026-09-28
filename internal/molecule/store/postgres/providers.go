package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type providers struct{ store *Store }

func (providerStore *providers) Save(operationContext context.Context, providerSpec atom.ProviderSpec) error {
	_, operationError := providerStore.store.pool.Exec(operationContext, `
		INSERT INTO providers (name, api_url, model_list_url, price_table_url, interval_seconds)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (name) DO UPDATE SET
			api_url = EXCLUDED.api_url,
			model_list_url = EXCLUDED.model_list_url,
			price_table_url = EXCLUDED.price_table_url,
			interval_seconds = EXCLUDED.interval_seconds`,
		providerSpec.Name, providerSpec.APIURL, providerSpec.ModelListURL, providerSpec.PriceTableURL, int64(providerSpec.Interval/time.Second))
	return operationError
}

func (providerStore *providers) Get(operationContext context.Context, identifier string) (atom.ProviderSpec, error) {
	var providerSpec atom.ProviderSpec
	var seconds int64
	operationError := providerStore.store.pool.QueryRow(operationContext, `
		SELECT name, api_url, model_list_url, price_table_url, interval_seconds
		FROM providers WHERE name = $1`, identifier).
		Scan(&providerSpec.Name, &providerSpec.APIURL, &providerSpec.ModelListURL, &providerSpec.PriceTableURL, &seconds)
	providerSpec.Interval = time.Duration(seconds) * time.Second
	return providerSpec, operationError
}

func (providerStore *providers) All(operationContext context.Context) ([]atom.ProviderSpec, error) {
	rows, operationError := providerStore.store.pool.Query(operationContext, `
		SELECT name, api_url, model_list_url, price_table_url, interval_seconds
		FROM providers ORDER BY name`)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var list []atom.ProviderSpec
	for rows.Next() {
		var providerSpec atom.ProviderSpec
		var seconds int64
		if operationError := rows.Scan(&providerSpec.Name, &providerSpec.APIURL, &providerSpec.ModelListURL, &providerSpec.PriceTableURL, &seconds); operationError != nil {
			return nil, operationError
		}
		providerSpec.Interval = time.Duration(seconds) * time.Second
		list = append(list, providerSpec)
	}
	return list, rows.Err()
}

func (providerStore *providers) Delete(operationContext context.Context, identifier string) error {
	_, operationError := providerStore.store.pool.Exec(operationContext, `DELETE FROM providers WHERE name = $1`, identifier)
	return operationError
}

func (providerStore *providers) SaveModels(operationContext context.Context, provider string, models []atom.ModelInfo) error {
	tx, operationError := providerStore.store.pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer tx.Rollback(operationContext)
	if _, operationError := tx.Exec(operationContext, `DELETE FROM models WHERE provider = $1`, provider); operationError != nil {
		return operationError
	}
	for _, model := range models {
		input, _ := json.Marshal(model.Input)
		output, _ := json.Marshal(model.Output)
		var prices any
		if model.Prices != nil {
			data, _ := json.Marshal(model.Prices)
			prices = data
		}
		if _, operationError := tx.Exec(operationContext, `
			INSERT INTO models (provider, id, level, input, output, tools, context_max, prices)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			provider, model.ID, model.Level, input, output, model.Tools, model.ContextMax, prices); operationError != nil {
			return operationError
		}
	}
	return tx.Commit(operationContext)
}

func (providerStore *providers) Models(operationContext context.Context, provider string) ([]atom.ModelInfo, error) {
	rows, operationError := providerStore.store.pool.Query(operationContext, `
		SELECT id, level, input, output, tools, context_max, prices
		FROM models WHERE provider = $1 ORDER BY id`, provider)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var list []atom.ModelInfo
	for rows.Next() {
		var model atom.ModelInfo
		var input, output, prices []byte
		if operationError := rows.Scan(&model.ID, &model.Level, &input, &output, &model.Tools, &model.ContextMax, &prices); operationError != nil {
			return nil, operationError
		}
		_ = json.Unmarshal(input, &model.Input)
		_ = json.Unmarshal(output, &model.Output)
		if len(prices) > 0 {
			_ = json.Unmarshal(prices, &model.Prices)
		}
		list = append(list, model)
	}
	return list, rows.Err()
}

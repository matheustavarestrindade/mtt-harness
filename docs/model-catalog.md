# Model Data, Reasoning, and Prices

## Model Data

The standard configuration uses the Models.dev JSON API.

The model catalog is part of OpenCode. Pi uses the model catalog.

The provider endpoint gives available model IDs. Models.dev supplies model data and token prices. The model catalog must not add provider model IDs to the provider response.

Add the fields below to a provider entry in `providers.json`:

```json
{
  "metadata_url": "https://models.dev/api.json",
  "metadata_format": "models_dev",
  "metadata_provider": "openai",
  "billing": "tokens"
}
```

The `metadata_provider` value selects the provider key in the model catalog. The URL can also be a local file path. The source limit is 32 MiB. The harness does not send provider credentials to the metadata source.

The value `refresh_hours` sets the interval for model data. The route `POST /providers/{id}/refresh` gets new model data. An error keeps the last correct model list. Model data uses the sequence that follows:

- Model configuration in `providers.json`.
- The provider model response.
- The metadata source.
- Provider defaults in `providers.json`.
- The model cache.

## Codex Model Selection

The Codex model endpoint uses the `client_version` query value. The value is part of `model_list_url` in `providers.json`. The endpoint can remove models because of the client version. The configuration uses Codex `0.162.1`.

The harness reads the `visibility` field. It includes models with `list` and models without the field. It does not include models with `hide` or `none`.

The `service_tiers` list gives model presets. The model preset ID is `MODEL_ID@TIER_ID`. The model name includes the service tier name. The fields `APIModel` and `ServiceTier` give the provider request values for `model` and `service_tier`. Session data, model permissions, and usage keep the model preset ID.

A model preset uses the provider model data. Prices can be different. You can set prices in the JSON file for the model preset ID. When the provider removes a model or service tier, the harness removes the related model preset. The database keeps the request values in the model cache.

With `chatgpt` authentication, the adapter does not send `max_output_tokens`. The harness keeps the output token limit.

## Reasoning Effort

The API gives available values in `ModelInfo.ReasoningEfforts`. Values are different for different models. The model name does not supply the values.

Supply `reasoning_effort` when you start a session, or use the session route:

```http
PUT /sessions/SESSION_ID/reasoning
Content-Type: application/json
Authorization: Bearer TOKEN

{"effort":"high"}
```

An empty string uses the model default. A selection in the database applies to the next model request. It does not stop an active request. A child agent uses `reasoning_effort` or the model default.

The Responses protocol uses `reasoning.effort`. Chat Completions uses `reasoning_effort`.

Responses uses `none` for the value `toggle` in the model catalog. For Chat Completions, the model data must contain the reasoning effort `none` before the adapter can use it.

Model configuration can supply `reasoning`, `reasoning_efforts`, `default_reasoning_effort`, and `reasoning_summary`. Use `reasoning_summary` only when the model has reasoning summaries. The standard OpenAI configuration selects `auto` for specified reasoning models. The Codex model response gives the available reasoning summary formats.

## Change the Session Model

The route `PUT /sessions/{id}/model` changes the model during a conversation:

```json
{"model":"provider/model","allow_compaction":false}
```

The API accepts a model with the same or a larger context limit. For a smaller context limit, the user must accept context compaction. The API gives `409` with the code `context_compaction_required`. After the user accepts context compaction, send the request with `allow_compaction: true`.

The new model must have a context limit. The selection applies to the next model request. When the context is too large, the harness removes the initial turn from the request. The database keeps the full message history. The last turn and system messages stay in the request.

The model and reasoning effort change together. If the new model does not have the previous reasoning effort, the selection uses the model default.

## Reasoning Text

OpenAI supplies reasoning summaries. DeepSeek supplies reasoning text. The API gives the text in `Message.Reasoning` and in model events. The UI section with the label `Thinking` is closed by default.

The database keeps continuation data. The API does not send continuation data. Only the adapter for the same provider uses the data in a subsequent model request.

## Prices

The prices are for 1,000,000 tokens. The harness uses local price configuration before the price table, provider model response, and metadata source. A price of 0 is different from price data that is not available.

A price tier applies when the full input tokens are above the price tier limit. Input tokens include cache tokens. Output tokens include reasoning tokens. A reasoning price can replace the output price for reasoning tokens.

A provider cost replaces a cost estimate. A cost estimate has `Estimated: true`. A total with a cost estimate also has `Estimated: true`. If the response has cache tokens without price data, a cost estimate is not available.

Use `billing: subscription` to prevent API token cost estimates. The standard provider entry for ChatGPT uses `subscription`. The statistics do not include subscription costs.

Provider costs can be different from cost estimates. DeepSeek prices can change with time. The UI identifies cost estimates. Use the provider account to get the provider cost.

## Checks

The test suite uses model IDs for the test and a local HTTP server. The command that follows also reads the live model catalog:

```sh
MTT_TEST_LIVE_MODEL_METADATA=1 go test -count=1 ./internal/molecule/provider -run TestModelsDevLiveCatalog -v
```

## Source Documents

- [Models.dev API](https://models.dev/#api)
- [Pi model data](https://github.com/badlogic/pi-mono/blob/main/packages/ai/scripts/generate-models.ts)
- [OpenAI reasoning](https://developers.openai.com/api/docs/guides/reasoning)
- [Codex model data](https://github.com/openai/codex/blob/rust-v0.162.1/codex-rs/protocol/src/openai_models.rs)
- [OpenCode Codex adapter](https://github.com/anomalyco/opencode/blob/dev/packages/opencode/src/plugin/openai/codex.ts)
- [DeepSeek reasoning](https://api-docs.deepseek.com/guides/thinking_mode)
- [DeepSeek Responses API](https://api-docs.deepseek.com/guides/responses_api)
- [DeepSeek prices](https://api-docs.deepseek.com/quick_start/pricing)

# mtt-harness Specification

Version: 0.2
Date: 2026-09-25

## 1. Purpose

mtt-harness is a program for AI agents. The program is one loop. The loop connects an AI model to tools and features.

The specification has 2 goals:

- Keep the core small.
- Keep the boundaries open.

The core contains the loop and 4 boundaries. A plugin can attach to a boundary. A plugin can read a value, change a value, or stop the flow. Thus, the user can change the behavior of the harness. The user does not change the core code.

The harness is one program. The program can have many instances. Section 5 tells about instances.

## 2. Terms and Definitions

Section 2 gives the meaning of the special terms for mtt-harness:

- `harness`: the program in the specification.
- `instance`: one workspace with sessions, messages, and processes.
- `tool`: a function which the model can start.
- `tool call`: one message from the model which starts a tool.
- `tool result`: the output of one tool call.
- `session`: the data from one set of messages with the model.
- `turn`: one model call and the tool plan from the model call.
- `stage`: a point in the loop where the harness gives a value to plugins.
- `boundary`: a group of stages with one purpose.
- `plugin`: a unit of code which changes the harness at a stage.
- `process`: a program which the harness starts.
- `middleware`: a function which reads a stage value and gives a new value.
- `watcher`: a function which reads process output events.
- `notification`: a message about the status of a process.
- `parent session`: the session which starts an agent.
- `child agent`: an agent which a parent session starts.
- `millisecond`: a unit of time equal to 0.001 second.
- `model list`: the models which an instance can use.
- `tool group`: the tools which the model can start in a session.
- `category`: a group of tools with the same purpose.
- `usage`: the tokens and the cost of a model call.
- `statistics`: the data about the operation of the harness.
- `MCP server`: a server that gives tools to the harness through the MCP protocol.
- `cost`: the value of a model call in a currency.

## 3. System Structure

The harness has one loop. The loop has 4 boundaries:

- The context boundary prepares the data for the model.
- The model boundary sends the request and receives the response.
- The plan boundary reads the tool plan from the response.
- The tool boundary runs the tools and collects the output.

The structure of the loop is:

```
      +------------------------------------------------------+
      |                        THE LOOP                      |
      |                                                      |
      |   context ---> model ---> plan ---> tool -------+    |
      |      ^                                         |     |
      |      +-----------------------------------------+     |
      +------------------------------------------------------+
```

The harness is one program with an API. The API can have many instances:

```
      +--------------------------------------------------+
      |                   THE HARNESS                    |
      |                                                  |
      |   instance A       instance B       instance C   |
      |   /project-a       /project-b       /project-c   |
      |   sessions...      sessions...      sessions...  |
      +--------------------------------------------------+
```

## 4. The Loop

The basic loop is:

```
run(instance, session):
    repeat
        context = build_context(session)
        request = pipe("model.request", context)
        stream  = model.stream(request)               # boundary 2

        tasks = []
        for part in stream:
            if part is a tool call with the full input:
                call  = pipe("action.plan", part)     # boundary 3
                input = pipe("tool.input", call)
                tasks.append(start(run_tool, input))  # boundary 4

        message = pipe("model.response", stream)
        write(message)

        results = wait_all(tasks)                     # tool plan sequence
        for completed_result in results:
            result = pipe("tool.result", completed_result)
            write(result)

        if tasks is empty:
            return
```

Requirements:

- R1: The loop must prepare a context for a model call.
- R2: The loop must send one request to the model for a turn.
- R3: The loop must write the model message before the next model call.
- R4: The loop must start a tool call when the harness reads the full input of the tool call.
- R5: The loop must run the tool calls of one message at the same time.
- R6: The loop must write a tool result when the tool call stops.
- R7: The loop must wait for the full tool result group before the next model call. One request must contain the full group.
- R8: The loop must stop when the model message does not have a tool call.
- R9: The loop must send the value of a stage through the pipeline of the stage.
- R10: The harness must run the messages of a session in sequence.
- R11: The harness must keep the new messages of a session in a queue.

The messages of a session wait in a queue. The harness runs one message at a time. Thus, the user can send a new message while the harness runs a message.

The database keeps a message in the queue before the API gives status `202`. The queue has the limits:

```
waiting messages per session: 128
waiting and running messages for the harness: 4096
```

The next program start reads the queue. A message which did not start can continue. The harness writes `run.interrupted` for a turn which cannot continue after the program stops. The harness does not run the tool plan of the turn again.

The event record keeps a tool result when the tool stops. The message record keeps the sequence from the tool plan.

The harness starts a tool when the full input is available during the model stream. A tool result does not start a new model request. The next model request contains the full tool result group. The group includes a tool error or a denied tool result.

### 4.1 Session Coordinator

The session coordinator keeps the queue state of one session. One goroutine runs the command loop of the coordinator. Only the command handlers of the coordinator change the queue state.

```
API requests -> queue directory -> session command inbox
                                      |
                               sessionCoordinator.runCommandLoop
                                      |
                           model and database workers
                                      |
                              completion messages
```

The command inbox has space for 32 commands. A response channel has space for one response. A caller can cancel a command request. The coordinator can send the response after the caller cancels the request.

The coordinator starts a worker for a model call or a database operation. The worker sends a completion message to the coordinator. The coordinator does not wait for the worker in a command handler. Thus, the coordinator can examine a status request or a `cancelCurrent` command during a database operation.

The mode of the coordinator is one of 6 values:

- `accepting`: the coordinator can put new messages in the queue.
- `reverting`: the coordinator cannot put new messages in the queue until the API completes the revert operation.
- `stopped`: messages can stay in the database, but a new turn cannot start.
- `failed`: the coordinator keeps an operation error and does not start a new turn.
- `closing`: the coordinator stops new commands which can start work. The coordinator waits for the workers before it stops.
- `deleting`: the coordinator stops new input while a worker removes a conversation.

The queue directory keeps the session coordinators. One goroutine changes the directory. A worker runs a database operation or a plugin callback. A command handler must not run a database operation or a plugin callback.

The queue keeps a coordinator until the queue closes or the conversation is removed. The `Close` operation stops admission, cancels the active workers, and waits until the workers stop. A timeout of the caller does not stop the `Close` operation. The program must close the queue before it closes the database.

Postgres transactions keep the database data correct. The memory store continues to use the mutex for the maps of the store. The session command loop does not replace the data interfaces or the permission checks.

The route `DELETE /sessions/{id}` removes a conversation and the child sessions. An active turn, messages in the queue, or a process with status `running` gives `409`. A parent session must stop before the operation removes a child session. New input to other conversations can continue.

A worker removes conversation messages, events, process records, and session permission decisions. The operation keeps usage records. The operation also keeps workspace permission decisions. Workspace files do not change.

The database keeps a tombstone with the session ID and parent session ID. Usage statistics use the IDs. A request cannot use a tombstone ID to put the conversation back into the store.

The queue and loop stop new work for the selected sessions before the database operation. The queue can complete the operation after the caller cancels the request. The database removes the session tree as one transaction. A storage error keeps the conversation. The queue can accept input again after a storage error.

### 4.2 Startup Prompt

The file `start_prompt.md` supplies the system instructions for the providers. The field `start_prompt_file` in the bootstrap file selects a different file. The command-line argument `--start-prompt-file` replaces the value from the bootstrap file. Relative paths use the directory from which you start the harness.

The harness reads the template one time when the program starts. After a template change, stop and start the harness. An empty file does not supply system instructions. A file error or an unknown variable causes a startup error.

The loop prepares the system message before the context and request stages. The message is before the history from the database for a model call. It is not written to the database. System messages from the database stay after it. The context limit includes the message.

The template accepts the variables:

- `{tool_list}`: the name and categories of the tools in the registry, in name sequence.
- `{NAME_info}`: the name, description, categories, and input schema of the tool `NAME` from the registry.
- `{workspace}`: the instance workspace directory.
- `{session_id}` and `{instance_id}`: the session and instance IDs.
- `{model}`: the session model, or the instance default when the session model is empty.
- `{agent_depth}`: 0 for the primary session, or the depth of the child agent.
- `{os}` and `{arch}`: the system and architecture of the harness program.

Tool data comes from the registry for a model call. For example, `{bash_info}` gives the full tool definition of `bash`. The `agent` definition includes the instance model list. Tool variables do not change the session tool group. The model uses tool discovery to add tools to the group.

Use 2 braces before and after the variable name for literal text. For example, `{{workspace}}` gives `{workspace}`. Substitution occurs one time. JSON braces do not change. If a tool in a variable is not in the registry, the turn gives an error before the model call.

See `docs/start-prompt.md` for variable rules and Docker configuration.

Requirements:

- R192: The harness must get default system instructions from the template file.
- R193: A model request must receive the template values for the session before the context stage.
- R194: The harness must not write the system message from the template to the database.
- R195: A tool definition from the template must come from the runtime registry and keep the full input schema.
- R196: Template substitution must not change the session tool group or run a command.
- R197: A tool variable for a tool which is not in the registry must give a turn error. Other sessions must continue.
- R198: The context limit must include the system message from the template.
- R199: Provider adapters must not add default system instructions.

## 5. Instances

A user starts an instance with a workspace directory. The harness gives an ID to the instance. The instance start request gives:

- The workspace directory.
- The model list and the model configuration.
- The process limit and the agent depth limit.

The model list gives the models for the sessions. A model list entry has an ID and a level. The instance uses the default model for a session which does not give a model.

The harness is one program. Many instances can run at the same time. The sessions, messages, tool calls, events, and processes of an instance stay in the instance. The harness must not mix the data of 2 instances.

An instance can stop and start again with the same data.

The database keeps a stopped instance and the sessions of the instance. The API can start the same instance again. A stopped instance must not start a model call or a process. File tools and shell commands must use the workspace of the instance.

Requirements:

- R12: The harness must give one API for many instances.
- R13: An instance must have a workspace directory and a model configuration.
- R14: The harness must not mix the data of 2 instances.
- R15: The harness must continue an instance with the same data after the instance stops and starts again.
- R16: The model configuration must give the model list.
- R17: The path guard must use the workspace directory of the instance.
- R18: The harness must read the instances from the database at the start.
- R19: The harness must give the statistics and the total cost of an instance.

### 5.1 Settings

The database keeps the settings of the harness. A setting has a name and a value. A setting is for the harness or for one instance. The harness uses the instance value before the harness value.

The settings are:

- `agent_depth_limit`: the maximum depth of an agent.
- `process_limit`: the maximum number of processes for an instance.
- `api_token`: the token for the API.

The initial start makes the `api_token` when the token is not in the bootstrap file. The harness shows the token one time. The user can change the token.

A token change applies to the next API request. The `api_token` setting is for the harness only. A new instance setting must replace a value from the instance start request.

The API gives the settings and changes the settings.

Requirements:

- R20: The database must keep the settings.
- R21: The harness must use the instance value before the harness value.
- R22: The agent depth limit must have the default 2.
- R23: The process limit must have the default 8.

## 6. Models

A provider connects the harness to one model API. One provider can give many models. Example: one API gives a small model, a large model, and a model with image input.

The Go interface of a provider is:

```go
type Provider interface {
    Name() string
    Models() []ModelInfo
    Stream(operationContext context.Context, request Request) (Stream, error)
}

type ModelInfo struct {
    ID         string
    Name       string `json:",omitempty"`
    Level      int
    Input      []MediaType
    Output     []MediaType
    Tools      bool
    ToolSupportUnknown bool `json:",omitempty"`
    ContextMax int
    Prices     *Prices
    Reasoning bool `json:",omitempty"`
    ReasoningEfforts []string `json:",omitempty"`
    DefaultReasoningEffort string `json:",omitempty"`
    ReasoningSummary string `json:",omitempty"`
    Billing string `json:",omitempty"`
}

type Content struct {
    Type MediaType
    Text string
    Data []byte
    MIME string
}

type MediaType string

const (
    Text  MediaType = "text"
    Image MediaType = "image"
    Audio MediaType = "audio"
    File  MediaType = "file"
)
```

The provider gives the model list for the instance. A model gives:

- The ID and the level.
- The input media types and the output media types.
- If the model can start tools.
- The context limit.

The field `Name` is optional. It gives a display label for the model. The field `ID` gives the value for model requests and instance model lists. A model version can change without a change to the model ID.

A message has content items. A content item has one media type and the data of the item. Thus, the harness can send text, an image, audio, or a file to a model.

A provider can use a standard API shape. The harness gives a standard adapter for a provider with the standard API shape. Thus, the harness can connect a new provider of the standard shape without new code.

The request gives the model ID, the messages, the tool schemas, and the parameters. A plugin can change the model or the provider at the `model.request` stage.

Requirements:

- R24: The harness must have one interface for the model providers.
- R25: A provider must give the model list.
- R26: A model must give the input media types and the output media types.
- R27: A content item must have one media type and the data of the item.
- R28: The harness must examine the input media types of the model before a model call.
- R29: The harness must not send a content item with a media type which is not an input type of the model.
- R30: The harness must use the standard adapter for a provider with the standard API shape.
- R31: A provider must send the response as a flow of data.
- R32: The harness must use the default model of the session when the request does not give a model.

### 6.1 Usage and Cost

The model ID can be `provider/model`. An ID without the provider is correct only when one provider has the ID. The instance model list and the agent tool use the full ID. The harness examines the model list after a plugin changes the model request.

The model can give the prices of the tokens. The prices are for 1,000,000 tokens. A model call gives the usage. The usage has:

- The input tokens.
- The cache read tokens.
- The cache write tokens.
- The output tokens.
- The reasoning tokens.
- The cost, when the model gives the prices.

The harness calculates the cost from the usage and the prices. The cost data is optional. Thus, the cost can be empty. The usage has the tokens.

A cost from the provider response replaces a cost estimate. A cost estimate has `Estimated: true`. A total with a cost estimate also has `Estimated: true`. A subscription provider does not get a token cost estimate.

A price tier applies when the full input tokens are above the price tier limit. The price tier applies to the full request. Output tokens include reasoning tokens. The field `Prices.Reasoning` can give a different price for reasoning tokens. The reasoning price replaces the output price for reasoning tokens.

A price of 0 and price data that is not available are different. If the response has cache tokens without price data, the cost is not available. Usage statistics include only costs with price data.

The cache hit rate is the cache read tokens divided by the full input tokens. The full input tokens are the input tokens, the cache read tokens, and the cache write tokens.

```go
type Prices struct {
    Currency   string
    Input      float64
    Output     float64
    CacheRead  float64
    CacheWrite float64
    CacheReadUnknown bool `json:",omitempty"`
    CacheWriteUnknown bool `json:",omitempty"`
    Reasoning *float64 `json:",omitempty"`
    Tiers []PriceTier `json:",omitempty"`
    Source string `json:",omitempty"`
}

type PriceTier struct {
    AboveInputTokens int
    Input float64
    Output float64
    CacheRead float64
    CacheWrite float64
    CacheReadUnknown bool `json:",omitempty"`
    CacheWriteUnknown bool `json:",omitempty"`
    Reasoning *float64 `json:",omitempty"`
}

type Cost struct {
    Currency string
    Value    float64
    Estimated bool `json:",omitempty"`
}

type Usage struct {
    Input      int
    CacheRead  int
    CacheWrite int
    Output     int
    Reasoning  int
    Cost       *Cost
}

type Statistics struct {
    Calls      int
    Input      int
    CacheRead  int
    CacheWrite int
    Output     int
    Reasoning  int
    Cost       *Cost
    Costs      []Cost
}
```

Requirements:

- R33: The prices must have a currency.
- R34: A provider must give the usage of a model call when the response has the usage.
- R35: The harness must record the usage of the model messages.
- R36: The harness must calculate the cost of a model call from the usage and the prices.
- R37: The harness must not calculate a cost when the model does not give the prices.
- R38: The cache hit rate must be the cache read tokens divided by the full input tokens.
- R39: The statistics must have the input tokens, the output tokens, the cache read tokens, the cache write tokens, the cache hit rate, and the cost.
- R40: The statistics for different currencies must stay apart.

The API gives `CacheHitRate` and `CacheHitPercentage`. The percentage is the rate multiplied by 100. The harness must use the prices from the provider configuration before prices from the database.

### 6.2 Provider Data

The default provider data is in `providers.json`. The file includes OpenAI and DeepSeek. The ChatGPT coding plan also uses data from the file. Source files must not contain a default provider list or model list.

The file has one entry for a provider. A missing or empty file does not add providers. The provider data has:

- The name of the provider.
- The API URL.
- The model list URL.
- The model list format: `openai` or `codex`. The default is `openai`.
- The price table URL. The field is optional.
- The interval for the model list.
- The prices of the models. The field is optional.
- The models. The field is optional.
- The default model data. The field is optional.
- The protocol: `responses` or `chat_completions`.
- The authentication method: `api_key`, `chatgpt`, or `none`.
- The metadata source: `metadata_url`, `metadata_format`, and `metadata_provider`. The fields are optional.
- The billing method: `tokens` or `subscription`. The default is `tokens`.

The harness keeps the provider data, the model lists, and the prices in the database. Thus, the instances read the model data from the database.

The secret of a provider is not in the file. The secret is in the database. The API sets a secret for the harness and a secret for one instance. The harness uses the secret of the instance before the secret of the harness.

A model list endpoint usually gives the model IDs and the limits. A model list endpoint can give the prices. Usually, the endpoint does not give the prices. Thus, the prices can come from:

- The model list response.
- The prices in the provider file.
- A price table in a file or at a URL.
- A model catalog from Models.dev.

The standard configuration uses the Models.dev JSON API for model data and token prices.

The provider endpoint controls model IDs. Models.dev must not add models to the provider list. The metadata source does not receive provider credentials. An error from the metadata source keeps the last correct model list.

The harness refreshes the model data at the interval. The default interval is 24 hours. The user can refresh the model data from the API. The harness keeps the last model list when the API gives an error.

A model list response can give only IDs. The harness uses model data in the sequence that follows:

- Data for a specified model in the JSON file.
- Data from the model list response.
- Data from the metadata source.
- Default data from `model_defaults` in the JSON file.
- Data from the model cache.

When a model list endpoint is available, the harness uses only the IDs in a correct response. An empty list removes available models. A request error or an incorrect response keeps the last correct list. Model data in the JSON file must not add IDs to the response list.

The field `ToolSupportUnknown` is `true` when the response does not give data about tools. Then the harness sends the tool definitions. The provider can reject the request. A value of `false` for `tools` removes tool definitions. The database keeps `ToolSupportUnknown` with `Tools`.

For the `codex` format, the adapter reads the model list URL with account authentication. The response has a `models` array with `slug`, `display_name`, `context_window`, and `input_modalities` fields. The configuration supplies the URL and client version. A source code change is not necessary to change the URL or version.

The Codex response also supplies `supported_reasoning_levels`, `default_reasoning_level`, and `supports_reasoning_summaries`. The adapter uses the data for reasoning effort selection. Token prices from Models.dev do not apply to subscription access.

A provider can use the `Refresher` interface:

```go
type Refresher interface {
    Refresh(operationContext context.Context) ([]atom.ModelInfo, error)
}
```

The standard adapter uses `Refresher` when the provider data has a model list URL. Thus, the harness finds a new model from the API. A plugin can also refresh the model data.

Requirements:

- R41: The provider file must give the default provider data and model data.
- R42: The provider data must give a name and an API URL. A model list URL is optional when the provider uses a model list from configuration.
- R43: The provider file can give the prices and the models.
- R44: The harness must keep the provider data, the model lists, and the prices in the database.
- R45: The harness must keep the secret of a provider in the database.
- R46: An instance can have a secret for a provider.
- R47: The harness must use the secret of the instance before the secret of the harness.
- R48: The API must let the user set the secret of a provider.
- R49: The harness must refresh the model data at the interval.
- R50: The default interval must be 24 hours.
- R51: The harness must keep the last model list when the API gives an error.
- R52: An instance must select the model list from the models of the providers.
- R53: The `Refresher` interface must give the model list and the prices from the API.
- R54: The standard adapter must implement `Refresher` when the provider data has a model list URL.

### 6.3 Context Limit

The context limit is `ModelInfo.ContextMax`. The harness keeps the system messages and the last turn. When the context is too large, the harness removes the initial turn from the request. A turn includes the tool calls and the tool results.

User input or a runtime notification can start a turn. The database keeps the full message history.

If the last turn is too large, the harness gives an error. The harness does not send the request. A provider can implement `TokenCounter` for the token count. The default token estimate uses text bytes and a media allowance. The estimate is not the usage. The provider response gives the usage.

```go
type TokenCounter interface {
    CountTokens(operationContext context.Context, request atom.Request) (int, error)
}
```

### 6.4 Provider Connections

The default provider IDs are `openai`, `deepseek`, and `openai-codex`. OpenAI and DeepSeek use API keys. The provider `openai-codex` uses a ChatGPT subscription. The test provider can run with the default providers. The default providers are available in test mode.

The provider file can override the default data. A file entry is not necessary for a default provider. The user sets an API key with the API or the UI. New requests use the new API key. The harness can continue to run during a key change.

The provider `openai-codex` uses OAuth device authentication. The API gives a URL, a user code, and a login ID. The user opens the OpenAI URL and supplies the code. The harness waits for OpenAI authorization. The user must supply the code in 15 minutes.

The database keeps OAuth credentials. The harness can refresh an OAuth credential before the expiry time. Credential requests run one at a time. If the user cancels or replaces a login, the harness must discard subsequent credential responses for the login. The harness stops the login worker before the database closes.

Provider responses do not contain API keys or OAuth tokens. The provider data includes `Connected` and `ModelCount`. The field `Connected` shows that a credential is in the database. It does not show model access. The provider examines model access during a model call.

The API gives the providers in the provider registry. A database entry without a provider in the registry is not in the response. The response includes the test provider when it is active.

The default providers use the Responses API. The adapter sends a tool call when the input JSON is available. It does not wait until the model stream stops. The database keeps the field `ProviderState` for the next request to the same provider. The message response does not contain the field `ProviderState`. The context estimate includes the bytes in the field `ProviderState`.

Requirements:

- R187: The default provider data must come from `providers.json`.
- R188: The user can set a provider key while the container runs.
- R189: The provider `openai-codex` must use account authentication, not an API key.
- R190: The API must not give provider authentication tokens to a client.
- R191: A tool result request must keep the reasoning data necessary for the provider.

### 6.5 Reasoning

A model supplies the available reasoning efforts in `ReasoningEfforts`. The model name does not supply the values. The field `Session.ReasoningEffort` gives the session selection. An empty string uses the model default. An incorrect selection gives an error before the model request.

A change to the session selection applies at the next model request. The rule also applies after a tool result group. The change does not stop an active model request. A child agent can select `reasoning_effort`. Without a selection, a child agent uses the model default.

The Responses adapter sends `reasoning.effort`. The Chat Completions adapter sends `reasoning_effort`. The field `reasoning_summary` in model configuration selects a reasoning summary format. The value `auto` selects a reasoning summary format from the provider.

A provider can supply reasoning text or a reasoning summary. The database keeps the text in `Message.Reasoning`.

The API sends `Message.Reasoning` and reasoning text in `model.chunk` events. The API does not send `ProviderState`. The adapter keeps continuation data for the same provider.

See `docs/model-catalog.md` for configuration and source documents.

### 6.6 Model Selection

The session model can change during a conversation. The next model request uses the new selection. The change does not stop an active request. The new model must be in the instance model list.

The API accepts a model with the same or a larger context limit. For a smaller context limit, the user must accept context compaction. If the previous context limit is not available, the user must accept context compaction. The new model must have a context limit.

The harness keeps the system messages and the last turn. When the context is too large, the harness removes the initial turn from the request. A turn includes the tool calls and the tool results. The database keeps the full message history. If the last turn is too large, the harness gives an error.

The database changes the model and reasoning effort together. If the new model does not have the previous reasoning effort, the session uses the model default. An operation with a previous selection must read the session again before a change.

## 7. Stages

The stage values are:

- `context.build`: the messages for the model.
- `model.request`: the request object.
- `model.response`: the model message.
- `action.plan`: the tool plan.
- `tool.input`: one tool call.
- `tool.result`: one tool result.
- `message.save`: one message before the database operation.
- `process.output`: one process event.

A plugin can do 4 operations on a stage:

- Operation 1: the plugin reads the value. The flow does not change.
- Operation 2: the plugin gives a new value.
- Operation 3: the plugin gives the verdict `allow`, `ask`, or `deny`.
- Operation 4: the plugin attaches a tool, a watcher, or a provider.

A plugin can select a verdict on the `tool.input` and `model.request` stages only.

## 8. Events

An event gives information to plugins. An event does not change the flow. The events are:

- `session.start`, `session.end`
- `turn.start`, `turn.end`
- `model.call`, `model.chunk`
- `action.received`
- `tool.start`, `tool.end`
- `permission.request`, `permission.decision`
- `process.start`, `process.output`, `process.exit`, `process.notification`
- `agent.start`, `agent.end`

Requirements:

- R55: The harness must record an event in the event record.
- R56: The harness must send an event to the attached plugins.

## 9. Plugins

A plugin is a Go package. A plugin gives a name and a `Setup` function. The `Setup` function attaches handlers to the harness object.

```go
type Plugin interface {
    Name() string
    Version() string
    Setup(harnessRuntime *Harness) error
}
```

The harness object gives 6 operations:

```go
func (harnessRuntime *Harness) On(event EventName, handler Handler) Unsubscribe
func (harnessRuntime *Harness) Tool(tool Tool) Unsubscribe
func (harnessRuntime *Harness) Provider(provider Provider) Unsubscribe
func (harnessRuntime *Harness) Watch(watcher ProcessWatcher) Unsubscribe

func Pipe[Value any](harnessRuntime *Harness, stage Stage[Value], middleware Middleware[Value]) Unsubscribe
func Decide[Value any](harnessRuntime *Harness, stage Stage[Value], decision Decision[Value]) Unsubscribe
```

The Go compiler does not let a method have a type parameter. Thus, `Pipe` and `Decide` are functions.

Requirements:

- R57: A plugin must not use the internal packages of the harness.
The harness can also get tools from an MCP server. The harness reads the servers from the MCP file. A server can have a program or a URL. The harness starts a server with a program. The harness connects to a server with a URL. Then the harness reads the tools of the server and gives the tools to the model.

The name of an MCP tool is `mcp__{server}__{tool}`. Thus, 2 servers can have a tool with the same name.

Requirements:

- R58: One registry must keep the tools of the harness and the plugins.
- R59: The harness must read the MCP servers from the MCP file.
- R60: The harness must start an MCP server that has a program.
- R61: The harness must connect to an MCP server that has a URL.
- R62: The harness must use the MCP protocol for the tools of a server.
- R63: The name of an MCP tool must be `mcp__{server}__{tool}`.
- R64: The harness must refresh the tools when an MCP server gives a change message.
- R65: The harness must run handlers in the sequence that the plugin attaches them.
- R66: The harness must stop the pipeline when a handler gives an error.
- R67: A plugin must attach a tool, a watcher, or a stage handler before the loop starts.

- R68: The event bus must send an event to the handlers of the harness and the plugins.
- R69: The model gateway must use the providers from the plugins.
- R70: The harness must keep the other handlers when it removes a handler.

## 10. Tools

The Go interface of a tool is:

```go
type Tool interface {
    Name() string
    Description() string
    Categories() []string
    InputSchema() Schema
    Check(operationContext context.Context, call ToolCall) Verdict
    Run(operationContext context.Context, call ToolCall) (ToolResult, error)
}
```

A tool call has 6 steps:

1. Read the input schema of the tool.
2. Examine the input against the schema.
3. Get the verdict from the `Check` function.
4. Get the verdicts from the policy plugins.
5. Select the last verdict.
6. Start the `Run` function when the last verdict lets the tool start.

The response from the model is a flow. A tool call is one item in the flow. The harness starts a tool call when the harness reads the full input of the tool call. The harness does not wait for the other data of the model message. Thus, the harness runs the tools of one message at the same time.

The verdict rules are:

- One `deny` verdict stops the tool call.
- One `ask` verdict sends a permission request to the user.
- The tool runs only when the verdicts are `allow`.

### 10.1 Example of the Path Guard

The model sends a `bash` tool call with the command `rm -rf /home/user/data`. The path is not in the workspace directory. The path guard gives the verdict `ask`.

The harness sends a permission request to the API client. The user gives the response `deny`. The tool does not start. The tool result has the status `denied`.

Requirements:

- R71: The path guard must resolve a symbolic link before the check.
- R72: The path guard must give the verdict `ask` when the path is not in the workspace directory.
- R73: A permission decision must have a scope. The scope is one time, the session, or always.
- R74: The harness must record the permission requests and the decisions.
- R75: The harness must wait for the permission decision.
- R76: The permission request must not have a timeout.
- R77: A tool must not start a process without a check.
- R78: The harness must keep the sequence of the tool results.
- R79: The harness must examine the input of the tool call against the input schema of the tool before the `Check` function.

The schema check uses JSON Schema. An incorrect schema must give an error. A schema reference must be in the schema document. The check must not read an external file or URL for a schema reference.

A verdict of `deny` must stop a tool before the harness reads a permission decision from the cache. The scope `once` is for one permission request. The scope `session` is for one session in one instance. The scope `always` is for one instance and continues after a program start.

### 10.2 Find Tools

The initial model request has the full definitions of `search_tool` and `file_actions`. The model starts `search_tool` with a query to get other tools. The query has a category or text. The result gives only the fields `Name` and `Categories` of a tool.

The harness makes a search document from the tool name, description, categories, and input schema. A tool can also give usage examples through the optional `SearchDocument` method. For semantic search, a model changes the document to a vector. The query uses the same model. Lexical search uses TF-IDF vectors from the same documents.

The function compares the query vector with the document vectors. It uses cosine similarity. The default minimum is 0.3 for semantic search and 0.01 for lexical search. Configuration can change the minimum values to a value from 0 to 1. Lexical search gives capability descriptions a weight of 0.75 and full search documents a weight of 0.25.

The search documents and vectors stay in the harness. They are not in the tool result or the model request. The result does not contain tool descriptions or input schemas. The harness gets the descriptions and schemas from the registry for the next model request.

A full tool name selects the tool directly. A category without query text gives the tools in the category. A request for vectors is not necessary for a full name or category. The function must ignore the difference between `A` and `a`. It does not use a text-generation model for tool discovery.

The input fields of `search_tool` are:

- `query`: the text which the function compares.
- `category`: the category of the tools. The field is optional.
- `limit`: the maximum number of tools in the result. The default is 10. The maximum is 50.

The result puts the largest cosine similarity first. Tools with the same cosine similarity are in name sequence. The category field must agree with the tool category. The result for a category without text is in tool name sequence. The result can be empty.

The harness keeps document vectors in memory. A change to the tool document causes new document vectors. A tool removal also removes the vectors from the cache. Lexical search calculates new TF-IDF weights after a tool change. MCP tool descriptions and schemas come from the server.

The `tool_search` section of `providers.json` gives the mode, model directory, and the minimum cosine similarity values. The mode can be `auto`, `semantic`, or `lexical`. The default mode is `auto`.

The mode `auto` uses semantic search when available. An error during model startup or vector calculation causes automatic fallback to lexical search. The backend stays in lexical search until the harness starts again. The harness records the selected backend and error cause.

The Docker image includes the MiniLM model and tokenizer from the model source. Hugot runs the model in the harness with Go. A model server and native library are not necessary. The image build gets model assets from a specified source revision. It compares SHA-256 values. The harness does not get model assets at runtime.

MiniLM accepts a maximum of 256 WordPiece tokens for an input. The tokenizer divides long documents into sections. It must keep the full document. The harness keeps a vector for a section. The search score is the largest cosine similarity of the tool's sections.

The `semantic` build tag includes the optional model adapter. Without the `semantic` build tag, a Go build has only lexical search. The Docker target `core` also uses lexical search. The registry uses the `Searcher` interface. It does not use Hugot. The model adapter can be removed independently of the lexical backend.

An error during vector calculation in `semantic` mode gives a tool error. It must not give an empty result with status `ok`. Cancellation must not cause automatic fallback. Cancellation stops subsequent chunks and discards the result in progress. The chunk in progress completes before model teardown. The harness must wait for tool discovery before it releases model resources.

Conversation vector recall is not necessary for tool discovery. Conversation vector recall is for a subsequent milestone.

The harness adds the found tools to the tool group of the session. Thus, the model can start the found tools on the next turn. The model can start `search_tool` again for other tools.

The harness gives the tool group and the `search_tool` tool in the model request. A plugin can change the tool group at the `model.request` stage.

Requirements:

- R80: A tool must have one or more categories.
- R81: Text queries must compare the query vector with the tool document vectors.
- R82: For semantic search, a model must make vectors. Lexical search must use TF-IDF. Tool discovery must not use a text-generation model.
- R83: A search document must include the tool description, usage examples when available, and input schema.
- R84: Text queries must apply the minimum cosine similarity from configuration.
- R85: The `search_tool` function must ignore the difference between `A` and `a`.
- R86: The `category` field must agree with the category of a tool.
- R87: The result must not have a number of tools above the `limit` field.
- R88: The default limit must be 10 and the maximum limit must be 50.
- R89: Text queries must put the largest cosine similarity first.
- R90: Tools with the same cosine similarity value must be in name sequence.
- R91: Tool discovery must give the same sequence for the same vectors and tool group.
- R92: The harness must give the `search_tool` and `file_actions` tools to a model that can start tools.
- R93: The result of `search_tool` must give only `Name` and `Categories` for the found tools.
- R94: The harness must add the found tools to the tool group of the session.
- R95: The harness must give the tool group and the `search_tool` tool in the model request.
- R96: The harness must not give a tool to the model when the tool is not in the tool group.

### 10.3 Tool Descriptions

The tool description and input schema give the model information about the tool. The input schema must give the purpose of a parameter. A parameter for a duration must have a unit. An optional parameter must have a description of the default behavior. The description must give the meaning of special input data, such as `0` or an empty string.

The tool description must tell the model about the behavior of the tool. For example, the `write` action replaces the full file content when the input does not give a line range. The process tools use a harness process ID, not a PID from the operating system. The `agent` tool uses the instance default model when the input does not give a model ID.

The model request must keep the parameter descriptions and defaults. A new model list must not replace the parameter descriptions of the `agent` tool. An MCP server gives the descriptions and schemas for the tools of the server. The harness must not add an incorrect unit to an external parameter.

### 10.4 File Tools

The tool `file_actions` replaces the tools `read`, `write`, and `replace`. One tool call has one `path` and an `actions` array with 1 to 32 items. The field `op` can be `read`, `write`, `replace`, `append`, `prepend`, or `list`. A relative path starts at the instance workspace. The initial model request has the full schema.

Actions run in input sequence on the same path. An action uses the content and line numbers from the previous action. File edits use temporary content. The actions and selected preview must be correct before the tool replaces the file. The tool uses one rename operation. An error prevents the file change.

The actions `read`, `write`, and `replace` have optional `start_line` and `end_line` parameters. Line numbers start at 1. The line range includes the start line and the end line. Without `start_line`, the range starts at line 1. Without `end_line`, the range continues to EOF.

The end of a line is LF or EOF. An empty file has 0 lines. The last LF does not make a new line. A start line must be in the file when the input gives a range. The `read` action stops at EOF when `end_line` is after the last line. The `write` and `replace` actions give an error when a line from the input is not in the file.

The preview limit for one tool call is 200 lines or 16 KiB of file text. The byte limit includes line-number labels. Status and truncation notices do not use the preview limit. If the selected text is too large, a truncation notice gives `start_line` and `start_byte` for the next read action. The truncation notice is not file content.

A `start_byte` value gives the position in bytes from the start of `start_line`. The default value is 0. The value must select a byte in the line. It must not divide a UTF-8 character. Use `start_byte` from the truncation notice to continue a long line. Other lines start at position 0.

The `write` action replaces the full lines in the range. The other file data does not change. An empty replacement removes the selected lines. Without a range, `write` replaces the full file content or makes a new file. The parent directory must be available.

The last line break of the selected text can be LF or CRLF. The tool keeps the last line break when the replacement has text but does not have a last LF. Internal line breaks in the replacement do not change. Without a range, the tool writes the replacement text without a change.

The `replace` action finds the text from `old_text` and gives the text from `new_text`. The input `old_text` must not be empty. An empty `new_text` removes the text match. The text must agree in letter case, space characters, and line breaks. The text match must stay in the selected range. The tool gives an error without a file change when it cannot find the text.

The `replace` action has 3 mode values:

- `first`: replace the initial text match in the range. This is the default mode.
- `last`: replace the last text match in the range.
- `all`: replace the text matches in sequence from the start of the range. A text match must not use text from a previous text match.

The tool does not examine the new text again. It does not use a regular expression. A file edit uses a temporary file and a rename operation. The file edit keeps the permission bits of the file. A file edit replaces the file at the path. Other hard links keep the previous content.

The harness puts tool calls for the same resolved path in sequence. Different paths can use different tool calls. The sequence does not include external editors.

The `append` action adds `content` at the end of the file. The `prepend` action adds `content` at the start. They do not add line breaks automatically. The file must be available. A previous `write` action without a line range can make the file in the same tool call.

The `list` action gives directory entries in name sequence. A name which starts with `.` is not removed. The tool does not follow symbolic links. The result gives the name, type, and number of bytes. The default `limit` is 100 directory entries, and the maximum is 200.

A directory cursor gives the position for the next directory page. A `list` action cannot be in a tool call with file actions.

A file edit result gives the resolved path, file status, and number of lines and bytes. The status text is `Created`, `Updated`, or `Unchanged`. The result of `replace` also gives the number of text matches and the mode.

The model selects the result. A unified diff is not the default. In a unified diff, the prefix `-` shows lines which the tool removes. New lines have the prefix `+`. The preview keeps LF and CRLF and shows a last line without LF. The line numbers refer to the full file.

The limits apply only to the preview. The file edit writes the full replacement.

A tool call with file edits must have a `return` object. The field `return.type` is necessary in the object. Use `return.type` to select the result:

- `summary`: file status, action information, and the number of lines and bytes.
- `diff`: a unified diff of previous and new content. The default value for `context_lines` is 3. The range is 0 to 100.
- `read`: new file text, with optional `start_line`, `end_line`, and `start_byte`.
- `list`: directory entries, with optional `limit` and `cursor`.

A `read` or `list` action also gives output. The model must not select the same output again in `return`. The limit applies to action output and output from `return` together. Without a file edit, the `return` field is optional.

The line range in `return` uses the new file content. It does not select the previous lines to replace. The default start is line 1. The default end is EOF.

The preview uses EOF when `return.end_line` is after the last line. The output limit is 200 lines or 16 KiB.

An incorrect `return` object or start line prevents the file edit. The tool examines the preview before the rename operation. A preview must use the content from the same file edit. The tool does not read the file again to make the preview.

The tool removes the same outer text from the diff input but keeps lines of context. The diff input contains previous file content and replacement file content. The maximum total is 256 KiB or 4000 lines. If the diff input is above the limit, the result gives the cause without the preview. Incorrect UTF-8 or NUL bytes also prevent a preview.

The tool makes the unified diff from the previous content and replacement content of the same file edit. It does not read the file again after the file edit. Thus, the result does not show a different file edit.

Use `on_error.return` to select an error diagnostic for the same path. The type of error diagnostic must be `read` or `list`. It cannot change file content or run the actions again.

After a file edit error, the preview uses previous file content. Temporary content and previews are discarded. The tool status stays `error`, with the action number and cause.

An error diagnostic cannot replace the initial error. Cancellation and incorrect input do not start error diagnostic I/O. The tool call does not change the file. Use the error diagnostic to correct the next action. Do not use the same incorrect input again.

The tool input can be:

```json
{"path":"src","actions":[{"op":"list","limit":100}]}
{"path":"src/main.go","actions":[{"op":"read","start_line":10,"end_line":40}]}
{"path":"notes.txt","actions":[{"op":"write","content":"hello\n"},{"op":"append","content":"world\n"}],"return":{"type":"read"}}
{"path":"src/main.go","actions":[{"op":"replace","old_text":"oldName","new_text":"newName","mode":"all"}],"return":{"type":"diff","context_lines":3},"on_error":{"return":{"type":"read","start_line":10,"end_line":50}}}
```

### 10.5 Read Output A/B Test

The process environment variable `MTT_READ_LINE_NUMBERS` selects the file text format of `file_actions`. The harness reads the variable when it attaches the tool. The control value is `false` or `0`. If the variable does not have a value, the harness selects the control. The test value is `true` or `1`. An incorrect value prevents a program start.

The control output gives file text without line-number labels. The test output has the file line number before the text. For example, a range from line 10 starts with `10: `. The line prefix is output data, not file content. A model must not put the line prefix into a file edit. The tool description gives the active output format to the model.

The A/B test does not change the line range in the tool input or the file content. The byte limit includes the line-number labels. The output can stop at a different byte because of the labels. The tool gives a truncation notice if the selected text is too large. The format applies to actions, `return`, and error diagnostics.

The model, task, and files must be the same for the control and the test. A new process start is necessary to change the output format.

Docker Compose reads the local `.env` file and gives the variable to the harness container. The `.env.example` file gives an example.

The repository must not contain the local `.env` file. The Go program reads the process environment only. The harness settings and provider secrets stay in the database.

Set the value in `.env`:

```dotenv
# Numbered output; use false for the plain-text control.
MTT_READ_LINE_NUMBERS=true
```

Start the harness:

```sh
docker compose up --build
```

## 11. Processes

The harness can start a process and monitor it.

```go
type Process interface {
    ID() string
    PID() int
    Write(data []byte) error
    Kill(signal Signal) error
    Wait() (ExitStatus, error)
    Events() <-chan ProcessEvent
}

type ProcessWatcher interface {
    Match(event ProcessEvent) bool
    OnMatch(operationContext context.Context, event ProcessEvent)
}
```

### 11.1 Notification Policy

The tool call gives the notification policy for a background process. The policy has the mode and the interval. The mode is one of 4 values:

- `none`: the harness does not send a notification.
- `exit`: the harness sends one notification when the process stops.
- `error`: the harness sends one notification when the process stops with an error status.
- `interval`: the harness sends one notification at the interval.

The interval is a number of milliseconds. The default mode is `exit`. A notification is a message in the session. Thus, the model reads the notification on the next turn.

The default value of `wait` for `bash` is `true`. The output goes into the tool result group. The process uses mode `none` before it starts. It does not send a notification for the output. The notification parameters apply only when `wait` is `false`.

A runtime notification has the message role `runtime`. It is process data, not a user request or a system instruction. The queue keeps the message role. The UI shows the message as `Runtime`.

The provider request uses the role `user` for the runtime notification. The notification is context data. It is not a system instruction or a tool result. The content gives the message source.

The unit for the `timeout` and `interval` input of `bash` is milliseconds. For example, `timeout: 60000` gives a limit of 60 seconds.

The default timeout is `0`. With `timeout: 0`, the harness does not stop the process automatically. The timeout applies when `wait` is `true` or `false`.

The default value of `wait` is `true`. A value of `false` gives the process ID immediately. When the user cancels a tool call, the process continues. Use `process_kill` to stop the process.

The `interval` parameter must be a minimum of 1 millisecond when `wait` is `false` and `notify` is `interval`. The values `none`, `exit`, and `error` do not use the interval. The mode `interval` also sends a notification when the process stops.

The process manager keeps the process after the initial turn stops. The process manager writes the exit status with an active database context. Output goes through `process.output` before the output buffer, watchers, and notifications.

The output buffer has a limit of 256 KiB for standard output and 256 KiB for standard error. The process supervisor keeps the last 128 completed processes. A client can read the output after a process stops. On Unix, a stop signal applies to the process group.

Requirements:

- R97: The harness must record a process and the status of a process in the database.
- R98: The harness must send an output event to the watchers.
- R99: A process must continue after the turn which starts it.
- R100: A tool can read the output of a process.
- R101: The harness must have a limit for the output buffer.
- R102: The harness must stop a process after the timeout.
- R103: A tool call must give the notification policy of the process.
- R104: The notification policy must have the mode `none`, `exit`, `error`, or `interval`.
- R105: On the mode `exit`, the harness must send a notification when the process stops.
- R106: On the mode `error`, the harness must send a notification when the process stops with an error status.
- R107: On the mode `interval`, the harness must send a notification at the interval.
- R108: The default notification mode for a background process must be `exit`. A tool which waits for the process must use `none`.
- R109: The harness must send a process event through the `process.output` stage before the watchers.

## 12. Agents

An agent is a loop which operates below the parent session in the same instance. The parent session starts an agent with the `agent` tool. The agent has:

- A task from the parent session.
- A new session.
- A model from the model list.
- The `finish` tool.

The depth of an agent is the number of agents above the agent. The parent session is at depth 0. A child agent of the parent session is at depth 1.

The agent operates in the workspace directory of the instance. The agent can use the tools of the instance. When the agent starts the `finish` tool, the agent loop stops. The input of the `finish` tool goes to the parent session as the result of the `agent` tool.

The parent session selects the model for the child agent. Thus, the parent session can start a child agent with a different model for a small task. A model list entry can have a level. Thus, the parent session can select a model with a level below the default level for a small task.

The instance has a limit for the agent depth. The harness starts a child agent only when the depth is below the limit. At the limit, the harness does not give the `agent` tool to the model.

```
parent session (depth 0)
    |
    +-- child agent (depth 1)
            |
            +-- child agent (depth 2)
```

Requirements:

- R110: A parent session can start a child agent.
- R111: A child agent must have a task from the parent session.
- R112: A child agent must have the `finish` tool.
- R113: The `finish` tool must stop the agent loop and give the result to the parent session.
- R114: An instance must have a limit for the agent depth.
- R115: The harness must not give the `agent` tool to the model when the depth is at the limit.
- R116: The harness must record the relation between a parent session and a child agent.
- R117: A child agent must operate in the workspace directory of the instance.
- R118: The `agent` tool can give the model for the child agent.
- R119: The child agent must use a model from the model list.
- R120: The child agent must use the default model when the `agent` tool does not give a model.
- R121: The harness must keep the usage of the child agents.
- R122: The result of the `agent` tool must give the usage of the child agent.
- R123: The statistics of a parent session must include the usage of the child agents.

## 13. Database

Postgres is the default database. The tables are:

- `instances`
- `sessions`
- `messages`
- `tool_calls`
- `tool_results`
- `events`
- `processes`
- `permissions`
- `usage_records`
- `providers`
- `models`
- `provider_keys`
- `provider_oauth`
- `settings`

The `messages` table keeps the content, the tool call data, and the tool results of a message.

Requirements:

- R124: The database must have a table for the instances.
- R125: The harness must write the messages, tool calls, tool results, and events to Postgres.
- R126: The harness must use an interface for the database.
- R127: The database must write one message and the tool calls of the message in one transaction.
- R128: The row of the data must have the instance ID.
- R129: The harness must write the usage of a model call to the `usage` table.
- R130: The harness must write the usage of a model call to the `usage_records` table.
- R131: The row of the usage must have the instance ID, the session ID, and the model ID.
- R132: The harness can use the memory store when the database is not available.
- R133: The harness must keep the settings and the provider secrets in the database.

Vector data is for version 2. The tables must have space for a vector column.

## 14. API

The harness does not have a user interface. The API has 2 parts:

- HTTP for commands.
- WebSocket for the event stream.

The initial API paths are:

- `POST /instances`: start an instance.
- `GET /instances`: read the instances.
- `GET /instances/{id}`: read an instance.
- `DELETE /instances/{id}`: stop an instance.
- `POST /instances/{id}/start`: start a stopped instance.
- `POST /instances/{id}/sessions`: start a session.
- `GET /instances/{id}/sessions`: read the sessions of an instance.
- `GET /instances/{id}/models`: read the model list of an instance.
- `GET /sessions/{id}`: read a session.
- `DELETE /sessions/{id}`: remove a conversation and the child sessions.
- `PUT /sessions/{id}/reasoning`: set the reasoning effort for a session.
- `PUT /sessions/{id}/model`: change the model of a session.
- `GET /sessions/{id}/messages`: read the messages of a session.
- `POST /sessions/{id}/cancel`: stop the current run.
- `DELETE /sessions/{id}/queue/{message_id}`: remove a message from the queue.
- `GET /sessions/{id}/status`: read the status of the current run and the messages in the queue.
- `POST /sessions/{id}/revert`: remove the messages after a message.
- `POST /sessions/{id}/messages`: send a user message.
- `GET /sessions/{id}/events`: send events through WebSocket.
- `GET /sessions/{id}/agents`: read the child agents of a session.
- `GET /sessions/{id}/processes`: read the processes of a session.
- `POST /permissions/{id}`: give the response to a permission request.
- `GET /processes/{id}/output`: send process output through WebSocket.
- `GET /providers`: read the providers.
- `GET /providers/{id}/models`: read the models of a provider.
- `POST /providers/{id}/refresh`: refresh the model data.
- `PUT /providers/{id}/key`: set the secret of a provider.
- `DELETE /providers/{id}/key`: remove the secret of a provider.
- `POST /providers/{id}/auth/device`: start device authentication.
- `GET /providers/{id}/auth/device/{login_id}`: get login status.
- `DELETE /providers/{id}/auth/device/{login_id}`: cancel a login.
- `DELETE /providers/{id}/auth`: remove the account credential.
- `GET /settings`: read the harness settings.
- `PUT /settings/{key}`: change a harness setting.
- `DELETE /settings/{key}`: remove a harness setting.
- `GET /instances/{id}/settings`: read the settings of an instance.
- `PUT /instances/{id}/settings/{key}`: change a setting of an instance.
- `DELETE /instances/{id}/settings/{key}`: remove a setting of an instance.
- `PUT /instances/{id}/providers/{provider}/key`: set the secret of an instance.
- `DELETE /instances/{id}/providers/{provider}/key`: remove the secret of an instance.
- `GET /sessions/{id}/statistics`: read the statistics of a session.
- `GET /instances/{id}/statistics`: read the statistics of an instance.
- `GET /statistics`: read the full statistics of the harness.
- `GET /health`: give the status of the harness.

The message input can have text or content items:

```json
{"content":"hello"}
{"content":[{"type":"image","mime":"image/png","data":"BASE64"}]}
```

The event path accepts `?since=N`. The database gives the sequence number. The API sends the events after the sequence number, then the new events. A slow client must not stop the loop.

When the user reverts a session, the API stops the current run. The API must wait until the current run stops. Then the API removes the messages after the given message. The API must not accept a new message while it removes messages. The usage record keeps the cost.

Requirements:

- R134: The API must send a permission request to the attached clients.
- R135: The API must use the initial response to a permission request.
- R136: The API must use a token.
- R137: The API must send an event with a sequence number.
- R138: The API must start an instance from a workspace directory.
- R139: The API must give the sessions of an instance.
- R140: The API must put a user message in the queue and must not wait for the current run.
- R141: The API must stop the current run of a session.
- R142: The API must remove a message from the queue.
- R143: The API must give the status of the current run and the messages in the queue.
- R144: The API must remove the messages after a message.
- R145: When the user reverts a session, the harness must stop the current run and empty the message queue of the session.
- R146: The API must give the model list of an instance.
- R147: The API must give the settings and change the settings.
- R148: The API must give the statistics of a session.
- R149: The API must give the statistics of an instance.
- R150: The API must give the full statistics of the harness.
- R151: The API must give the providers and the provider models.
- R152: The API must let the user refresh the model data of a provider.

## 15. Atoms, Molecules, Organisms

The code has 3 layers.

**Atoms** are values and small units. The atoms are:

- `Message`, `Content`, `ToolCall`, `ToolResult`, `ToolGroup`, `Usage`, `UsageRecord`, `Cost`, `Prices`, `Statistics`, `Verdict`
- `PermissionRequest`, `PermissionDecision`, `ProcessSpec`, `NotifyPolicy`, `ProviderSpec`, `ProcessEvent`
- `InstanceSpec`, `ModelSpec`, `ModelInfo`, `AgentTask`, `Event`, `EventName`, `Stage`, `Schema`
- `SessionID`, `Session`, `ProcessRecord`

**Molecules** are small units with one task. The molecules are:

- `Pipeline`
- `ToolExecutor`
- `ContextBuilder`
- `ProviderAdapter`
- `SessionStore`
- `InstanceStore`
- `ProcessSupervisor`
- `PermissionEngine`
- `EventBus`

**Organisms** are full subsystems. The organisms are:

- `AgentLoop`
- `ToolRegistry`
- `PluginHost`
- `InstanceManager`
- `ProcessManager`
- `SessionMemory`
- `ModelGateway`

The layer rules are:

- An atom must not use a molecule or an organism.
- A molecule must not use an organism.
- An organism can use molecules and atoms.
- A plugin sees only atoms and the harness object.

## 16. Files and Packages

### 16.1 Files

The files of the project are:

```
mtt-harness/
  go.mod
  .env.example                 # local benchmark configuration template
  mtt.example.json             # the example bootstrap file
  providers.json               # the provider data
  mcp.example.json             # the example MCP server file
  start_prompt.md              # the shared system-prompt template
  cmd/mtt/
    main.go                     # the entry point
    application.go              # runtime wiring and API lifecycle
    options.go                  # command-line arguments
    startup_errors.go           # required startup operations
    authentication.go           # API token setup
    storage.go                  # stores, instance restore, process limit
    providers.go                # provider loading and refresh
    mcp.go                      # MCP loading and tool registration
    tools.go                    # built-in tool registration
    experiments.go              # process-level benchmark switches
    tool_search.go              # search configuration and lifecycle
    tool_search_{core,semantic}.go # optional backend factory
    plugins.go                  # plugin registration
  atom/                        # atoms: values only
    message.go
    content.go
    tool.go
    model.go
    process.go
    event.go
    instance.go
    verdict.go
    stage.go
  harness/                     # the plugin interface
    harness.go                 # the Harness object
    events.go                  # event subscriptions
    pipeline.go                # middleware and decisions
    context.go                 # the session in the context
    workspace.go               # shared workspace path resolution
    plugin.go                  # the Plugin interface
    tool.go                    # the Tool interface
    provider.go                # the Provider interface
    process.go                 # the Process interface
  internal/
    config/config.go           # the bootstrap file
    operation/errors.go        # contextual runtime errors
    testutil/assertions.go     # shared test assertions
    molecule/
      schema/schema.go         # the input schema check
      pipeline/pipeline.go     # the middleware chain
      contextbuilder/context.go
      startprompt/template.go  # template loading and variable syntax
      startprompt/variables.go # session and tool text substitution
      embedding/embedding.go   # the encoder and chunk interfaces
      embedding/minilm/        # optional pure-Go model adapter
      toolsearch/config.go     # tool search configuration
      toolsearch/searcher.go   # the stable ranking interface
      toolsearch/selector.go   # mode selection and automatic fallback
      toolsearch/lexical.go    # TF-IDF and cosine similarity
      toolsearch/documents.go  # internal tool documents
      toolsearch/index.go      # document vectors and cosine similarity
      provider/standard.go     # the standard adapter
      provider/request.go      # HTTP requests and content encoding
      provider/content.go      # text and media encoding
      provider/stream.go       # streamed response decoding
      provider/models.go       # model refresh
      provider/prices.go       # price tables
      provider/config.go       # the provider file
      provider/test.go         # the test provider
      store/store.go           # the data interfaces
      store/memory/memory.go   # the memory store
      store/postgres/postgres.go
      store/{memory,postgres}/
        instances.go           # instance records
        sessions.go            # session and message records
        queue.go               # durable pending messages
        events.go              # event records
        processes.go           # process records
        permissions.go         # permission decisions
        usage.go               # statistics
        providers.go           # provider and model records
        secrets.go             # provider keys
        settings.go            # harness and instance settings
      process/supervisor.go
      process/handle.go        # process IO and completion
      process/group_unix.go    # process-group signals
      permission/engine.go
      permission/broker.go     # the permission broker
      eventbus/eventbus.go
    organism/
      loop/loop.go             # the AgentLoop
      loop/queue.go            # the message queue
      loop/queue_lifecycle.go  # revert, recovery, and shutdown
      loop/queue_commands.go   # directory requests and responses
      loop/queue_directory.go  # session ownership and application lifecycle
      loop/queue_recovery.go   # restoration and instance control workers
      loop/session_coordinator.go # one command loop per session
      loop/coordinator_commands.go # session requests and completions
      loop/coordinator_operations.go # database workers
      loop/coordinator_execution.go # model workers
      loop/requests.go         # request preparation and policy
      loop/start_prompt.go     # request-local system instructions
      loop/responses.go        # model response collection
      loop/messages.go         # response and result persistence
      loop/models.go          # model selection and media checks
      loop/tools.go            # tool execution and discovery
      loop/agents.go           # child agents and completion
      loop/permissions.go      # permission decisions
      loop/usage.go            # model cost calculation
      loop/events.go           # loop events
      registry/registry.go     # the ToolRegistry
      registry/search.go       # category filters and tool discovery
      plugins/host.go          # the PluginHost
      instances/manager.go     # the InstanceManager
      processes/manager.go     # the ProcessManager
      processes/observer.go    # completion and output observers
      processes/notifications.go
      memory/session.go        # the SessionMemory
      gateway/gateway.go       # the ModelGateway
    api/
      server.go                # server wiring and routes
      authentication.go        # request authentication
      instances.go             # workspace instances
      sessions.go              # sessions and child agents
      messages.go              # queue, cancellation, and revert
      responses.go             # JSON and HTTP error helpers
      settings.go              # harness and instance settings
      provider_keys.go         # provider secrets
      providers.go             # provider model lists and refresh
      permissions.go           # permission responses
      statistics.go            # usage and cost
      processes.go             # session process lists
      websocket.go             # the event and output streams
    mcp/
      client.go                # the MCP client
      stdio.go                 # the standard input and output transport
      http.go                  # the streamable HTTP transport
      tool.go                  # the MCP tool adapter
      config.go                # the MCP server file
    tools/
      bash.go
      file_actions.go          # the file_actions tool and atomic action chain
      file_actions_input.go    # action and output validation
      file_actions_schema.go   # the model-facing input contract
      file_actions_inspect.go  # read-only actions and error diagnostics
      file_actions_output.go   # the shared output budget
      file_directory.go       # bounded directory pages
      file_mutations.go       # line writes and literal text replacement
      file_preview.go         # bounded file text and continuation cursors
      file_diff.go            # bounded unified diff output
      read.go                 # internal legacy adapter, not registered at startup
      write.go                # internal legacy adapter, not registered at startup
      replace.go              # internal legacy adapter, not registered at startup
      line_range.go            # shared line boundary rules
      file_edit.go             # per-path edit ordering and atomic replacement
      search.go                # the `search_tool` tool
      process.go               # the process tools
      agent.go
      finish.go                # the agent stop tool
      schema.go                # the input schema helper
  plugins/
    pathtools/pathtools.go     # a plugin in the same module
  scripts/ste
  internal/architecture/layers_test.go
  ste/terms.json
  Spec.md
  AGENTS.md
```

The module name in `go.mod` is `github.com/matheustavarestrindade/mtt-harness`.

### 16.2 Packages

The packages have 3 groups:

- `atom`: the atoms. The package does not use a package of the project.
- `harness`: the plugin interface. The package uses `atom` only.
- `internal`: the molecules, the organisms, the API, the MCP client, and the tools. A plugin must not use the internal packages. The architecture test examines the imports of a plugin in the module.

The bootstrap file gives the port, the database URL, the provider file, and the MCP file. The file `mtt.json` is local. The file `mtt.example.json` gives the keys.

The program in `cmd/mtt` reads the bootstrap file. Then the program attaches the tools, the providers, and the MCP servers. Then the program starts the API.

### 16.3 Rules

- R153: The `atom` package must not use a package of the project.
- R154: The `harness` package must use the `atom` package only.
- R155: A molecule must be in the `internal/molecule` directory.
- R156: An organism must be in the `internal/organism` directory.
- R157: The program must attach the tools and the plugins in the `plugins` directory.

### 16.4 Code

The Go code must use the full name of a variable. The name must give the purpose of the variable. For example:

```go
harnessRuntime := harness.New()
toolRegistry := registry.New()
```

The name of a function must give the operation and the object of the operation. Use `requestOAuthTokens` to identify a token request. Use `activateDiscoveredTools` to identify a tool group change.

The function must stop when the input is not correct or an operation gives an error. The error check must come before the primary operation.

An error helper must contain the error check for a group of functions. The start of the program can stop when an operation gives an error. A tool must give an error to the loop. A tool error must not stop the program.

The file `cmd/mtt/main.go` must start the program only. The code for configuration, the database, the providers, and MCP must be in different files. A file must have one purpose.

One registry must control one function of the harness. The code must not have 2 registries for the same purpose.

The behavior must stay the same after a change to the code structure. The test must examine the behavior after a change to the code structure.

## 17. Interfaces

Section 17 gives the interfaces for the code. Sections 6, 9, and 10 give the `Provider`, `Plugin`, and `Tool` interfaces.

### 17.1 Plugins

The `harness` package gives the `Harness` object and the function types:

```go
type Harness struct { /* ... */ }

func (harnessRuntime *Harness) On(event atom.EventName, handler Handler) Unsubscribe
func (harnessRuntime *Harness) Tool(tool Tool) Unsubscribe
func (harnessRuntime *Harness) Provider(provider Provider) Unsubscribe
func (harnessRuntime *Harness) Watch(watcher ProcessWatcher) Unsubscribe

func Pipe[Value any](harnessRuntime *Harness, stage atom.Stage[Value], middleware Middleware[Value]) Unsubscribe
func Decide[Value any](harnessRuntime *Harness, stage atom.Stage[Value], decision Decision[Value]) Unsubscribe

type Unsubscribe func()

type Handler func(operationContext context.Context, event atom.Event)
type Middleware[Value any] func(operationContext context.Context, value Value) (Value, error)
type Decision[Value any] func(operationContext context.Context, value Value) (atom.Verdict, error)
```

### 17.2 Molecules

The interfaces for the molecules are:

```go
type ContextBuilder interface {
    Build(operationContext context.Context, session atom.SessionID) ([]atom.Message, error)
}

type EventBus interface {
    Send(operationContext context.Context, event atom.Event) error
    On(name atom.EventName, handler Handler) Unsubscribe
}

type PermissionEngine interface {
    Cached(operationContext context.Context, session atom.Session, target string) (atom.PermissionDecision, bool, error)
    Remember(operationContext context.Context, session atom.Session, target string, decision atom.PermissionDecision) error
}

type ProcessSupervisor interface {
    Start(operationContext context.Context, processSpec atom.ProcessSpec) (Process, error)
    Get(id string) (Process, bool)
    All() []Process
}
```

### 17.3 Data Interfaces

The interfaces for the database are:

```go
type InstanceStore interface {
    Save(operationContext context.Context, instanceSpec atom.InstanceSpec) error
    Get(operationContext context.Context, instanceID string) (atom.InstanceSpec, error)
    All(operationContext context.Context) ([]atom.InstanceSpec, error)
    Delete(operationContext context.Context, instanceID string) error
}

type SessionStore interface {
    Save(operationContext context.Context, session atom.Session) error
    SetModelSelection(operationContext context.Context, sessionID atom.SessionID, previous, next atom.SessionModelSelection) error
    GetModelSelection(operationContext context.Context, sessionID atom.SessionID) (atom.SessionModelSelection, bool, error)
    Get(operationContext context.Context, sessionID atom.SessionID) (atom.Session, error)
    Agents(operationContext context.Context, parent atom.SessionID) ([]atom.SessionID, error)
    List(operationContext context.Context, instanceID string) ([]atom.Session, error)
    Append(operationContext context.Context, message atom.Message) error
    Messages(operationContext context.Context, sessionID atom.SessionID) ([]atom.Message, error)
    DeleteAfter(operationContext context.Context, sessionID atom.SessionID, messageID string) (int, error)
    DeleteConversation(operationContext context.Context, sessionID atom.SessionID) ([]atom.SessionID, error)
}

type EventStore interface {
    Append(operationContext context.Context, event atom.Event) error
    Record(operationContext context.Context, event atom.Event) (atom.Event, error)
    Since(operationContext context.Context, instanceID string, sequenceNumber uint64) ([]atom.Event, error)
}

type ProcessStore interface {
    Save(operationContext context.Context, process atom.ProcessRecord) error
    Get(operationContext context.Context, processID string) (atom.ProcessRecord, error)
    List(operationContext context.Context, session atom.SessionID) ([]atom.ProcessRecord, error)
    CountRunning(operationContext context.Context, instanceID string) (int, error)
}

type PermissionStore interface {
    Save(operationContext context.Context, decision atom.PermissionDecision) error
    Get(operationContext context.Context, requestID string) (atom.PermissionDecision, error)
    Resolve(operationContext context.Context, session atom.Session, target string) (atom.PermissionDecision, bool, error)
}

type UsageStore interface {
    Save(operationContext context.Context, record atom.UsageRecord) error
    Session(operationContext context.Context, sessionID atom.SessionID) (atom.Statistics, error)
    Instance(operationContext context.Context, instanceID string) (atom.Statistics, error)
    All(operationContext context.Context) (atom.Statistics, error)
}

type ProviderStore interface {
    Save(operationContext context.Context, providerSpec atom.ProviderSpec) error
    Get(operationContext context.Context, providerID string) (atom.ProviderSpec, error)
    All(operationContext context.Context) ([]atom.ProviderSpec, error)
    Delete(operationContext context.Context, providerID string) error
    SaveModels(operationContext context.Context, provider string, models []atom.ModelInfo) error
    Models(operationContext context.Context, provider string) ([]atom.ModelInfo, error)
}

type QueueStore interface {
    Enqueue(operationContext context.Context, message atom.Message, limit int) error
    All(operationContext context.Context) ([]atom.QueuedMessage, error)
    Start(operationContext context.Context, messageID string) error
    Finish(operationContext context.Context, messageID string) error
    Remove(operationContext context.Context, sessionID atom.SessionID, messageID string) (bool, error)
    ClearPending(operationContext context.Context, sessionID atom.SessionID) (int, error)
}

type SecretStore interface {
    SaveOAuthCredential(operationContext context.Context, provider string, credential atom.OAuthCredential) error
    OAuthCredential(operationContext context.Context, provider string) (atom.OAuthCredential, error)
    DeleteOAuthCredential(operationContext context.Context, provider string) error
    SaveProviderKey(operationContext context.Context, provider string, key string) error
    ProviderKey(operationContext context.Context, provider string) (string, error)
    SaveInstanceKey(operationContext context.Context, instanceID string, provider string, key string) error
    InstanceKey(operationContext context.Context, instanceID string, provider string) (string, error)
    ResolveKey(operationContext context.Context, instanceID string, provider string) (string, error)
    DeleteProviderKey(operationContext context.Context, provider string) error
    DeleteInstanceKey(operationContext context.Context, instanceID string, provider string) error
}

type SettingsStore interface {
    Save(operationContext context.Context, scope string, key string, value string) error
    Get(operationContext context.Context, scope string, key string) (string, error)
    All(operationContext context.Context, scope string) (map[string]string, error)
    Delete(operationContext context.Context, scope string, key string) error
    Resolve(operationContext context.Context, instanceID string, key string) (string, error)
}
```

### 17.4 Organisms

The interfaces for the organisms are:

```go
type InstanceManager interface {
    Start(operationContext context.Context, instanceSpec atom.InstanceSpec) (*instances.Instance, error)
    Get(instanceID string) (*instances.Instance, bool)
    All() []*instances.Instance
    Stop(operationContext context.Context, instanceID string) error
    IsRunning(instanceID string) bool
    AgentDepthLimit(operationContext context.Context, instanceID string) (int, error)
    ProcessLimit(operationContext context.Context, instanceID string) (int, error)
}

type Instance interface {
    ID() string
    Workspace() string
    Sessions() SessionManager
}

type SessionManager interface {
    Start(operationContext context.Context, parent atom.SessionID) (atom.SessionID, error)
    Get(operationContext context.Context, sessionID atom.SessionID) (atom.Session, bool)
    Agents(operationContext context.Context, parent atom.SessionID) ([]atom.SessionID, error)
}

type ToolRegistry interface {
    Add(tool Tool) error
    Get(name string) (Tool, bool)
    All() []Tool
    Find(query string, category string, limit int) []Tool
    Upsert(tool Tool)
    Remove(name string)
}

type ModelGateway interface {
    Add(provider Provider) error
    Provider(name string) (Provider, bool)
    Model(modelID string) (atom.ModelInfo, Provider, bool)
    Resolve(modelID string) (atom.ModelInfo, Provider, error)
    ResolveAllowed(modelID string, allowed []string) (atom.ModelInfo, Provider, error)
    Refresh(operationContext context.Context, providerID string) ([]atom.ModelInfo, error)
}
```

Requirements:

- R158: The `harness` package must contain the `Harness`, `Plugin`, `Tool`, `Provider`, and `ProcessWatcher` interfaces.
- R159: The `internal/molecule/store` package must contain the data interfaces.
- R160: The Postgres adapter must use the data interfaces.

### 17.5 Runtime Requirements

- R161: File tools and shell commands must use the workspace of the instance.
- R162: A model call must use a model from the model list of the instance.
- R163: An MCP connection must continue until the harness stops or the connection gives an error.
- R164: The API must wait for the current run to stop before it removes messages.
- R165: A permission decision must apply only to the instance and session of the decision scope.
- R166: The database must keep a message in the queue before the API gives status `202`.
- R167: The harness must use the prices from the provider configuration before prices from the database.
- R168: The standard adapter must give an error for a content type which the adapter cannot send.
- R169: The context limit must not remove a message from the database.
- R170: The event stream must use the sequence number from the database.
- R171: The harness must keep the data of a stopped instance.
- R172: The queue must give an error when the number of messages is at the queue limit.
- R173: One goroutine must change the queue state of a session coordinator.
- R174: The session coordinator must use commands and completion messages to control the workers.
- R175: A command handler must not wait for a model call, a tool call, or a database operation.
- R176: The coordinator must continue after a caller cancels a command request.
- R177: The `Close` operation must wait until the queue workers stop. Then the program can close the database.
- R178: A revert operation must not start work in a stopped coordinator.
- R179: A tool input schema must give the parameter descriptions, units, defaults, and the meaning of special input data.
- R180: A model request must keep the tool parameter descriptions when the harness changes the model list.
- R181: The file tools must use the same line number and range rules.
- R182: The `read` tool must give only the selected file lines.
- R183: A file edit with a range must keep the file data before and after the range.
- R184: The `replace` tool must have the mode values `first`, `last`, and `all`.
- R185: The `replace` tool must not change the file when it cannot find the text.
- R186: The line prefix must be optional for the A/B test.

## 18. Protection

- The harness must not write a secret to the event record.
- The path guard must resolve a symbolic link before the path check.
- The verdict `deny` must stop the tool call before the `Run` function.
- The harness must only add to the event record.
- The harness must keep the secrets in the database.
- The initial start must make the `api_token` and show the token one time.
- The bootstrap file must not be in the repository.
- The MCP file must not be in the repository.

## 19. Milestones

The milestones are:

- Milestone 0: the loop and a model with test data. Goal: the loop runs.
- Milestone 1: tools and the permission system. Goal: tools run with checks.
- Milestone 2: the Postgres database and instances. Goal: data continues after a start and the instances stay apart.
- Milestone 3: processes and notifications. Goal: a process runs and sends notifications.
- Milestone 4: the plugin system and the MCP servers. Goal: a plugin changes the loop and an MCP server gives tools.
- Milestone 5: agents. Goal: a child agent gives a result to the parent session.
- Milestone 6: vector recall. Goal: the harness finds messages with an equivalent meaning.

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
        ctx     = build_context(session)
        request = pipe("model.request", ctx)
        stream  = model.stream(request)               # boundary 2

        tasks = []
        for part in stream:
            if part is a tool call with the full input:
                call  = pipe("action.plan", part)     # boundary 3
                input = pipe("tool.input", call)
                tasks.append(start(run_tool, input))  # boundary 4

        message = pipe("model.response", stream)
        write(message)

        for task in tasks:
            result = pipe("tool.result", wait(task))
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
- R7: The loop must wait for the tool results before the next model call.
- R8: The loop must stop when the model message does not have a tool call.
- R9: The loop must send the value of a stage through the pipeline of the stage.
- R10: The harness must run the messages of a session in sequence.
- R11: The harness must keep the new messages of a session in a queue.

The messages of a session wait in a queue. The harness runs one message at a time. Thus, the user can send a new message while the harness runs a message.

## 5. Instances

A user starts an instance with a workspace directory. The harness gives an ID to the instance. The instance start request gives:

- The workspace directory.
- The model list and the model configuration.
- The process limit and the agent depth limit.

The model list gives the models for the sessions. A model list entry has an ID and a level. The instance uses the default model for a session which does not give a model.

The harness is one program. Many instances can run at the same time. The sessions, messages, tool calls, events, and processes of an instance stay in the instance. The harness must not mix the data of 2 instances.

An instance can stop and start again with the same data.

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
    Stream(ctx context.Context, request Request) (Stream, error)
}

type ModelInfo struct {
    ID         string
    Level      int
    Input      []MediaType
    Output     []MediaType
    Tools      bool
    ContextMax int
    Prices     *Prices
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

The model can give the prices of the tokens. The prices are for 1,000,000 tokens. A model call gives the usage. The usage has:

- The input tokens.
- The cache read tokens.
- The cache write tokens.
- The output tokens.
- The reasoning tokens.
- The cost, when the model gives the prices.

The harness calculates the cost from the usage and the prices. The cost data is optional. Thus, the cost can be empty. The usage has the tokens.

The cache hit rate is the cache read tokens divided by the full input tokens. The full input tokens are the input tokens, the cache read tokens, and the cache write tokens.

```go
type Prices struct {
    Currency   string
    Input      float64
    Output     float64
    CacheRead  float64
    CacheWrite float64
}

type Cost struct {
    Currency string
    Value    float64
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
}
```

Requirements:

- R33: The prices must have a currency.
- R34: A provider must give the usage of a model call when the response has the usage.
- R35: The harness must record the usage of the model messages.
- R36: The harness must calculate the cost of a model call from the usage and the prices.
- R37: The harness must not give a cost when the model does not give the prices.
- R38: The cache hit rate must be the cache read tokens divided by the full input tokens.
- R39: The statistics must have the input tokens, the output tokens, the cache read tokens, the cache write tokens, the cache hit rate, and the cost.
- R40: The statistics for different currencies must stay apart.

### 6.2 Provider Data

The harness reads the providers from a JSON file. The file has one entry for a provider. The provider data has:

- The name of the provider.
- The API URL.
- The model list URL.
- The price table URL. The field is optional.
- The interval for the model list.
- The prices of the models. The field is optional.
- The models. The field is optional.

The harness keeps the provider data, the model lists, and the prices in the database. Thus, the instances read the model data from the database.

The secret of a provider is not in the file. The secret is in the database. The API sets a secret for the harness and a secret for one instance. The harness uses the secret of the instance before the secret of the harness.

A model list endpoint usually gives the model IDs and the limits. A model list endpoint can give the prices. Usually, the endpoint does not give the prices. Thus, the prices can come from:

- The model list response.
- The prices in the provider file.
- A price table in a file or at a URL.

The harness refreshes the model data at the interval. The default interval is 24 hours. The user can refresh the model data from the API. The harness keeps the last model list when the API gives an error.

A provider can use the `Refresher` interface:

```go
type Refresher interface {
    Refresh(ctx context.Context) ([]atom.ModelInfo, error)
}
```

The standard adapter uses `Refresher` when the provider data has a model list URL. Thus, the harness finds a new model from the API. A plugin can also refresh the model data.

Requirements:

- R41: The harness must read the provider data from a JSON file.
- R42: The provider data must give a name, an API URL, and a model list URL.
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
    Setup(h *Harness) error
}
```

The harness object gives 6 operations:

```go
func (h *Harness) On(event EventName, handler Handler) Unsubscribe
func (h *Harness) Tool(tool Tool) Unsubscribe
func (h *Harness) Provider(provider Provider) Unsubscribe
func (h *Harness) Watch(watcher ProcessWatcher) Unsubscribe

func Pipe[T any](h *Harness, stage Stage[T], fn Middleware[T]) Unsubscribe
func Decide[T any](h *Harness, stage Stage[T], fn Decision[T]) Unsubscribe
```

The Go compiler does not let a method have a type parameter. Thus, `Pipe` and `Decide` are functions.

Requirements:

- R57: A plugin must not use the internal packages of the harness.
The harness can also get tools from an MCP server. The harness reads the servers from the MCP file. A server can have a program or a URL. The harness starts a server with a program. The harness connects to a server with a URL. Then the harness reads the tools of the server and gives the tools to the model.

The name of an MCP tool is `mcp__{server}__{tool}`. Thus, 2 servers can have a tool with the same name.

Requirements:

- R58: A plugin must not use the internal packages of the harness.
- R59: The harness must read the MCP servers from the MCP file.
- R60: The harness must start an MCP server that has a program.
- R61: The harness must connect to an MCP server that has a URL.
- R62: The harness must use the MCP protocol for the tools of a server.
- R63: The name of an MCP tool must be `mcp__{server}__{tool}`.
- R64: The harness must refresh the tools when an MCP server gives a change message.
- R65: The harness must run handlers in the sequence that the plugin attaches them.
- R66: The harness must stop the pipeline when a handler gives an error.
- R67: A plugin must attach a tool, a watcher, or a stage handler before the loop starts.

- R68: The harness must run handlers in the sequence that the plugin attaches them.
- R69: The harness must stop the pipeline when a handler gives an error.
- R70: A plugin must attach a tool, a watcher, or a stage handler before the loop starts.

## 10. Tools

The Go interface of a tool is:

```go
type Tool interface {
    Name() string
    Description() string
    Categories() []string
    InputSchema() Schema
    Check(ctx context.Context, call ToolCall) Verdict
    Run(ctx context.Context, call ToolCall) (ToolResult, error)
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

### 10.2 Find Tools

The harness gives the model the `search_tool` tool. The model starts `search_tool` with a query. The query has a category or text. The tool result gives the tools which agree with the query. Each found tool gives:

- The name and the description.
- The category.
- The input schema.

The `search_tool` function compares the query with the search text. The search text is the name, the description, and the categories of a tool. The function divides the query into words. A tool agrees with the query when the search text has the words of the query.

The function must ignore the difference between `A` and `a`. The function uses code only. Thus, the function does not use a model.

The input fields of `search_tool` are:

- `query`: the text which the function compares.
- `category`: the category of the tools. The field is optional.
- `limit`: the maximum number of tools in the result. The default is 10. The maximum is 50.

The sequence of the result has 3 groups. The category group is before the name group. The name group is before the description group. Thus, the result is the same for the same query and the same tool group. The tool result can be empty.

The harness adds the found tools to the tool group of the session. Thus, the model can start the found tools on the next turn. The model can start `search_tool` again for other tools.

The harness gives the tool group and the `search_tool` tool in the model request. A plugin can change the tool group at the `model.request` stage.

Requirements:

- R80: A tool must have one or more categories.
- R81: The `search_tool` function must compare the query with the search text.
- R82: The `search_tool` function must not use a model.
- R83: The `search_tool` function must divide the query into words.
- R84: A tool must agree with the query when the search text of the tool has the words of the query.
- R85: The `search_tool` function must ignore the difference between `A` and `a`.
- R86: The `category` field must agree with the category of a tool.
- R87: The result must not have a number of tools above the `limit` field.
- R88: The default limit must be 10 and the maximum limit must be 50.
- R89: The result must put the tools which agree by category before the tools which agree by name.
- R90: The result must put the tools which agree by name before the tools which agree by description.
- R91: The `search_tool` function must give the same result for the same query and the same tool group.
- R92: The harness must give the `search_tool` tool to the model.
- R93: The result of `search_tool` must give the name, the description, the category, and the input schema of the found tools.
- R94: The harness must add the found tools to the tool group of the session.
- R95: The harness must give the tool group and the `search_tool` tool in the model request.
- R96: The harness must not give a tool to the model when the tool is not in the tool group.

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
    OnMatch(ctx context.Context, event ProcessEvent)
}
```

### 11.1 Notification Policy

The tool call gives the notification policy for a process. The policy has the mode and the interval. The mode is one of 3 values:

- `exit`: the harness sends one notification when the process stops.
- `error`: the harness sends one notification when the process stops with an error status.
- `interval`: the harness sends one notification at the interval.

The interval is a number of milliseconds. The default mode is `exit`. A notification is a message in the session. Thus, the model reads the notification on the next turn.

Requirements:

- R97: The harness must record a process and the status of a process in the database.
- R98: The harness must send an output event to the watchers.
- R99: A process must continue after the turn which starts it.
- R100: A tool can read the output of a process.
- R101: The harness must have a limit for the output buffer.
- R102: The harness must stop a process after the timeout.
- R103: A tool call must give the notification policy of the process.
- R104: The notification policy must have the mode `exit`, `error`, or `interval`.
- R105: On the mode `exit`, the harness must send a notification when the process stops.
- R106: On the mode `error`, the harness must send a notification when the process stops with an error status.
- R107: On the mode `interval`, the harness must send a notification at the interval.
- R108: The default notification mode must be `exit`.
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
- `POST /instances/{id}/sessions`: start a session.
- `GET /instances/{id}/sessions`: read the sessions of an instance.
- `GET /instances/{id}/models`: read the model list of an instance.
- `GET /sessions/{id}`: read a session.
- `GET /sessions/{id}/messages`: read the messages of a session.
- `POST /sessions/{id}/cancel`: stop the current run.
- `GET /sessions/{id}/status`: read the status of the current run.
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

Requirements:

- R134: The API must send a permission request to the attached clients.
- R135: The API must use the initial response to a permission request.
- R136: The API must use a token.
- R137: The API must send an event with a sequence number.
- R138: The API must start an instance from a workspace directory.
- R139: The API must give the sessions of an instance.
- R140: The API must put a user message in the queue and must not wait for the current run.
- R141: The API must stop the current run of a session.
- R142: The API must give the status of the current run of a session.
- R143: The API must remove the messages after a message.
- R144: When the user reverts a session, the harness must stop the current run and empty the message queue of the session.
- R145: The API must give the model list of an instance.
- R146: The API must give the settings and change the settings.
- R147: The API must give the statistics of a session.
- R148: The API must give the statistics of an instance.
- R149: The API must give the full statistics of the harness.
- R150: The API must give the providers and the provider models.
- R151: The API must let the user refresh the model data of a provider.

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
- `PermissionSystem`
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
  mtt.example.json             # the example bootstrap file
  providers.json               # the provider data
  mcp.example.json             # the example MCP server file
  cmd/mtt/main.go              # start the API
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
    context.go                 # the session in the context
    plugin.go                  # the Plugin interface
    tool.go                    # the Tool interface
    provider.go                # the Provider interface
    process.go                 # the Process interface
  internal/
    config/config.go           # the bootstrap file
    molecule/
      schema/schema.go         # the input schema check
      pipeline/pipeline.go     # the middleware chain
      contextbuilder/context.go
      provider/standard.go     # the standard adapter
      provider/config.go       # the provider file
      provider/test.go         # the test provider
      store/store.go           # the data interfaces
      store/memory/memory.go   # the memory store
      store/postgres/postgres.go
      process/supervisor.go
      permission/engine.go
      permission/broker.go     # the permission broker
      eventbus/eventbus.go
    organism/
      loop/loop.go
      loop/queue.go            # the message queue             # the AgentLoop
      registry/registry.go     # the ToolRegistry
      plugins/host.go          # the PluginHost
      instances/manager.go     # the InstanceManager
      processes/manager.go     # the ProcessManager
      memory/session.go        # the SessionMemory
      permissions/system.go    # the PermissionSystem
      gateway/gateway.go       # the ModelGateway
    api/
      server.go                # the HTTP handlers
      websocket.go             # the event and output streams
    mcp/
      client.go                # the MCP client
      stdio.go                 # the standard input and output transport
      http.go                  # the streamable HTTP transport
      tool.go                  # the MCP tool adapter
      config.go                # the MCP server file
    tools/
      bash.go
      read.go
      write.go
      search.go                # the `search_tool` tool
      process.go               # the process tools
      agent.go
      finish.go                # the agent stop tool
      schema.go                # the input schema helper
  plugins/
    pathtools/pathtools.go     # a plugin in the same module
  scripts/ste
  ste/terms.json
  Spec.md
  AGENTS.md
```

The module name in `go.mod` is `github.com/matheustavarestrindade/mtt-harness`.

### 16.2 Packages

The packages have 3 groups:

- `atom`: the atoms. The package does not use a package of the project.
- `harness`: the plugin interface. The package uses `atom` only.
- `internal`: the molecules, the organisms, the API, the bridge, and the tools. A plugin cannot use the `internal` packages. The Go compiler gives an error.

The bootstrap file gives the port, the database URL, the provider file, and the plugin file. The file `mtt.json` is local. The file `mtt.example.json` gives the keys.

The program in `cmd/mtt` reads the bootstrap file. Then the program attaches the tools, the providers, and the MCP servers. Then the program starts the API.

### 16.3 Rules

- R152: The `atom` package must not use a package of the project.
- R153: The `harness` package must use the `atom` package only.
- R154: A molecule must be in the `internal/molecule` directory.
- R155: An organism must be in the `internal/organism` directory.
- R156: The program must attach the tools and the plugins in the `plugins` directory.

## 17. Interfaces

Section 17 gives the interfaces for the code. Sections 6, 9, and 10 give the `Provider`, `Plugin`, and `Tool` interfaces.

### 17.1 Plugins

The `harness` package gives the `Harness` object and the function types:

```go
type Harness struct { /* ... */ }

func (h *Harness) On(event atom.EventName, handler Handler) Unsubscribe
func (h *Harness) Tool(tool Tool) Unsubscribe
func (h *Harness) Provider(provider Provider) Unsubscribe
func (h *Harness) Watch(watcher ProcessWatcher) Unsubscribe

func Pipe[T any](h *Harness, stage atom.Stage[T], fn Middleware[T]) Unsubscribe
func Decide[T any](h *Harness, stage atom.Stage[T], fn Decision[T]) Unsubscribe

type Unsubscribe func()

type Handler func(ctx context.Context, event atom.Event)
type Middleware[T any] func(ctx context.Context, value T) (T, error)
type Decision[T any] func(ctx context.Context, value T) (atom.Verdict, error)
```

### 17.2 Molecules

The interfaces for the molecules are:

```go
type ContextBuilder interface {
    Build(ctx context.Context, session atom.SessionID) ([]atom.Message, error)
}

type EventBus interface {
    Send(event atom.Event)
    On(name atom.EventName, handler Handler) Unsubscribe
}

type PermissionEngine interface {
    Ask(ctx context.Context, request atom.PermissionRequest) (atom.PermissionDecision, error)
}

type ProcessSupervisor interface {
    Start(ctx context.Context, spec atom.ProcessSpec) (Process, error)
    Get(id string) (Process, bool)
    All() []Process
}
```

### 17.3 Data Interfaces

The interfaces for the database are:

```go
type InstanceStore interface {
    Save(ctx context.Context, spec atom.InstanceSpec) error
    Get(ctx context.Context, id string) (atom.InstanceSpec, error)
    All(ctx context.Context) ([]atom.InstanceSpec, error)
    Delete(ctx context.Context, id string) error
}

type SessionStore interface {
    Save(ctx context.Context, session atom.Session) error
    Get(ctx context.Context, id atom.SessionID) (atom.Session, error)
    Agents(ctx context.Context, parent atom.SessionID) ([]atom.SessionID, error)
    Append(ctx context.Context, message atom.Message) error
    Messages(ctx context.Context, id atom.SessionID) ([]atom.Message, error)
}

type EventStore interface {
    Append(ctx context.Context, event atom.Event) error
    Since(ctx context.Context, instanceID string, seq uint64) ([]atom.Event, error)
}

type ProcessStore interface {
    Save(ctx context.Context, process atom.ProcessRecord) error
    Get(ctx context.Context, id string) (atom.ProcessRecord, error)
    List(ctx context.Context, session atom.SessionID) ([]atom.ProcessRecord, error)
}

type PermissionStore interface {
    Save(ctx context.Context, decision atom.PermissionDecision) error
    Get(ctx context.Context, id string) (atom.PermissionDecision, error)
}

type UsageStore interface {
    Save(ctx context.Context, record atom.UsageRecord) error
    Session(ctx context.Context, id atom.SessionID) (atom.Statistics, error)
    Instance(ctx context.Context, id string) (atom.Statistics, error)
    All(ctx context.Context) (atom.Statistics, error)
}

type ProviderStore interface {
    Save(ctx context.Context, spec atom.ProviderSpec) error
    Get(ctx context.Context, id string) (atom.ProviderSpec, error)
    All(ctx context.Context) ([]atom.ProviderSpec, error)
    Delete(ctx context.Context, id string) error
    SaveModels(ctx context.Context, provider string, models []atom.ModelInfo) error
    Models(ctx context.Context, provider string) ([]atom.ModelInfo, error)
}
```

### 17.4 Organisms

The interfaces for the organisms are:

```go
type InstanceManager interface {
    Start(ctx context.Context, spec atom.InstanceSpec) (Instance, error)
    Get(id string) (Instance, bool)
    All() []Instance
    Stop(ctx context.Context, id string) error
}

type Instance interface {
    ID() string
    Workspace() string
    Sessions() SessionManager
}

type SessionManager interface {
    Start(ctx context.Context, parent atom.SessionID) (atom.SessionID, error)
    Get(ctx context.Context, id atom.SessionID) (atom.Session, error)
    Agents(ctx context.Context, parent atom.SessionID) ([]atom.SessionID, error)
}

type ToolRegistry interface {
    Add(tool Tool) error
    Get(name string) (Tool, bool)
    All() []Tool
    Find(query string, category string, limit int) []Tool
}

type ModelGateway interface {
    Add(provider Provider) error
    Provider(name string) (Provider, bool)
    Model(id string) (atom.ModelInfo, Provider, bool)
}
```

Requirements:

- R157: The `harness` package must contain the `Harness`, `Plugin`, `Tool`, `Provider`, and `ProcessWatcher` interfaces.
- R158: The `internal/molecule/store` package must contain the data interfaces.
- R159: The Postgres adapter must use the data interfaces.

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
- Milestone 4: the plugin system and the bridge. Goal: a plugin changes the loop.
- Milestone 5: agents. Goal: a child agent gives a result to the parent session.
- Milestone 6: vector recall. Goal: the harness finds messages with an equivalent meaning.

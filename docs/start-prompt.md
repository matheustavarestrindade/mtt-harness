# Startup Prompt

## Configuration

The file `start_prompt.md` supplies the system instructions for the providers. The primary session and child agents use the same template.

The harness reads the file one time when the program starts. To select a different file, set `start_prompt_file` in `mtt.json`:

```json
{
  "start_prompt_file": "start_prompt.md"
}
```

The default path is `start_prompt.md`. Relative paths use the directory from which you start the harness. A path that starts with `/` does not use the process directory.

The command-line argument replaces the value from the bootstrap file:

```sh
go run ./cmd/mtt --start-prompt-file ./prompts/coding.md
```

An empty file does not supply system instructions. If the file is not available, the program stops. An error while the harness reads the file also stops the program. An unknown variable causes a startup error.

After a template change, stop and start the harness.

## Default Behavior

The default template gives tool discovery and selection rules. The model must read the full tool description and input schema before a tool call. The tool definition gives the rules for the operation. The template does not give a different file action procedure.

The model uses a definition that is available in the request. If the definition is not available, the model uses `search_tool`. The next model request gives the full definition. A tool name does not supply a tool definition.

The default template includes the tool catalog, not full tool definitions. The initial request gives `search_tool` through the provider tool data. A file tool definition is available after tool discovery. Child sessions also have the `finish` tool.

File edits can use information from the user, previous messages, or tool results. The model gets more file data only when necessary for a file edit. The tool definition gives output rules and steps for an error. The model must run the checks necessary for the task.

## Task State

The default template gives the task tracking rules. Task tracking is optional. The model gets the `task_state` definition through tool discovery before a tool call.

The loop adds a task state snapshot before context and request stages. After 3 model responses without a task update, it adds a task reminder. The task reminder tells the model to change task state before other work. Tool discovery comes first when the definition is not available. The task reminder does not stop other tools.

The task state snapshot and task reminder are not written to the database. The context limit includes them. See `task-state.md` for task update and response counter rules.

## Spaced Repetition

The optional spaced repetition plugin uses 3 smaller prompt templates. The files use the same template renderer as the startup prompt. A reminder does not activate tools. The bootstrap file field `spaced_repetition_prompts` and command-line argument of the same name select the directory.

The plugin adds reminders after context growth. It keeps the initial startup prompt and previous reminder text stable. Instruction recovery uses workspace memory search at compression level `M`. See `../plugins/spaced_repetition/README.md`.

## User Communication

The default template uses ISO 24495-1 for messages to the user. English text also uses ASD-STE100. The model uses the chat language that the user selects.

The template has 4 goals for messages:

- `Relevant`: give the information necessary for the task.
- `Findable`: put information in a clear sequence.
- `Understandable`: use clear text.
- `Usable`: give the information necessary for the next action.

The rules apply to messages to the user. The model keeps messages to other agents short.

See:

- [ISO 24495-1:2023](https://www.iso.org/standard/78907.html)
- [`IPLF`](https://www.iplfederation.org/iso-standard/)

## Variables

Put a variable between braces, for example `{workspace}`. Use the same text as the variable name in the list.

- `{tool_list}`: the name and categories of the tools in the registry, in name sequence.
- `{NAME_info}`: the full definition of the tool with the name `NAME`.
- `{workspace}`: the workspace directory of the instance.
- `{session_id}`: the ID of the session.
- `{instance_id}`: the ID of the instance.
- `{model}`: the session model, or the instance default if the session model is empty.
- `{agent_depth}`: the depth of the agent. The primary session has depth 0.
- `{os}`: the system on which the harness runs, for example `linux`.
- `{arch}`: the architecture of the harness program, for example `amd64`.

The model value is from the session before the pipeline stages run. A plugin can change the model after substitution. In Docker, the system and workspace values are for the container.

For example, `{bash_info}` gives the definition of `bash`. `{file_actions_info}` gives the definition of `file_actions`. `{mcp__server__lookup_info}` gives the definition of the MCP tool `mcp__server__lookup`.

A tool definition is JSON with `name`, `description`, `categories`, and `input_schema`. The schema includes parameter descriptions, defaults, and units. The definition of `agent` includes the model list for the instance.

Tool variables do not change the session tool group. The initial request has the definition of `search_tool`. The model uses `search_tool` to add tools to the group. The model gets `file_actions` through tool discovery. Tool permissions and input checks continue to apply.

The harness gets tool data from the registry for a model request. If a tool in a variable is not in the registry, the turn gives an error before the model call. Other sessions can continue. The rule also applies if an MCP server removes a tool.

Search documents and vectors are not included in the template values. The usage documents for tool discovery are not included.

## Example

```markdown
You are a coding assistant. Complete the user's task in {workspace}.

Registered tools:
{tool_list}

Command tool instructions:
{bash_info}

File action instructions:
{file_actions_info}
```

The regular expression for a variable name is:

```text
[A-Za-z_][A-Za-z0-9_.:-]*
```

The variable name must use the same letter case as the variable or tool name.

Use 2 braces before and after the variable name for literal text. For example, `{{tool_list}}` becomes `{tool_list}` without a tool list. JSON braces do not change, for example `{"nested":{"value":1}}`.

Substitution occurs one time. Braces in a tool description or a workspace path do not cause substitution again. Variables do not run commands or read environment variables. Other text does not change.

## Model Requests

The loop puts the system instructions before the session messages from the database for a model call. Subsequent model calls get one copy. A child agent gets the session ID, model, and depth of the session for the child agent.

The system message from the template is not written to the database. System messages from the database stay after it. Context and request middleware can change the system instructions. The context limit includes the system message.

Task state and task reminders are runtime data after conversation messages. They do not change the system prompt prefix. The context plugin keeps memory snapshot text, sequence, and position until context removal. Context notices are also runtime data after conversation messages.

The tool list does not change when a plugin is set to `OFF`. Tool discovery and tool definitions use the applied plugin state. A tool definition is necessary before a tool call.

For the `chat_completions` protocol, the adapter sends a system message. For the `responses` protocol, the adapter sends the text in `instructions`. Provider adapters do not add default system instructions when the text is empty.

## Docker

The `core` and `runtime` images contain the template at `/etc/mtt/start_prompt.md`. The default image command selects the path. If you replace the image command, include `--start-prompt-file /etc/mtt/start_prompt.md`.

The Docker configuration sets the command-line argument `--start-prompt-file`. The template and configuration files use read-only mounts at `/etc/mtt`.

The host file is `./start_prompt.md`. The file path in the container is `/etc/mtt/start_prompt.md`. Change the host file, then run:

```sh
docker compose up -d --force-recreate mtt
```

The command also applies a file replacement from an editor. A new image build is not necessary when you change the host template file.

## Code

- `internal/molecule/startprompt/`: the template, variable checks, and text substitution.
- `internal/organism/loop/start_prompt.go`: session values and registry access.
- `internal/organism/loop/requests.go`: the system message before the pipeline.
- `cmd/mtt/application.go`: the code which reads the template when the program starts.

The molecule uses atoms and the standard library. The organism supplies registry access. The template does not use a provider or a store.

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

For example, `{bash_info}` gives the definition of `bash`. `{read_info}` gives the definition of `read`. `{mcp__server__lookup_info}` gives the definition of the MCP tool `mcp__server__lookup`.

A tool definition is JSON with `name`, `description`, `categories`, and `input_schema`. The schema includes parameter descriptions, defaults, and units. The definition of `agent` includes the model list for the instance.

Tool variables do not change the session tool group. The model uses `search_tool` to add tools to the group. Tool permissions and input checks continue to apply.

The harness gets tool data from the registry for a model request. If a tool in a variable is not in the registry, the turn gives an error before the model call. Other sessions can continue. The rule also applies if an MCP server removes a tool.

Search documents and vectors are not included in the template values. The usage documents for tool discovery are not included.

## Example

```markdown
You are a coding assistant. Complete the user's task in {workspace}.

Registered tools:
{tool_list}

Command tool instructions:
{bash_info}

File read instructions:
{read_info}
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

# MTT Console

The UI is a client of the harness API. The harness keeps the sessions, queue, tools, permissions, and data. The UI sends HTTP requests and reads WebSocket events.

## Start the UI

The minimum Node.js version is `22.12`. Start the harness with the commands in `../AGENTS.md`. The default harness URL is `http://localhost:18080`.

Run `../build.sh` for the native harness and UI, with PostgreSQL in Docker. The native UI uses the Vite proxy and port `18081`. The UI files are in `ui/dist`. See `../docs/native-start.md`.

Run the commands from `ui/`:

```sh
npm ci
cp .env.example .env
npm run dev
```

Open `http://localhost:5174`. Select the button with the label `Connect harness`. The API URL is `/api`. The credential is in the harness configuration or initial console output.

The UI keeps the token in session storage for the browser tab. Do not put a token in a frontend environment variable.

The UI proxy URL is in `ui/.env`:

```dotenv
HARNESS_API_URL=http://127.0.0.1:18080
```

Vite uses the URL for HTTP and WebSocket requests. The browser uses `/api` on the UI URL. The proxy does not change harness CORS behavior. Start Vite again after a proxy change.

## Docker and Tailscale

The Docker image and Docker Compose file are in `ui/`. The image contains the UI and an nginx proxy. Run `../start.sh` for PostgreSQL, the harness, and the UI. The Docker Compose file uses an internal API connection.

Use the subsequent command for a different UI with a harness on the host computer.

```sh
# From ui/
docker compose up --build -d
```

The default UI port is `18081`. The container port is `8080`. The host bind address is `0.0.0.0`, which includes the Tailscale interface.

```text
Local:     http://localhost:18081
Tailscale: http://HOST_TAILSCALE_IP:18081
API URL in the connection form: /api
```

The host Tailscale address is `100.82.190.3`. The URL is `http://100.82.190.3:18081`. The connection must agree with Tailscale access rules and the host firewall.

Container configuration in `ui/.env`:

```dotenv
UI_BIND_ADDRESS=0.0.0.0
UI_PORT=18081
HARNESS_UPSTREAM=http://host.docker.internal:18080
```

Set `UI_BIND_ADDRESS` to the host Tailscale IP to select one interface. The value `0.0.0.0` selects the host interfaces. Authentication is necessary for the API.

The proxy sends requests to `HARNESS_UPSTREAM` without `/api/`. It keeps the authentication header and WebSocket query. The nginx access log does not record API URLs because a WebSocket URL can contain a token.

```sh
# Stop the UI container only.
docker compose down
```

## Phone Access

Open the Docker URL on a phone with Tailscale access to the host. For the local server, open `http://HOST_LAN_IP:5174`. Keep `/api` as the API URL in the browser.

The navigation drawer contains the workspaces and sessions. The message composer stays at the bottom of the screen. The `Enter` key sends a message. The keys `Shift+Enter`, `Command+Enter`, and `Ctrl+Enter` make a new line. Text composition does not send a message.

## A Conversation

The message composer starts with one text line. More text increases the composer height automatically. The maximum height is 192 pixels. More text uses a scrollbar. An empty message draft uses the minimum height again.

On a large screen, use `Collapse navigation` or `Expand navigation` in the header. The browser keeps the setting. On a small screen, use the navigation drawer.

On a small screen, the header shows the workspace name and model. The navigation drawer contains `Tasks`, `Memory`, `Session activity`, and `Configuration`. The `New session` action is above the session list. A panel selection closes the navigation drawer and opens the selected panel.

The sidebar shows the session list for the selected workspace. The `Switch workspace` button opens a `DropdownMenu` with workspaces and a `New workspace` action. The sidebar keeps the session list. A navigation drawer stays open until session selection.

1. Connect to the API.
2. Make a workspace with a server directory and a default model.
3. Make a session.
4. Send a message.
5. Read the response and tool results.

The directory path is on the harness server, not on the browser device. The directory must be available before a workspace starts. The workspace request does not make a directory. Use a full path, such as `/home/user/projects/project`, on the harness server.

The workspace form starts with an empty directory field. The form does not expand `~`. A path with `~` shows an error before the request. For a container server, use the directory path that is available in the container.

Open `Configuration → Settings → Providers` to connect a provider. The API contracts are in `../routes.md`.

Open the workspace menu. Use `Unlink workspace` to remove the workspace reference. The dialog shows the path. Files and directories stay the same. The harness keeps session data, memory records, and usage records. Active work can prevent the unlink operation.

After the unlink operation, the UI can select a different workspace. If a different workspace is not available, the UI shows an empty workspace list. The URL changes with the selection. A new request for the same directory makes a new workspace reference.

## Navigation URLs

The browser URL keeps the selected workspace and session. Refresh the browser to open the same conversation. Use `Back` or `Forward` for the previous or next selection. The optional `agent` parameter opens a child agent in the parent conversation.

```text
/?workspace=INSTANCE_ID&session=SESSION_ID
```

The URL contains IDs, not credentials or message text. An API connection is necessary before a new browser tab opens the selected conversation.

## Child Agents

The sidebar shows child agents below the parent session. Lines show the parent and child relation. Select an agent to see the agent output in the parent conversation. The parent message draft does not change.

An open agent panel has a height of 320 pixels. The panel scrollbar is different from the conversation scrollbar. The panel shows model text, `Thinking`, tool calls, and the last result.

Select the button with the label `Task` to read the task text. A child agent can have more child agents. The panel can show child output.

Select the button with the label `Back to parent agent` to see the parent agent.

The selected conversation receives agent events. An active agent has a status dot with animation. An agent with the `Inactive` label has a constant status dot. The UI shows permission requests. The labels `Reconnecting`, `Cancelled`, and `Failed` identify other agent status data.

Select a conversation to get agent status data.

New output moves the panel to the last line. Use the scrollbar to stop automatic movement. Use `Scroll to latest agent output` to start automatic movement again. The preview shows the last 80 output messages. The live text limit is 100,000 text characters. Message history replaces live text after a model response.

## File Attachments

The `Upload attachments` button opens a `DropdownMenu`. The list shows `Images`, `Documents`, `Video`, and `Audio`. You cannot select a media type with the label `Not supported`. A selection opens a file list for the related file formats.

The selected model gives the input media types. The UI uses input capabilities from the API. Output media types do not change the input media types. The `Documents` UI control accepts text files when the model accepts text input.

Models with text input accept UTF-8 text files as text content. The model and provider must accept the file format.

```text
Image: .png .jpg .jpeg .webp .gif
Audio: .wav .mp3
Video: .mp4 .mpeg .mpg .mov .webm
File:  .pdf and document formats
```

Audio input uses the Chat Completions protocol. Video input uses a compatible provider. Provider limits apply to document formats.

A message can contain 8 file attachments and 10 MiB of file data. A text file can contain 512 KiB. The full JSON request limit is 16 MiB with base64 data. A rejected file does not remove the message draft. The UI examines input media types before it sends the message.

The composer accepts a file selection, a file drop, or clipboard files. A message can contain only file attachments. History and queue receipts show file attachments. Images have a preview. Audio and video have browser UI controls. The `Download` UI control gives the file data.

## Provider Connections

Open the settings dialog from the header, sidebar, or phone navigation drawer. Use the button with the label `Providers`. A provider has a name and connection status. Use the `Configure` button to open the connection form.

OpenAI and DeepSeek have API key forms. Supply the key in the field. Select the button with the label `Save API key`. The UI puts the key in the harness database and gets the model list.

A refresh error does not remove the key from the database.

For a ChatGPT subscription, use the `Configure` button for `OpenAI coding plan`. Use the button with the label `Sign in with ChatGPT`. Open the OpenAI URL and supply the user code. Device-code login in the ChatGPT account settings must be active. The user must supply the code in 15 minutes. Keep the provider section open to see the result.

Model IDs use a provider prefix:

```text
OpenAI API key:        openai/MODEL
DeepSeek API key:      deepseek/MODEL
ChatGPT subscription: openai-codex/MODEL
```

Use the `Models` button to get and show model IDs from the API. Use the button again to close the list. The list has the `Refresh models` button. A model list error shows in the list. A list that is closed does not use space in the provider section.

The workspace form shows the models from providers with a connection. The session form shows the model list from the instance API. Model lists have provider sections in name sequence. Use the text field to find a model or provider. The forms show the model label without the model ID. Requests use the full model ID.

DeepSeek uses the API ID `deepseek-flash` for `DeepSeek-V4.1-Flash`. The other model is `deepseek-v4-pro`, with the label `DeepSeek-V4-Pro-0813`. See the [DeepSeek model data](https://api-docs.deepseek.com/quick_start/pricing). The test model is for connection checks.

A ChatGPT subscription does not include OpenAI API key usage. After you connect to a provider, make a workspace and select a model from the provider.

Provider keys and OAuth tokens stay in the harness database. The UI does not keep provider keys in browser storage. The key field becomes empty after the API accepts the key. The button with the label `Disconnect` removes the credential.

## Settings and Usage

The message input area shows `Last context`, `Out`, `Cache`, and `Total`. Token values come from the provider for the last response in the selected session. `Last context` adds `Input`, `CacheRead`, `CacheWrite`, and `Output`. Output includes reasoning tokens. Child agents do not add tokens to the last response.

The model context limit comes from the model for the last response. A model change does not replace the limit for a previous response. The percentage is not available without a model context limit. `Cache` is the percentage of input tokens from `CacheRead`. The UI shows `—` when usage data is not available.

The UI changes the values after a provider response. The UI does not count subsequent messages or tool results until the provider gives new usage data. `Total` gives available cost data for the conversation and child agents. The `≈` symbol identifies a cost estimate. Context compaction does not change token or cost statistics.

The `ConfigurationMenu` component gives `Settings` and `Workspace settings`. `Settings` has `General`, `Usage`, `Providers`, and `Connection`. `Workspace settings` has `General`, `Instructions`, `Sidekick`, `Watchers`, and `Usage`. `Model and thinking` opens session settings when a session is selected. The UI keeps the conversation and message draft when the dialog closes. Use `Close settings` or the `Escape` key.

The general section sets the agent depth limit and process limit. The `Settings` section sets harness defaults. The `Workspace settings` section sets workspace values. Supply an integer of 0 or more. Select the button with the label `Save`.

A value of 0 prevents new child agents or processes. An empty field removes the selected setting. The harness then uses the default value.

The usage section shows model calls, input tokens, output tokens, reasoning tokens, cache data, and cost data. Harness settings gives harness usage. Workspace settings gives workspace or session usage. Session usage data includes child agents. Select the button with the label `Refresh usage` to get new usage data.

The usage section uses `Input`. The UI shows `CacheRead` and `CacheWrite` in different fields. The worker usage value is `Input + Output`.

The API gives usage data for the available history. It does not give subscription limits. The UI shows `0.0` cost for a new conversation without model usage. The UI shows `Unavailable` when usage has occurred but price data is not available.

Select a model name in the provider section to read token prices and reasoning efforts. The session form also shows token prices. A cost estimate has the label `Estimated cost`. A subscription model does not use API token prices.

## Instruction Reminders

Open `Configuration → Workspace settings → Instructions`, or use `Instruction reminders` in the Memory panel. The UI controls set the plugin state, interval, reminder pattern, and recovery worker model.

The panel shows `low`, `medium`, `high`, and `deferred` reminders. It also shows added input tokens and worker usage. Automatic reminders do not make worker model calls. A worker model is necessary for the `remember_instructions` tool.

## Sidekick

The `Sidekick` section is in `Workspace settings`. The task panel also has a `Sidekick settings` button. A worker model is necessary before the plugin state can be `ON`.

The panel shows `Notes delivered` and worker usage. Workspace memory and project files are different retrieval sources. The harness gives Sidekick notes to the agent in a model request.

The conversation has disclosure controls for Sidekick notes and source references. Open a Sidekick note to examine the context added to a model request. Child agent previews also show activity entries. The section `Earlier Sidekick context` contains activity entries without a reminder anchor in the view.

## Workspace Memory

The `Memory` panel is on the right side of a large screen. Use the header control to open or close it. Small screens use a navigation drawer. Memory data applies to the workspace, across sessions.

The panel shows memory records, source messages, memory jobs, agent tokens, costs, and context data. Open an agent row for more usage data. Use `Memory settings` to select a worker model or change the workspace plugin state. The UI uses the harness plugin API.

See `docs/memory.md` for usage data and API limits.

## Reasoning

The session form has a control with the label `Thinking`. In the message composer, use `Configuration → Model and thinking` to open the dialog. The dialog has fields with the labels `Model` and `Thinking`. The available reasoning efforts come from the selected model.

To use the model default, select the value with the label `Default`. The database keeps the selection. A change applies to the next model request. The dialog shows the result of a change.

The model control changes the model during the conversation. The same dialog shows a message before a change to a model with a smaller context limit. When the context is too large, the harness removes the initial turn from the request. The database keeps the conversation history. The UI keeps the message draft. Select the button with the label `Switch and compact` to continue.

Model messages can have a section with the label `Thinking`. The section is closed by default. Select the button with the label `Thinking` to read the reasoning text or reasoning summary. The provider supplies the text. The UI can show the text during the model response and after it is in history.

## Message Display

Model messages use Markdown for headings, lists, hyperlinks, tables, and code blocks. Code blocks have syntax highlighting. The buttons with the labels `Copy code` and `Download source` use the full source text. The UI does not run message code. HTML in message text stays as text.

A code block with the format `csv` or `tsv` has a table preview. Use the button with the label `Source` to read the full source text. A file text result from `file_actions` can have a CSV preview. Previous tool results keep the preview. Data cells can contain CSV delimiters and line breaks.

One section contains a tool call and the related tool result. Open the section to read the input and output. The section shows a new tool result when the API gives the data. The UI shows a tool result in a different section if the tool call is not in the history.

Large previews have limits. The source download keeps the full source. The copy button also keeps the full source. See `docs/messages.md` for the limits and libraries.

## Functions

Use the button with the label `Delete session` to remove a conversation. The button is adjacent to the session in the navigation. The dialog gives the session ID. Select the button with the label `Delete session` in the dialog to continue. The operation removes the conversation and the child sessions from the server.

Stop active work and remove messages from the queue before session deletion. Stop the processes of the session. Workspace files and usage statistics stay available. The UI keeps the message draft for a different session. The dialog shows an error from session deletion.

The UI can make workspaces and sessions. The user can select an instance or session from the navigation. The UI can start a stopped workspace. It can send text, stop a turn, remove a message from the queue, and accept or deny a permission request.

The UI shows message history, tool input, tool results, events, token usage, and cost data. A session WebSocket gives view snapshots and live events. The UI reads status from state frames. After a socket error, `GET /sessions/{id}/state` can give the status data. The HTTP request interval is 5 seconds.

The conversation starts with 50 messages. Use the scrollbar to read history pages. The history window has a limit of 150 messages. The byte limit is 8 MiB. The UI can keep one larger message. The message bytes stay the same.

The `Latest` control puts the view at the end of the conversation. The history window then contains 50 messages. The last response usage does not change with the history page.

The `bash` tool uses a pseudo-terminal. The terminal view uses xterm.js. The user can send input. Terminal resize and `Ctrl-C` are available.

When a terminal view closes, the socket stops but the process continues. Process exit closes the socket.

The `Tasks` control opens task state on the right side. On large screens, the control is in the header. On small screens, the control is in the navigation drawer. The panel contains task statuses and the `DOING` task title and description.

A task item with status `done` stays in the list while a task item has status `pending` or `in_progress`. The harness removes content from TODO and `DOING` when the last task item becomes `done` or `cancelled`. Empty task state shows `No active tasks` in the panel.

The UI does not have a file editor or revert control. See `docs/design.md` for the UI boundary and plans.

## UI Compilation

```sh
npm run check
npm run build
npm run preview
```

The output directory is `ui/dist/`. The preview server uses port 4173 and the UI proxy configuration. The Docker image has a server and a proxy. A server for files only does not give the API proxy.

## Test Suite

```sh
npx playwright install chromium
npm run test:e2e
```

The browser tests use API fixtures. The test group includes small screens, large screens, authentication, message input, cancel commands, permissions, settings, and long output. It also includes Markdown, code blocks, CSV data, and tool results. The test server uses port 15173.

To use a browser executable from the computer:

```sh
PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/path/to/chrome npm run test:e2e
```

Project rules are in `AGENTS.md`.

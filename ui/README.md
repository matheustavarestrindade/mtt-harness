# MTT Console

The UI is a client of the harness API. The harness keeps the sessions, queue, tools, permissions, and data. The UI sends HTTP requests and reads WebSocket events.

## Start the UI

The minimum Node.js version is `22.12`. Start the harness with the commands in `../AGENTS.md`. The default harness URL is `http://localhost:18080`.

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

The Docker image and Docker Compose file are in `ui/`. The image contains the UI and an nginx proxy. It uses the harness API on the host computer.

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
UI_DOCKER_SUBNET=172.16.1.0/24
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

1. Connect to the API.
2. Make a workspace with a server directory and a default model. For the test provider, use `test/test-model`.
3. Make a session.
4. Send a message.
5. Read the response and tool results.

With the default Docker configuration, the host directory `./workspace` is at `/workspace` in the harness container. The path is on the harness server, not on the browser device. Put project files in the host directory `./workspace`.

The directory must be available before a workspace starts. The workspace request does not make a directory. For `/workspace/teste`, first make the host directory `./workspace/teste`. A path such as `/teste` refers to a different directory in the container.

The harness includes the test provider by default in Docker. Open `Settings`, then `Providers`, to connect OpenAI or DeepSeek. The API contracts are in `../routes.md`.

## Provider Connections

Open the settings dialog from the header, sidebar, or phone navigation drawer. Use the button with the label `Providers`. OpenAI and DeepSeek have API key forms. Supply the key in the field. Select the button with the label `Save API key`. The UI puts the key in the harness database and gets the model list.

A refresh error does not remove the key from the database.

For a ChatGPT subscription, use the button with the label `Sign in with ChatGPT`. Open the OpenAI URL and supply the user code. Device-code login in the ChatGPT account settings must be active. The user must supply the code in 15 minutes. Keep the provider section open to see the result.

Model IDs use a provider prefix:

```text
OpenAI API key:        openai/MODEL
DeepSeek API key:      deepseek/MODEL
ChatGPT subscription: openai-codex/MODEL
```

The provider section shows model IDs from the API. The list changes when the user refreshes model data. The provider section shows a model list error.

The workspace form shows the models from providers with a connection. The session form shows the model list from the instance API. The forms show the optional model label with the ID. Requests use the ID.

DeepSeek uses the API ID `deepseek-flash` for `DeepSeek-V4.1-Flash`. The other model is `deepseek-v4-pro`, with the label `DeepSeek-V4-Pro-0813`. See the [DeepSeek model data](https://api-docs.deepseek.com/quick_start/pricing). The test model is for connection checks.

A ChatGPT subscription does not include OpenAI API key usage. After you connect to a provider, make a workspace and select a model from the provider.

Provider keys and OAuth tokens stay in the harness database. The UI does not keep provider keys in browser storage. The key field becomes empty after the API accepts the key. The button with the label `Disconnect` removes the credential.

## Settings and Usage

The settings dialog has 4 sections: `General`, `Usage`, `Providers`, and `Connection`. The UI keeps the conversation and message draft when the dialog closes. Use the button with the label `Close`, or use the `Escape` key.

The general section sets the agent depth limit and process limit. Select the harness or workspace scope. Supply an integer of 0 or more. Select the button with the label `Save`.

A value of 0 prevents new child agents or processes. An empty field removes the selected setting. The harness then uses the default value.

The usage section shows model calls, input tokens, output tokens, reasoning tokens, cache data, and cost data. Select the harness, workspace, or session scope. Session usage data includes child agents. Select the button with the label `Refresh usage` to get new usage data.

The API gives usage data for the available history. It does not give subscription limits. The UI shows `0.0` cost for a new conversation without model usage. The UI shows `Unavailable` when usage has occurred but price data is not available.

Select a model name in the provider section to read token prices and reasoning efforts. The session form also shows token prices. A cost estimate has the label `Estimated cost`. A subscription model does not use API token prices.

## Workspace Memory

The `Memory` panel is on the right side of a large screen. Use the header control to open or close it. Small screens use a navigation drawer. Memory data applies to the workspace, across sessions.

The panel shows memory records, source messages, memory jobs, agent tokens, costs, and context data. Open an agent row for more usage data. Use `Memory settings` to select a worker model or change the workspace plugin state. The UI uses the harness plugin API.

See `docs/memory.md` for usage data and API limits.

## Reasoning

The session form has a control with the label `Thinking`. In the message composer, use the button with the label `Session settings` to open the dialog. The dialog has fields with the labels `Model` and `Thinking`. The available reasoning efforts come from the selected model.

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

The UI shows message history, tool input, tool results, events, token usage, and cost data. It reads status and history at an interval of 1.5 seconds. A WebSocket event can also cause a new request. The UI shows model text during the response. Message IDs connect event text to the history.

The progress panel shows task state above the composer. It contains task statuses and the `DOING` task title and description. A task item with status `done` stays in the list while a task item has status `pending` or `in_progress`. The harness removes content from TODO and `DOING` when the last task item becomes `done` or `cancelled`. Empty task state removes the progress panel.

The UI does not have a file editor, media upload, process terminal, or revert control. See `docs/design.md` for the UI boundary and plans.

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

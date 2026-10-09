# UI Architecture

## Boundary

The UI is an API client. The harness controls model calls, tool calls, workspace access, queue state, permissions, and database data. The UI has forms, selected views, and message drafts.

A UI requirement must not change the Go code, API contracts, database schema, or harness configuration without user approval. The UI uses the API contracts in `../../routes.md`. UI packages, configuration, test suite, proxy, and documents stay in `ui/`.

## Files

```text
ui/
  package.json                 # frontend dependencies and commands
  package-lock.json            # dependency versions
  components.json              # shadcn-svelte configuration
  vite.config.ts               # development HTTP/WebSocket proxy
  Dockerfile                   # UI build and nginx image
  compose.yaml                 # UI service and published port
  nginx/default.conf.template  # deployment HTTP/WebSocket proxy
  .env.example                 # proxy targets; no secrets
  src/
    App.svelte                 # entry point only
    app.css                    # design tokens and responsive styles
    lib/
      atoms/
        types.ts               # Go JSON shapes
        format.ts              # pure display helpers
        content.ts             # content URL and file helpers
        transcript.ts          # call/result pairing in history order
        settings.ts            # editable settings and section types
        utils.ts               # shared component types and class utility
        Brand.svelte           # application mark
        ui/                    # shadcn-svelte primitives
      molecules/
        api/client.ts          # authenticated HTTP requests
        api/events.ts          # event connection and cursor
        connection-storage.ts  # tab-scoped preferences
        content/               # Markdown, code, and CSV components
        *.svelte               # forms, messages, composer, usage
      organisms/
        console.svelte.ts      # rune-based client state
        Navigation.svelte      # workspace and session navigation
        Conversation.svelte    # conversation workflow
        ProvidersPanel.svelte  # provider keys and device sign-in workflow
        SettingsDialog.svelte  # settings modal and section selection
        RuntimeSettingsPanel.svelte # harness and workspace limits
        UsagePanel.svelte      # recorded usage with scope selection
        Workbench.svelte       # layout and user actions
  tests/                       # browser tests and API fixtures
  docs/                        # UI documents
```

The client uses Svelte 5 runes, TypeScript, Tailwind CSS, shadcn-svelte, Lucide, and Sonner. The icon package is `lucide-svelte`. The shadcn-svelte components also use `@lucide/svelte`.

## Components

The UI uses the `neutral` color configuration from shadcn-svelte. Components use the same color values. User approval is necessary before a color change.

The dependency direction is `atoms <- molecules <- organisms`. An atom import must not refer to a molecule or organism. A molecule import must not refer to an organism. The `check:layers` command examines TypeScript and Svelte imports. The shadcn-svelte configuration puts new primitives in `atoms/ui`.

The shadcn-svelte registry gives the buttons, fields, dialogs, sheet, badges, and notifications. The console components add navigation, connection forms, message history, usage, and the composer.

The `SelectField` molecule uses the shadcn-svelte `Select` components. Model lists and settings use the molecule. Long model text uses line breaks. A long model list uses a scrollbar.

The model ID and reasoning effort values do not change. The model default uses an empty reasoning effort. A model change stays at the previous selection until the API accepts the change.

Use the header control to close the sidebar on large screens. The header control can also open the sidebar. A navigation drawer replaces the sidebar on small screens. The UI controls have labels and keyboard focus. The composer uses the viewport height and safe-area padding. Long output must not increase the document width.

The composer starts with one text line. The composer height increases with the message draft, to a maximum of 192 pixels. More text uses a scrollbar. An empty message draft uses the minimum height again. A width change also adjusts the height for line breaks.

The navigation control stays in the header when the sidebar closes. The browser keeps the setting in local storage. The setting does not contain credentials or conversation text.

The sidebar shows the session list for the selected workspace. The header shows the workspace name. The `WorkspaceSwitcher` molecule uses the shadcn-svelte `DropdownMenu` component. The component shows workspaces and the `New workspace` action.

Workspace selection changes the session list. On a small screen, the navigation drawer stays open until session selection. A session has one text line in the list.

The `Enter` key sends the message. The keys `Shift+Enter`, `Command+Enter`, and `Ctrl+Enter` add a new line. Text composition does not send a message.

The `AttachmentDrafts` organism keeps file attachments for a session. The `Composer` molecule receives files from the browser. The `AttachmentTray` molecule shows attachment drafts and error messages. The `MediaContent` molecule shows content in history and queue receipts.

The selected model gives the input media types. The UI examines input media types before it sends a message. If the API rejects the message, the UI keeps the message draft. After the API accepts a message, the UI removes the file attachments from the message draft. The UI removes object URLs when files are removed.

## Task State

The `Tasks` control in the header opens the `TaskPanel` organism on the right side. Small screens use a navigation drawer.

The `TaskProgress` molecule shows session task state in the panel. The content has a task title, description, and task items with task status. A button opens or closes the content. The panel has a scrollbar.

The console reads `GET /sessions/{id}/task-state` and receives `task_state.updated` events. It accepts data only for the selected session and the same or newer revision. An empty TODO list with `DOING: null` removes the progress panel. An error on the task state route does not remove the session or message draft.

## Memory Panel

The `MemoryPanel` organism shows workspace memory data on the right side. A screen width below 1280 pixels uses a navigation drawer. The header control opens or closes the panel. The panel has a scrollbar.

The `memory-panel.svelte.ts` file controls data requests and configuration requests. The API client uses harness plugin routes. A workspace change or connection change cancels previous requests. Previous requests also stop when the panel closes. A previous response must not change the new selected view.

The `memory.ts` atom contains API types. The atom calculates usage data. The `MemoryAgentUsage` molecule shows agents and models.

The UI does not change memory content or model context. See `memory.md` for usage data and configuration behavior.

## Spaced Repetition

The `Instructions` section in the settings dialog uses the plugin name `spaced_repetition`. A button in the Memory panel opens the section. The UI shows workspace settings for the interval, reminder pattern, worker model, and reasoning effort.

The `WorkspacePluginPanelState` organism controls plugin requests. A response from a previous workspace must not change the selected view. Memory and spaced repetition use the same organism.

The UI does not add the cost of reminder tokens to worker cost. Mobile settings navigation uses 2 rows. The UI controls stay in the viewport.

## Sidekick

The `SidekickPanel` organism uses the plugin name `sidekick`. The settings dialog and task panel give access to the workspace configuration. The UI can select the worker model, reasoning effort, and retrieval sources. The UI also gives time and byte limits.

The panel uses `WorkspacePluginPanelState` for data requests and configuration requests. Workspace usage includes worker model calls. The input for a Sidekick note is part of session usage. The harness calculates the text for a Sidekick note.

## API Data

Response keys agree with the Go types. For example, an instance has `Workspace` and `DefaultModel`. Request keys use the handler format, such as `workspace` and `default_model`. A `null` collection becomes an empty array in the client.

A `202` response shows that the queue accepted a message. It is not a model response. The UI keeps a message receipt until history or queue status gives the result. A queue message from a different client can have an ID without text in the UI.

A session change cancels previous data requests and event connections. A previous response must not change the new selected view. The UI does not automatically send a message again after a request error. The API can accept the message before the client receives the error.

The UI sends `DELETE /sessions/{id}` after the user accepts the dialog. The API response gives the IDs of the sessions that the operation removed. The client uses the IDs to remove sessions from the navigation and stop event connections. A previous response must not put the sessions back into the navigation.

The message draft and view of a different session do not change. The dialog shows an error from session deletion.

A new session view reads events from sequence 0. Permission requests and decisions come from the event stream. A new connection after an error uses the last event sequence. Message history from the API is the source for completed responses.

The client uses `message_id` to connect model event text to message history. Reasoning text has a section that is closed by default. The session reasoning effort control uses the values from model data. A previous request must not replace a new selection.

The UI shows model text with Markdown markup. The DOMPurify library removes dangerous markup before display. HTML in message text stays as text. The code block gives the format for syntax highlighting. A media item has a type and filename in the UI. The UI does not run code from model output.

The `transcript.ts` atom uses `ToolCallID` and `ToolCalls[].ID` to connect messages in the same session. The UI keeps the model message sequence. The UI shows a tool result in a different section if the tool call is not in the history. The tool component prepares input and output when the user opens the section.

Tool sections, reasoning sections, and model text use the UI background. The sections have a line between them. A different color identifies a tool section. When a tool section opens, one panel contains input and output. The panel has a background color and border. Input and output have a line between them.

Content views in the panel do not have a card container.

See `messages.md` for the libraries, content limits, and source buttons.

## Settings

Settings use one dialog above the conversation. Large screens have a sidebar for section selection. Small screens have a horizontal section list. The content section has a scrollbar. The user can close the dialog with the `Escape` key. The dialog keeps keyboard focus while it is open.

The dialog has connection, provider, usage, and general settings sections. The usage section sends requests to the API to get statistics. The general section sends configuration requests to the API. An empty value removes the selected setting.

Section requests stop when the dialog closes. They also stop when the user selects a different section. Previous requests must not change the selected scope. The UI keeps the conversation and message draft.

## Provider Connections

The provider section uses the default OpenAI and DeepSeek configuration. API key forms are molecules. The provider organism controls key requests, model lists, and device login status.

Model data can include token prices and reasoning efforts. The UI shows model data from the API. The UI does not get a model catalog from a different program. A cost estimate has a label. Subscription access does not get an API token price estimate.

The provider `openai-codex` uses OpenAI device authentication. OAuth tokens stay on the server. The UI gets a user code and an OpenAI URL. When the provider section closes, the browser stops status requests. Server authentication can continue for 15 minutes or until the user cancels it.

## API Limits

- The API does not have a route to read open permission requests. Session events give the permission requests and decisions.
- Queue status gives message IDs only. The UI can show text from the local message receipt or message history.
- The API does not have a route to set a session title. The UI shows a short session ID and the model ID.
- The message route does not have pagination. The UI reads the full history.
- The API does not have an HTTP route to stop a process. The UI does not add a route to the harness.
- The API does not have a CORS handler. The UI uses a same-origin proxy.

## Plans

The next UI version can add media upload, a revert control, and process output. Use the API contracts for a new function. User approval is necessary before a change to the harness.

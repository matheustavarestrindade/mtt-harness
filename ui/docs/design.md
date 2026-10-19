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
        session-tree.ts        # parent and child relations
        agent-activity.ts      # event values and agent tool data
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
        AgentPanel.svelte      # fixed-height child output
        agent-monitor.svelte.ts # child relations and event ownership
        agent-feed.svelte.ts   # child history and live output
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

The workspace menu has an `Unlink workspace` action. The `UnlinkWorkspaceDialog` molecule shows the workspace path. The `HarnessConsole` organism removes the workspace reference after the API accepts the operation. A previous API response cannot put an unlinked workspace back in the list. Message drafts for different workspaces stay the same. The URL changes with the workspace selection.

The UI uses the `neutral` color configuration from shadcn-svelte. Components use the same color values. User approval is necessary before a color change.

The dependency direction is `atoms <- molecules <- organisms`. An atom import must not refer to a molecule or organism. A molecule import must not refer to an organism. The `check:layers` command examines TypeScript and Svelte imports. The shadcn-svelte configuration puts new primitives in `atoms/ui`.

The shadcn-svelte registry gives the buttons, fields, dialogs, sheet, badges, and notifications. The console components add navigation, connection forms, message history, usage, and the composer.

The `ModelCombobox` molecule uses the shadcn-svelte `Command` and `Popover` components. Model lists have provider sections in name sequence. A text field finds models or providers. A model has a display label without the model ID. The API receives the full model ID.

The `SelectField` molecule uses the shadcn-svelte `Select` components for reasoning effort and other settings. Long model text uses line breaks. A long model list uses a scrollbar. The model control keeps keyboard focus after a selection or API error. The `Escape` key closes the model list before the dialog.

The model ID and reasoning effort values do not change. The model default uses an empty reasoning effort. A model change stays at the previous selection until the API accepts the change.

Use the header control to close the sidebar on large screens. The header control can also open the sidebar. A navigation drawer replaces the sidebar on small screens. The UI controls have labels and keyboard focus. The composer uses the viewport height and safe-area padding. Long output must not increase the document width.

The composer starts with one text line. The composer height increases with the message draft, to a maximum of 192 pixels. More text uses a scrollbar. An empty message draft uses the minimum height again. A width change also adjusts the height for line breaks.

The navigation control stays in the header when the sidebar closes. The browser keeps the setting in local storage. The setting does not contain credentials or conversation text.

The sidebar shows the session list for the selected workspace. The header shows the workspace name. The `WorkspaceSwitcher` molecule uses the shadcn-svelte `DropdownMenu` component. The component shows workspaces and the `New workspace` action.

Workspace selection changes the session list. On a small screen, the navigation drawer stays open until session selection. A session has one text line in the list.

The `Enter` key sends the message. The keys `Shift+Enter`, `Command+Enter`, and `Ctrl+Enter` add a new line. Text composition does not send a message.

The `AttachmentDrafts` organism keeps file attachments for a session. The `Composer` molecule receives files from the browser. The `AttachmentTray` molecule shows attachment drafts and error messages. The `MediaContent` molecule shows content in history and queue receipts.

The selected model gives the input media types. The UI examines input media types before it sends a message. If the API rejects the message, the UI keeps the message draft. After the API accepts a message, the UI removes the file attachments from the message draft. The UI removes object URLs when files are removed.

## URL Navigation

The URL uses the query parameters `workspace`, `session`, and `agent`. The workspace value is an instance ID. The session value is a parent session ID. The optional agent value is a child session ID.

```text
/?workspace=INSTANCE_ID&session=SESSION_ID&agent=AGENT_ID
```

Refresh the browser to open the same workspace and conversation. Use `Back` or `Forward` for the previous or next selection. The client uses API requests for the selection. The selection parameters contain IDs only.

The client keeps other query parameters and the URL fragment. A previous request must not replace a new selection. The client changes the URL after session deletion. If a workspace or session is not available, the client shows an error. It does not make a workspace or session.

Route data is in the atom `navigation-route.ts`. Browser history is in the molecule `browser-navigation.ts`. The `HarnessConsole` organism controls selection, API requests, and cancellation.

## Child Agents

The client uses `Session.Parent` for agent navigation. The sidebar shows child agents below the parent session. Agent selection opens the parent conversation and the selected agent panel. A child of an agent opens in the same panel.

The `AgentMonitor` organism controls agent data for the selected conversation. The `AgentFeed` organism controls one event connection, data requests, and the live preview. A session change or workspace change stops previous data requests. Previous responses cannot change the new view. A completed agent closes the event connection.

The session data gives `ParentMessageID` and `ParentToolCallID` for new agents. The API uses the sequence of events for previous agent origins.

Task text does not identify a tool call. Parallel tool calls can have the same task text. The UI shows an agent without a message ID after the conversation history.

Child agents run directly in the loop. The view snapshot gives active session IDs, permissions, and the last turn status. The value `Completed: false` does not give active status.

The panel uses `MessageCard`, `ThinkingSection`, and the content components. The organism supplies the agent section through `renderTool`. The `finish` result has a different response section.

An open panel has a height of 320 pixels. Output uses the panel scrollbar. The display interval is 100 ms. An open panel reads history pages.

When an agent panel closes, an active agent uses `details=false`. The UI removes the body window when the panel closes. The same history limits apply to the parent conversation and agent previews.

## Task State

The `Tasks` control opens the `TaskPanel` organism on the right side. On large screens, the control is in the header. On small screens, the control is in the navigation drawer.

The `TaskProgress` molecule shows session task state in the panel. The content has a task title, description, and task items with task status. A button opens or closes the content. The panel has a scrollbar.

The view snapshot contains task state. The console also receives `task_state.updated` events. It accepts data only for the selected session and the same or newer revision. An empty TODO list with `DOING: null` removes the progress panel. An error for task state does not remove the session or message draft.

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

The `SessionActivity` organism reads Sidekick notes after the request boundary. The `SidekickSection` molecule has disclosure controls for Sidekick notes and source references. A tool result can be a reminder anchor. The UI puts the activity entry after the tool section. The history section contains other activity entries. Child agent previews use the same components.

The client gets activity data after model requests, session events, and new WebSocket connections. Output chunks do not start the API requests. A session change cancels previous requests. The history revision identifies activity entries from previous history. The client keeps a maximum of 1000 activity entries. Activity data does not become a conversation message.

## Watchers

The editor has `General`, `When to run`, `Review input`, `Tool access`, and `Execution limits` sections. `Run on` uses `Select` with `type="multiple"`. `DropdownMenu` contains file operations. The `enabled` value uses a `Switch`.

The `Conversation to review` field gives the token limit for `{last_context}`. The `Tooltip` gives information about context variables. The `interval_tokens` value is used for `context_growth` events.

The Settings pages use `SettingsPageTitle` and `Tabs`. The watcher panels also use `Tabs`. The panels have the same left edge. `Instructions` and Watchers use `PluginPanelFooter`. The refresh action is at the bottom. The button text is the same as the text in the Sidekick view.

See `ui/docs/component-audit.md` for the control audit.

`Settings → Watchers` gives workspace watcher definitions. The editor uses `Schema.JSON` for the tool catalog and default watcher definition. A model is necessary. The user can select file operations for `file_actions`. Tool grants do not use the permission pipeline.

Use `New watcher`, `Edit`, `Duplicate`, or `Remove`. An input preview uses the selected primary session and does not start a model. Watcher usage is workspace usage. The `CacheRead` and `CacheWrite` values are different from `Input`.

The `SessionActivity` API and `plugin.activity` events give watcher run records. The view gives `Decision`, `Outcome`, and `InputPreview`. The API does not give `Message.Reasoning` from worker sessions.

The UI uses the watcher run record for a runtime continuation when the `run_ids` values agree. Responses from a previous workspace or session must not replace new data.

## API Data

Response keys agree with the Go types. For example, an instance has `Workspace` and `DefaultModel`. Request keys use the handler format, such as `workspace` and `default_model`. A `null` collection becomes an empty array in the client.

A `202` response shows that the queue accepted a message. It is not a model response. The UI keeps a message receipt until history or queue status gives the result. The view snapshot gives queue previews after a page refresh or a new connection.

The `QueuedMessage` molecule shows `Queued` and an `X` control on the message. The UI does not show a queue panel or a `Message accepted` label. A cancellation error keeps the message. A previous state frame must not put the message back after cancellation.

The queue preview keeps the user role and user message bar. The UI shows `Preview limited` if content is not in the queue preview. The `Show full message` control reads the full content from the queue route. The client compares the session ID and queue IDs before it accepts the result.

The UI accepts the `GET` response only while the ID is in the queue. A previous response must not put a message back after cancellation. The UI does not keep message content in browser storage.

A session change cancels previous data requests and event connections. A previous response must not change the new selected view. The UI does not automatically send a message again after a request error. The API can accept the message before the client receives the error.

The UI sends `DELETE /sessions/{id}` after the user accepts the dialog. The API response gives the IDs of the sessions that the operation removed. The client uses the IDs to remove sessions from the navigation and stop event connections. A previous response must not put the sessions back into the navigation.

The message draft and view of a different session do not change. The dialog shows an error from session deletion.

A new session view uses `/events?view=true`. The connection starts with a state frame. The connection then gives events and new state frames. The connection does not send the full output history.

The UI reads status from state frames. After a socket error, `GET /sessions/{id}/state` can give the status data. The HTTP request interval is 5 seconds. A new state frame stops the HTTP timer.

The `SessionHistory` organism starts with the last 50 messages. The history window has a limit of 150 messages. The byte limit is 8 MiB. The UI can keep one larger message. The message bytes stay the same.

The `HistoryScroll` organism gets history pages. A DOM anchor keeps the reading position. The `Latest` control puts the view at the end of the conversation. The history window then contains 50 messages.

If the user moves up the conversation, the UI stops automatic movement to the end. A small distance has the same effect. New output and state frames must not move the view down. Automatic movement can start again when the user moves to the end or uses `Latest`.

The UI removes previous views with `URL.revokeObjectURL`, `Terminal.dispose`, and `WebSocket.close`.

The client keeps message data when message content does not change. Source bytes, MIME type, and URL control Blob URLs. A file attachment keeps the source URL when the content does not change. A content change releases the previous Blob URL.

The client uses `message_id` to connect model event text to message history. Reasoning text has a section that is closed by default. The session reasoning effort control uses the values from model data. A previous request must not replace a new selection.

The UI shows model text with Markdown markup. The DOMPurify library removes dangerous markup before display. HTML in message text stays as text. The code block gives the format for syntax highlighting. A media item has a type and filename in the UI. The UI does not run code from model output.

The `transcript.ts` atom uses `ToolCallID` and `ToolCalls[].ID` to connect messages in the same session. The UI keeps the model message sequence. The UI shows a tool result in a different section if the tool call is not in the history. The tool component prepares input and output when the user opens the section.

Tool sections, reasoning sections, and model text use the UI background. The sections have a line between them. A different color identifies a tool section. When a tool section opens, one panel contains input and output. The panel has a background color and border. Input and output have a line between them.

Content views in the panel do not have a card container.

See `messages.md` for the libraries, content limits, and source buttons.

## Settings

The settings pages use `SettingsPageTitle`. The settings pages have the same page title dimensions. `SettingsDialog` gives the same left edge to the settings pages.

The `Tabs` component controls the settings pages and Watcher sections. The tab highlight stays in the tab track. The UI shows the selected settings page.

See `ui/docs/component-audit.md` for the control audit.

The `UsageBar` molecule uses `LatestResponse.Usage` from the history metadata. It does not use token values from `Statistics` or from the history page. `Last context` is the sum of `Input`, `CacheRead`, `CacheWrite`, and `Output`. Output includes reasoning tokens. `Out` and `Cache` use the same request.

The `LatestResponse.Model` field identifies the model for the response. The model supplies the context limit. The model for a previous response can be different from the selected model. The UI shows `—` when usage data is not available.

The client removes previous model IDs on session selection. The conversation cost includes child agents. The label `Total` identifies the scope. The settings dialog gives usage statistics. Context compaction does not remove previous usage from the statistics.

Settings use one dialog above the conversation. Large screens have a sidebar for section selection. Small screens have a horizontal section list. The content section has a scrollbar. The user can close the dialog with the `Escape` key. The dialog keeps keyboard focus while it is open.

The dialog has connection, provider, usage, and general settings sections. The usage section sends requests to the API to get statistics. The general section sends configuration requests to the API. An empty value removes the selected setting.

Section requests stop when the dialog closes. They also stop when the user selects a different section. Previous requests must not change the selected scope. The UI keeps the conversation and message draft.

## Provider Connections

The provider section uses the default OpenAI and DeepSeek configuration. API key forms are molecules. The provider organism controls key requests, model lists, and device login status.

The `ProviderCard` molecule shows the provider name, connection status, and buttons. The provider section has one column on small screens. A large screen has 2 columns. The `Configure` button opens the connection form. The `Models` button opens a model list. The sections are closed by default.

The UI gets a model list when the user opens the list. Model list requests stop when the provider section closes. The UI keeps an active device login when the user closes a connection form.

Model data can include token prices and reasoning efforts. The UI shows model data from the API. The UI does not get a model catalog from a different program. A cost estimate has a label. Subscription access does not get an API token price estimate.

The provider `openai-codex` uses OpenAI device authentication. OAuth tokens stay on the server. The UI gets a user code and an OpenAI URL. When the provider section closes, the browser stops status requests. Server authentication can continue for 15 minutes or until the user cancels it.

## API Limits

- The session view snapshot contains open permission requests. Session events give new requests and decisions.
- Queue status gives message IDs only. The UI can show text from the local message receipt or message history.
- The API does not have a route to set a session title. The UI shows a short session ID and the model ID.
- The history route gives history pages. The history revision changes after a `revert` operation.
- Process socket input and terminal resize are available.
- The API does not have a CORS handler. The UI uses a same-origin proxy.

## Plans

The next UI version can add a file editor and a revert control. Use the API contracts for a new function. User approval is necessary before a change to the harness.

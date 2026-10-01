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
        utils.ts               # shared component types and class utility
        Brand.svelte           # application mark
        ui/                    # shadcn-svelte primitives
      molecules/
        api/client.ts          # authenticated HTTP requests
        api/events.ts          # event connection and cursor
        connection-storage.ts  # tab-scoped preferences
        *.svelte               # forms, messages, composer, usage
      organisms/
        console.svelte.ts      # rune-based client state
        Navigation.svelte      # workspace and session navigation
        Conversation.svelte    # conversation workflow
        ProvidersPanel.svelte  # provider keys and device sign-in workflow
        Workbench.svelte       # layout and user actions
  tests/                       # browser tests and API fixtures
  docs/                        # UI documents
```

The client uses Svelte 5 runes, TypeScript, Tailwind CSS, shadcn-svelte, Lucide, and Sonner. The icon package is `lucide-svelte`. The shadcn-svelte components also use `@lucide/svelte`.

## Components

The UI uses the `neutral` color configuration from shadcn-svelte. Components use the same color values. User approval is necessary before a color change.

The dependency direction is `atoms <- molecules <- organisms`. An atom import must not refer to a molecule or organism. A molecule import must not refer to an organism. The `check:layers` command examines TypeScript and Svelte imports. The shadcn-svelte configuration puts new primitives in `atoms/ui`.

The shadcn-svelte registry gives the buttons, fields, dialogs, sheet, badges, and notifications. The console components add navigation, connection forms, message history, usage, and the composer.

The sidebar stays open on large screens. A navigation drawer replaces the sidebar on small screens. The UI controls have labels and keyboard focus. The composer uses the viewport height and safe-area padding. Long output must not increase the document width.

## API Data

Response keys agree with the Go types. For example, an instance has `Workspace` and `DefaultModel`. Request keys use the handler format, such as `workspace` and `default_model`. A `null` collection becomes an empty array in the client.

A `202` response shows that the queue accepted a message. It is not a model response. The UI keeps a message receipt until history or queue status gives the result. A queue message from a different client can have an ID without text in the UI.

A session change cancels previous data requests and event connections. A previous response must not change the new selected view. The UI does not automatically send a message again after a request error. The API can accept the message before the client receives the error.

A new session view reads events from sequence 0. Permission requests and decisions come from the event stream. A new connection after an error uses the last event sequence. Message history from the API is the source for completed responses.

The UI shows model and tool text without HTML content. A media item has a type and filename in the UI. The UI does not run code from model output.

## Provider Connections

The provider screen uses the default OpenAI and DeepSeek configuration. API key forms are molecules. The provider organism controls key requests, model lists, and device login status.

The provider `openai-codex` uses OpenAI device authentication. OAuth tokens stay on the server. The UI gets a user code and an OpenAI URL. When the provider screen closes, the browser stops status requests. Server authentication can continue for 15 minutes or until the user cancels it.

## API Limits

- The API does not have a route to read open permission requests. Session events give the permission requests and decisions.
- Queue status gives message IDs only. The UI can show text from the local message receipt or message history.
- The API does not have a route to set a session title. The UI shows a short session ID and the model ID.
- The message route does not have pagination. The UI reads the full history.
- The API does not have an HTTP route to stop a process. The UI does not add a route to the harness.
- The API does not have a CORS handler. The UI uses a same-origin proxy.

## Plans

The next UI version can add Markdown, media upload, settings, a revert control, and process output. Use the API contracts for a new function. User approval is necessary before a change to the harness.

# Message Display

## Libraries

The UI uses the libraries in the list:

- [Marked](https://marked.js.org/) reads Markdown text.
- [DOMPurify](https://github.com/cure53/DOMPurify) removes dangerous markup.
- [Highlight.js](https://highlightjs.org/) gives syntax highlighting.
- [Papa Parse](https://www.papaparse.com/docs) reads CSV data.
- [xterm.js](https://xtermjs.org/) shows terminal output with ANSI sequences.
- [ANSIUp](https://github.com/drudru/ansi_up) shows ANSI text color in data fields.

The dependency versions are in `ui/package-lock.json`. The code is in `ui/src/lib/molecules/content/`.

## Reasoning

The field `Message.Reasoning` contains reasoning text or a reasoning summary from the provider. The section with the label `Thinking` is closed by default. Select the button with the label `Thinking` to read the text. Content views are prepared when the section opens.

The client receives text from `model.chunk` events during the response. The field `message_id` connects event text to message history. A message has one display area. An open reasoning section stays open after the message is in history. The section is closed when the UI starts.

The client prepares event text at an interval of 100 milliseconds. A session change removes previous event text. The API does not send continuation data in messages or events.

During reasoning, the `Thinking` label includes a short text preview. The preview uses text from the provider. It selects the last heading or available text. The full reasoning text stays available in the section.

A text preview uses a maximum of 120 characters. The UI reads a maximum of 8192 characters for a preview.

The UI uses `ActivityIndicator` for work in progress. The icon moves during a response. The sidebar shows the icon before the selected session label. If navigation is closed, the header shows the icon.

The conversation does not have a row for the icon. A permission request removes the icon. The `prefers-reduced-motion` setting stops the movement.

## Markdown and Code

The UI removes an internal `context_message` label from the start of text in an `assistant` message. This also applies while the model sends the label in stream chunks. The message copy button uses the text shown to the user. User text and tool output do not change. Text in code blocks or between quotation marks stays the same. The UI does not change message history.

Markdown has headings, lists, hyperlinks, tables, and code blocks. HTML in message text stays as text. The UI uses HTTP and HTTPS hyperlinks. It can also use hyperlinks with the format `mailto`. The UI removes event handlers and executable markup. Code does not run.

The UI gets the syntax highlighting library when a code block opens. The code block supplies the code format. The UI does not find the format automatically. The UI shows text if syntax highlighting is not available for a format.

The copy button uses the full source text. The source download also contains the full source text. The clipboard is available through HTTP if the browser permission is active.

## CSV Data

A code block with the format `csv` or `tsv` has a table view and a source view. A CSV text file can also have a preview. A file text result from `file_actions` uses the file extension to select the content format. The UI shows text for more output or error data. Previous tool results keep the file view.

CSV data cells can contain CSV delimiters, quotation marks, and line breaks. Data cell values stay as text. The header field sets the column names. The UI shows a format error with the source text.

## Limits

- A Markdown preview uses a maximum of 100,000 characters. The full message source stays available.
- A source preview uses a maximum of 60,000 characters. Use the button with the label `Show full source` to read the full text.
- Syntax highlighting uses a maximum of 30,000 characters. Larger output stays as text.
- A CSV preview uses a maximum of 200 table rows and 50 columns. The text limit is 1,000,000 characters.
- A file preview reads a base64 data field with a maximum of 2,000,000 characters.

The source download keeps the full text given to the component. The copy button also keeps the full text. A Markdown preview limit does not change the message copy button.

## Tool Sections

One tool section contains the tool call and tool result. The `transcript.ts` atom uses session IDs and tool call IDs to connect them. The UI can receive tool results in a different sequence. The tool call sequence stays the same.

For `file_actions`, the section label shows the action sequence and path. The input has the full action data. One tool call has one result section, with the selected action output.

Tool input uses field labels and lists. The UI uses tables for tool result data. The file action sections keep the action sequence. The fields show input parameters and file text. JSON text is not the default view.

A data preview has a limit of 400 fields. The preview depth limit is 8. A text field uses a maximum of 32000 characters. The copy button and source download keep the input data. A JSON result above 1000000 characters uses the text view.

JSON result fields and tables can contain ANSI sequences. ANSIUp gives text color and background color to the preview. DOMPurify removes executable markup. Tool input stays the same.

The client prepares content views when the user opens a tool section. The section shows input, output, and the value of `ToolCallID`. The same section shows a new tool result. The UI shows a tool result in a different section if the tool call is not in the history.

Tool sections, reasoning sections, and model text use the UI background. The sections have a line between them. A different color identifies a tool section. When a tool section opens, one panel contains input and output. The panel has a background color and border. Input and output have a line between them.

Content views in the panel do not have a card container. Copy buttons, source downloads, and CSV views stay available.

Message history does not include tool status fields. The UI shows the label `Result` when a tool result is available. It does not change error text in the output.

## Terminal Output

A `bash` section can show new output before the tool result is available. The fields `ProcessSpec.ToolCallID` and `ProcessSpec.ToolMessageID` connect the process to the tool call. Child agent panels also use the terminal view.

The user can read terminal output and use the copy button. The terminal view does not send input to the process. The UI gets xterm.js when the terminal view opens. ANSI sequences control text color and cursor position. The cursor can replace a line with new output.

The process output WebSocket is active while the terminal view is open. A new connection gives a new output snapshot. Byte offsets show output gaps. The UTF-8 decoders keep data for standard output and standard error. The output events do not start message history reads.

The terminal height is 288 pixels. The view uses the available width. The user can move through output with the scrollbar.

The client keeps a maximum of 524288 characters. The limit for output chunks is 512. The terminal keeps 2000 lines. The UI removes previous output when the buffer is full.

The copy button uses terminal text. The source download keeps output text with ANSI sequences. When a tool section closes, the output connection stops. A session change also stops the connection. The connection stops when an agent panel closes. It also stops when the UI connection closes.

The process exit status is different from the tool result. A process can continue after an inactivity return. If the process buffer is not available, the UI uses the tool result. The terminal view also shows previous `bash` output.

## Process Notifications

The harness keeps process notifications as `runtime` messages for the model. The UI puts process notifications in the applicable `bash` section. The UI does not show the notification text as a conversation message. The API message history stays the same.

The fields `ToolCallID` and `ToolMessageID` connect a process record to a tool call. For a previous tool call with `wait: false`, the UI can use the process ID in the tool result. Command text and time values do not connect output to a tool call.

If the tool call is not available, the UI shows one process output section. The section is closed by default. Previous exit output stays available after the process buffer is removed. Other runtime messages and user text stay in the conversation.

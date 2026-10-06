# Message Display

## Libraries

The UI uses the libraries in the list:

- [Marked](https://marked.js.org/) reads Markdown text.
- [DOMPurify](https://github.com/cure53/DOMPurify) removes dangerous markup.
- [Highlight.js](https://highlightjs.org/) gives syntax highlighting.
- [Papa Parse](https://www.papaparse.com/docs) reads CSV data.

The dependency versions are in `ui/package-lock.json`. The code is in `ui/src/lib/molecules/content/`.

## Reasoning

The field `Message.Reasoning` contains reasoning text or a reasoning summary from the provider. The section with the label `Thinking` is closed by default. Select the button with the label `Thinking` to read the text. Content views are prepared when the section opens.

The client receives text from `model.chunk` events during the response. The field `message_id` connects event text to message history. A message has one display area. An open reasoning section stays open after the message is in history. The section is closed when the UI starts.

The client prepares event text at an interval of 100 milliseconds. A session change removes previous event text. The API does not send continuation data in messages or events.

## Markdown and Code

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

The client prepares content views when the user opens a tool section. The section shows input, output, and the value of `ToolCallID`. The same section shows a new tool result. The UI shows a tool result in a different section if the tool call is not in the history.

Tool sections, reasoning sections, and model text use the UI background. The sections have a line between them. A different color identifies a tool section. When a tool section opens, one panel contains input and output. The panel has a background color and border. Input and output have a line between them.

Content views in the panel do not have a card container. Copy buttons, source downloads, and CSV views stay available.

Message history does not include tool status fields. The UI shows the label `Result` when a tool result is available. It does not change error text in the output.

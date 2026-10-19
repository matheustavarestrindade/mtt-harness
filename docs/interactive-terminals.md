# Process Terminals

## Tool Configuration

The `bash` tool uses a pseudo-terminal by default. Standard input, standard output, and standard error use the terminal. The process API puts the terminal output in `stdout`. ANSI sequences stay in the output.

```text
Input       Default   Valid values
terminal    true      true or false
columns     120       2..500
rows        30        1..200
```

With `terminal: false`, the process uses pipes. Standard output and standard error use different streams. Pipe input is available. Terminal resize is not available.

The terminal environment uses `TERM=xterm-256color` and `COLORTERM=truecolor`. A value in `ProcessSpec.Env` stays the same. A command can select output without ANSI sequences. A container can have a different terminal configuration.

The library is `github.com/creack/pty` v1.1.24. The Unix adapter supplies the pseudo-terminal. CGO is not necessary for terminals.

## Process Socket

The process socket path is:

```text
GET /processes/{id}/output
```

Authentication is necessary. The start frame has `terminal` and `input` fields. The output frames keep `data`, `data_base64`, byte offsets, and exit status. The UI sends output bytes to xterm.js. Pipe standard output and standard error use different UTF-8 decoders.

The client can send the JSON frames:

```json
{"type":"input","data_base64":"aGVsbG8K"}
{"type":"resize","columns":120,"rows":30}
{"type":"signal","signal":"interrupt"}
```

An input frame can use a UTF-8 `data` string as an alternative to `data_base64`.

```text
Decoded input per frame     4096 bytes
Socket frame limit          16 KiB
UI input action limit       64 KiB
Input write time limit      2 seconds
```

An input error does not stop the process. A `stream: control` frame gives the error. The UI does not send the input again automatically. The input is not a conversation message.

`Ctrl-C` sends byte `0x03` to a terminal. For a pipe process, the `interrupt` control sends `SIGINT` to the process group. The `resize` frame gives new dimensions to the terminal. Terminal views of one process use the same input and dimensions.

## View Control

The UI uses `@xterm/xterm` with an output buffer limit. The terminal view controls the socket, terminal, resize observer, and input handlers. When the terminal view closes, the UI stops the socket and terminal. The process continues.

Process exit sends the last output and exit frame, then closes the socket. The input stops. The UI can show the output after the process stops. The harness uses the same cancellation rules.

The UI uses output from the process buffer or the tool result. Output without ANSI sequences uses the default text color.

## Source Documents

- [creack/pty](https://github.com/creack/pty)
- [xterm.js](https://xtermjs.org/docs/)

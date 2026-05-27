# markdown

Glamour wrapper used by the transcript widget. Two surfaces:

* `RenderFinal(body, width) string` — for completed messages. Plain Glamour
  render with the auto-detected (light/dark) style.
* `RenderStreaming(body, width) string` — for in-flight messages. If the
  buffer ends mid-code-fence we append a synthetic closing ``` ``` so Glamour
  doesn't panic / corrupt — then strip the synthetic block from the output.

The caller throttles `RenderStreaming` to one render per 50 ms; we don't
re-implement throttling here because the throttle window is owned by the
parent Model.

## Why synthetic close

Glamour parses with goldmark, which requires balanced fences. A stream that
ends in `\n` ``` ``` `go\nfn` (no trailing ``` ```) is malformed; goldmark either
keeps reading or treats the remainder as a paragraph. Either way the user
sees garbage. Appending a synthetic close lets goldmark render the half-block
as a proper fenced code block; the final render (no synthetic close) replaces
it with the real closed block.

## NO_COLOR fallback

When `NO_COLOR` is set or the caller's theme is monochrome the renderer falls
back to Glamour's `ASCIIStyle` (no ANSI sequences emitted).

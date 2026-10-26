// Edits of a Markdown text field: each takes the value and the selection and
// returns the new value with the selection to restore.
export type MarkdownEdit = { value: string; start: number; end: number };

// wrapSelection puts marks around the selection (**bold**, *italic*); with
// nothing selected it inserts the marks around a placeholder and selects it.
// Marks already around the selection are taken off.
export function wrapSelection(
  value: string,
  selectionStart: number,
  selectionEnd: number,
  mark: string,
  placeholder: string
): MarkdownEdit {
  const [start, end] = trimSelection(value, selectionStart, selectionEnd);
  const before = value.slice(0, start);
  const after = value.slice(end);
  if (before.endsWith(mark) && after.startsWith(mark)) {
    return {
      value: before.slice(0, -mark.length) + value.slice(start, end) + after.slice(mark.length),
      start: start - mark.length,
      end: end - mark.length,
    };
  }
  const selected = value.slice(start, end) || placeholder;
  return {
    value: before + mark + selected + mark + after,
    start: start + mark.length,
    end: start + mark.length + selected.length,
  };
}

// insertLink makes the selection the text of a link and selects the URL to
// type over.
export function insertLink(value: string, selectionStart: number, selectionEnd: number): MarkdownEdit {
  const [start, end] = trimSelection(value, selectionStart, selectionEnd);
  const text = value.slice(start, end) || "link text";
  const url = "https://";
  const inserted = `[${text}](${url})`;
  const urlStart = start + text.length + 3;
  return {
    value: value.slice(0, start) + inserted + value.slice(end),
    start: urlStart,
    end: urlStart + url.length,
  };
}

// prefixLines starts every line of the selection with a prefix ("# ", "- ")
// or a number ("1. ", "2. "), or takes it off when every line has it.
export function prefixLines(
  value: string,
  start: number,
  end: number,
  prefix: string | "numbered"
): MarkdownEdit {
  const lineStart = value.lastIndexOf("\n", start - 1) + 1;
  const lineEndIndex = value.indexOf("\n", Math.max(end - (end > start ? 1 : 0), start));
  const lineEnd = lineEndIndex === -1 ? value.length : lineEndIndex;
  const lines = value.slice(lineStart, lineEnd).split("\n");

  const pattern = prefix === "numbered" ? /^\d+\. / : new RegExp(`^${escapeRegExp(prefix)}`);
  const hasPrefix = lines.every((line) => pattern.test(line));
  const nextLines = lines.map((line, index) => {
    if (hasPrefix) {
      return line.replace(pattern, "");
    }
    return (prefix === "numbered" ? `${index + 1}. ` : prefix) + line;
  });
  const replaced = nextLines.join("\n");
  return {
    value: value.slice(0, lineStart) + replaced + value.slice(lineEnd),
    start: lineStart,
    end: lineStart + replaced.length,
  };
}

// trimSelection leaves spaces and line breaks at the edges of a selection
// outside the marks: a double click often takes the space after a word, and
// "**word **" is not bold.
function trimSelection(value: string, start: number, end: number): [number, number] {
  while (start < end && /\s/.test(value[start])) {
    start += 1;
  }
  while (end > start && /\s/.test(value[end - 1])) {
    end -= 1;
  }
  return [start, end];
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

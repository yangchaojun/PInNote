/**
 * Textarea-level markdown formatting helpers. Each helper returns
 * { content, selectionStart, selectionEnd } so the caller can restore the
 * selection after setting the new value.
 */

export interface FormatResult {
  content: string;
  selectionStart: number;
  selectionEnd: number;
}

export type FormatFn = (content: string, start: number, end: number) => FormatResult;

/** Wraps the selection with `prefix`/`suffix` (e.g. ** for bold). */
function wrap(prefix: string, suffix = prefix): FormatFn {
  return (content, start, end) => {
    const selected = content.slice(start, end);
    const before = content.slice(Math.max(0, start - prefix.length), start);
    const after = content.slice(end, end + suffix.length);

    // Toggling: strip the markers when the selection is already wrapped.
    if (selected.startsWith(prefix) && selected.endsWith(suffix) && selected.length > prefix.length + suffix.length) {
      const inner = selected.slice(prefix.length, selected.length - suffix.length);
      return {
        content: content.slice(0, start) + inner + content.slice(end),
        selectionStart: start,
        selectionEnd: start + inner.length,
      };
    }
    if (before === prefix && after === suffix) {
      return {
        content: content.slice(0, start - prefix.length) + selected + content.slice(end + suffix.length),
        selectionStart: start - prefix.length,
        selectionEnd: end - prefix.length,
      };
    }

    const next = prefix + (selected || "文本") + suffix;
    return {
      content: content.slice(0, start) + next + content.slice(end),
      selectionStart: start + prefix.length,
      // When there was a selection keep it selected inside the markers.
      selectionEnd: start + prefix.length + (selected ? selected.length : "文本".length),
    };
  };
}

/** Prefixes each selected line with `marker` (headings, lists, quotes). */
function toggleLinePrefix(marker: string | ((i: number) => string)): FormatFn {
  return (content, start, end) => {
    const lineStart = content.lastIndexOf("\n", Math.max(0, start - 1)) + 1;
    const lineEndIdx = content.indexOf("\n", end);
    const lineEnd = lineEndIdx === -1 ? content.length : lineEndIdx;
    const lines = content.slice(lineStart, lineEnd).split("\n");

    const markers = lines.map((_, i) => (typeof marker === "string" ? marker : marker(i)));
    const allPrefixed = lines.every((l, i) => l.startsWith(markers[i]));

    let deltaBefore = 0;
    let deltaTotal = 0;
    const next = lines
      .map((line, i) => {
        const m = markers[i];
        if (allPrefixed) {
          if (i === 0) deltaBefore = -m.length;
          deltaTotal -= m.length;
          return line.slice(m.length);
        }
        if (i === 0) deltaBefore = m.length;
        deltaTotal += m.length;
        return m + line;
      })
      .join("\n");

    return {
      content: content.slice(0, lineStart) + next + content.slice(lineEnd),
      selectionStart: Math.max(lineStart, start + deltaBefore),
      selectionEnd: Math.max(lineStart, end + deltaTotal),
    };
  };
}

export const formatBold = wrap("**");
export const formatItalic = wrap("*");
export const formatStrikethrough = wrap("~~");
export const formatCode = wrap("`");
export const formatLink: FormatFn = (content, start, end) => {
  const selected = content.slice(start, end) || "链接";
  const text = `[${selected}](url)`;
  return {
    content: content.slice(0, start) + text + content.slice(end),
    selectionStart: start + selected.length + 3,
    selectionEnd: start + selected.length + 6,
  };
};
export const formatHeading = (level: number) => toggleLinePrefix("#".repeat(level) + " ");
export const formatBulletList = toggleLinePrefix("- ");
export const formatOrderedList = toggleLinePrefix((i) => `${i + 1}. `);
export const formatBlockquote = toggleLinePrefix("> ");
export const formatCheckbox = toggleLinePrefix("- [ ] ");

/**
 * Toggles the checkbox on the given (1-based) markdown source line, used by
 * interactive task-list checkboxes in the preview.
 */
export function toggleCheckboxAtLine(content: string, line: number): string {
  const lines = content.split("\n");
  const idx = line - 1;
  if (idx < 0 || idx >= lines.length) return content;
  lines[idx] = lines[idx].replace(/^(\s*[-*+]\s+\[)([ xX])(\])/, (_, a, mark, b) =>
    a + (mark === " " ? "x" : " ") + b,
  );
  return lines.join("\n");
}

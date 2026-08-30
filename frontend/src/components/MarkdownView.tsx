import { memo } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

interface MarkdownViewProps {
  content: string;
  /** Called with the 1-based source line when a task checkbox is clicked. */
  onToggleCheckbox?: (line: number) => void;
}

/**
 * Renders markdown with GFM support (tables, strikethrough and task-list
 * checkboxes). Checkboxes become interactive when onToggleCheckbox is given.
 *
 * hast `input` nodes carry no source position, so the source line is stamped
 * onto the parent task-list `li` and read back from the DOM on toggle.
 */
function MarkdownViewInner({ content, onToggleCheckbox }: MarkdownViewProps) {
  return (
    <div className="markdown-body">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          li({ node, children, className, ...props }) {
            const line = node?.position?.start?.line ?? 0;
            const isTask = typeof className === "string" && className.includes("task-list-item");
            return (
              <li className={className} {...props} data-source-line={isTask ? line : undefined}>
                {children}
              </li>
            );
          },
          input({ checked, type }) {
            if (type !== "checkbox") return <input type={type} checked={checked} readOnly />;
            return (
              <input
                type="checkbox"
                checked={!!checked}
                onClick={(e) => e.stopPropagation()}
                onChange={(e) => {
                  const li = (e.target as HTMLElement).closest("li");
                  const line = Number(li?.getAttribute("data-source-line") ?? 0);
                  if (line > 0) onToggleCheckbox?.(line);
                }}
              />
            );
          },
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}

export const MarkdownView = memo(MarkdownViewInner);

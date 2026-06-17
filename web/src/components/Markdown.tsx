import { memo } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import { cn } from "@/lib/utils";
import { CsvBlock } from "./CsvBlock";

// csvCodeOverride intercepts fenced ```csv / ```tsv blocks and renders them
// as interactive CsvBlock (table + chart toggle) instead of plain <pre>.
// All other languages (go, json, etc.) fall through to highlight.js.
function csvCodeOverride({
  className,
  children,
}: {
  className?: string;
  children?: React.ReactNode;
}) {
  const lang = /language-(csv|tsv)/.exec(className || "")?.[1];
  const text = String(children ?? "").replace(/\n$/, "");
  if (lang === "csv" || lang === "tsv") {
    return <CsvBlock code={text} />;
  }
  // Fall back to the default <pre><code> rendering — rehype-highlight has
  // already tokenized children at this point.
  return (
    <code className={className}>{children}</code>
  );
}

export const Markdown = memo(function Markdown({
  content,
  className,
}: {
  content: string;
  className?: string;
}) {
  return (
    <div className={cn("markdown-body", className)}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeHighlight]}
        components={{ code: csvCodeOverride }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
});

import React from 'react';
import ReactMarkdown from 'react-markdown';
import { Link } from 'react-router';
import remarkGfm from 'remark-gfm';
import { JsonTable } from './JsonTable';
import { MarkdownTable } from './MarkdownTable';
import { normalizeAiPath } from '../../utils/aiPaths';

function extractTextFromChildren(children: React.ReactNode): string {
  let text = '';
  React.Children.forEach(children, (child) => {
    if (typeof child === 'string') {
      text += child;
    } else if (React.isValidElement(child)) {
      text += extractTextFromChildren((child.props as any)?.children);
    }
  });
  return text;
}

function isJsonArray(str: string): boolean {
  try {
    const parsed = JSON.parse(str.trim());
    return Array.isArray(parsed) && parsed.length > 0 && typeof parsed[0] === 'object' && parsed[0] !== null;
  } catch {
    return false;
  }
}

export function MessageContent({ content }: { content: string }) {
  return (
    <div className="prose prose-sm max-w-none break-words dark:prose-invert prose-a:text-primary prose-a:underline">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          h1: ({ children }) => <h1 className="mb-2 mt-4 text-base font-bold text-foreground">{children}</h1>,
          h2: ({ children }) => <h2 className="mb-2 mt-4 text-sm font-bold text-foreground">{children}</h2>,
          h3: ({ children }) => <h3 className="mb-1.5 mt-3 text-sm font-semibold text-foreground">{children}</h3>,
          a: ({ href, children }) => {
            const normalized = typeof href === "string" ? normalizeAiPath(href) : "";
            if (normalized.startsWith("/") && !normalized.startsWith("//")) {
              return <Link to={normalized} className="font-medium text-primary underline underline-offset-2">{children}</Link>;
            }
            return <a href={href} target="_blank" rel="noreferrer" className="font-medium text-primary underline underline-offset-2">{children}</a>;
          },
          p: ({ children }) => <p className="my-3 text-sm leading-relaxed text-foreground">{children}</p>,
          strong: ({ children }) => <strong className="font-semibold text-foreground">{children}</strong>,
          table: ({ children }) => <MarkdownTable>{children}</MarkdownTable>,
          th: ({ children }) => <th className="break-words border border-border/60 bg-muted/30 px-3 py-2 text-left font-semibold">{children}</th>,
          td: ({ children }) => <td className="border border-border/60 px-3 py-2 align-top break-words">{children}</td>,
          code: ({ children }) => (
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-caption break-all">{children}</code>
          ),
          pre: ({ children }) => {
            const text = extractTextFromChildren(children).trim();
            if (text && isJsonArray(text)) {
              return <JsonTable content={text} />;
            }
            return (
              <pre className="overflow-x-auto rounded-lg bg-muted p-3 text-xs leading-relaxed">{children}</pre>
            );
          },
          ul: ({ children }) => <ul className="my-2 list-disc space-y-1 pl-5 text-sm">{children}</ul>,
          ol: ({ children }) => <ol className="my-2 list-decimal space-y-1 pl-5 text-sm">{children}</ol>,
          li: ({ children }) => <li className="leading-relaxed">{children}</li>,
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}


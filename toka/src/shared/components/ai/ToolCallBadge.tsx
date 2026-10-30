import { Check, Loader2 } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '../../ui/tooltip';

type Props = {
  toolName: string;
  status: 'loading' | 'done';
  resultPreview?: string;
};

const labelForTool = (toolName: string | undefined | null) => {
  const safe = typeof toolName === 'string' ? toolName : '';
  const base = safe.replace(/_/g, ' ').trim();
  return base.length ? base.charAt(0).toUpperCase() + base.slice(1) : 'Executando';
};

export function ToolCallBadge({ toolName, status, resultPreview }: Props) {
  const label = labelForTool(toolName);
  const loadingText = `${label}…`;

  const previewText = (() => {
    const raw = (resultPreview || '').trim();
    if (!raw) return '';
    try {
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === 'object' && typeof (parsed as any).message === 'string') {
        return (parsed as any).message;
      }
      return JSON.stringify(parsed, null, 2);
    } catch {
      return raw;
    }
  })();

  const badge = (
    <div className="inline-flex items-center gap-2 text-xs rounded-full px-3 py-1 bg-primary/10 border border-primary/20 text-primary max-w-full min-w-0">
      {status === 'loading' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
      <span className="min-w-0 truncate">{status === 'loading' ? loadingText : label}</span>
    </div>
  );

  if (!previewText || status !== 'done') return badge;

  return (
    <Tooltip>
      <TooltipTrigger asChild>{badge}</TooltipTrigger>
      <TooltipContent
        side="top"
        sideOffset={8}
        className="bg-popover text-popover-foreground border border-border/60 shadow-lg max-w-[min(420px,calc(100vw-2rem))]"
      >
        <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed">{previewText}</pre>
      </TooltipContent>
    </Tooltip>
  );
}


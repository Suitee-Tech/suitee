import { ExternalLink, Copy, Wand2 } from "lucide-react";
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "../../ui/card";
import { cn } from "../../ui/utils";
import { normalizeAiPath } from "../../utils/aiPaths";
import { Button } from "../../ui/button";
import { Badge } from "../../ui/badge";

export type SuiteeCardItem = {
  key?: string;
  title: string;
  subtitle?: string;
  badges?: string[];
  status_color?: "green" | "yellow" | "red" | "gray";
  open_path?: string;
  copy?: string;
  action_prompt?: string;
  metadata?: Array<{ kind: "currency" | "date" | "phone" | "duration"; value: string }>;
  details?: Array<{ label: string; value: string }>;
};

export type SuiteeCardsBlock = {
  title?: string;
  items: SuiteeCardItem[];
};

type Props = {
  block: SuiteeCardsBlock;
  onOpen: (path: string) => void;
  onCopy: (text: string) => void;
  onAction: (prompt: string) => void;
};

const STATUS_BORDER: Record<NonNullable<SuiteeCardItem["status_color"]>, string> = {
  green: "border-l-status-ok",
  yellow: "border-l-status-warning",
  red: "border-l-status-critical",
  gray: "border-l-muted-foreground",
};

export function ResultCards({ block, onOpen, onCopy, onAction }: Props) {
  const labelForMeta = (kind: "currency" | "date" | "phone" | "duration" | string) => {
    switch (kind) {
      case "currency":
        return "Valor";
      case "date":
        return "Data";
      case "phone":
        return "Telefone";
      case "duration":
        return "Duração";
      default:
        return "";
    }
  };

  return (
    <div className="space-y-3">
      {block.title && <div className="text-xs text-muted-foreground">{block.title}</div>}
      <div className="space-y-2">
        {block.items.map((it, idx) => (
          <Card
            key={it.key || `${it.title}_${idx}`}
            className={cn(
              "border-border/60 bg-background/60",
              it.status_color && "border-l-4",
              it.status_color && STATUS_BORDER[it.status_color],
            )}
          >
            <CardHeader className="pb-2">
              <CardTitle className="text-sm">{it.title}</CardTitle>
              {(it.subtitle || "").trim() !== "" && <div className="text-xs text-muted-foreground mt-1">{it.subtitle}</div>}
              {Array.isArray(it.badges) && it.badges.length > 0 && (
                <div className="flex flex-wrap gap-2 mt-2">
                  {it.badges.map((b) => (
                    <Badge key={b} variant="outline" className="rounded-full">
                      {b}
                    </Badge>
                  ))}
                </div>
              )}
            </CardHeader>
            <CardContent className="pt-0 space-y-2">
              {it.metadata && it.metadata.length > 0 && (
                <div className="flex flex-wrap gap-2">
                  {it.metadata.slice(0, 4).map((m, i) => (
                    <span key={i} className="text-caption text-muted-foreground">
                      <span className="text-foreground/70">{labelForMeta(m.kind)}: </span>
                      {m.value}
                    </span>
                  ))}
                </div>
              )}
              {it.details && it.details.length > 0 && (
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-1">
                  {it.details.slice(0, 6).map((d, i) => (
                    <div key={i} className="truncate text-caption text-muted-foreground">
                      <span className="text-foreground/70">{d.label}: </span>
                      {d.value}
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
            <CardFooter className="gap-2 flex-wrap">
              {it.open_path && (
                <Button size="sm" variant="outline" onClick={() => onOpen(normalizeAiPath(it.open_path))}>
                  <ExternalLink className="h-4 w-4" />
                  Abrir
                </Button>
              )}
              {it.copy && (
                <Button size="sm" variant="outline" onClick={() => onCopy(it.copy!)}>
                  <Copy className="h-4 w-4" />
                  Copiar
                </Button>
              )}
              {it.action_prompt && (
                <Button size="sm" onClick={() => onAction(it.action_prompt!)}>
                  <Wand2 className="h-4 w-4" />
                  Sugerir próximos passos
                </Button>
              )}
            </CardFooter>
          </Card>
        ))}
      </div>
    </div>
  );
}


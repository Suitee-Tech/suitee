import { ExternalLink } from "lucide-react";
import { useNavigate } from "react-router";

import { Button } from "../../ui/button";
import { normalizeAiPath } from "../../utils/aiPaths";
import { UserAvatar } from "../UserAvatar";

interface JsonTableProps {
  content: string;
}

const HIDDEN_COLUMNS = new Set(["path", "open_path"]);
const HEADER_LABELS: Record<string, string> = {
  title: "Título",
  name: "Nome",
  nome: "Nome",
  subtitle: "Detalhe",
  description: "Descrição",
  descricao: "Descrição",
  status: "Status",
  role: "Perfil",
  email: "Email",
  phone: "Telefone",
  created_at: "Cadastro",
  updated_at: "Atualizado",
  date: "Data",
  value: "Valor",
  total: "Total",
};

const STATUS_LABELS: Record<string, string> = {
  PENDING: "Pendente",
  APPROVED: "Aprovado",
  PAID: "Pago",
  ACTIVE: "Ativo",
  INACTIVE: "Inativo",
  CANCELLED: "Cancelado",
};

function friendlyEntity(path: string): string {
  if (path.startsWith("/leads/")) return "Lead";
  if (path.startsWith("/imoveis/")) return "Imóvel";
  if (path.startsWith("/esteira/deals/")) return "Negócio";
  if (path.startsWith("/esteira")) return "Esteira";
  if (path.startsWith("/contratos/")) return "Contrato";
  if (path.startsWith("/contratos")) return "Contratos";
  if (path.startsWith("/financeiro/repasses")) return "Repasse";
  if (path.startsWith("/financeiro/cobrancas")) return "Cobrança";
  if (path.startsWith("/financeiro/comissoes")) return "Comissão";
  if (path.startsWith("/financeiro/impostos")) return "Imposto";
  if (path.startsWith("/financeiro/payouts")) return "Repasse";
  if (path.startsWith("/financeiro")) return "Financeiro";
  if (path.startsWith("/documentos")) return "Documento";
  if (path.startsWith("/agenda")) return "Agenda";
  if (path.startsWith("/mensagens")) return "Mensagem";
  if (path.startsWith("/relatorios")) return "Relatório";
  if (path.startsWith("/parceiros")) return "Parceiro";
  if (path.startsWith("/locacao/")) return "Locação";
  if (path.startsWith("/locacao")) return "Locações";
  if (path.startsWith("/manutencao")) return "Manutenção";
  if (path.startsWith("/chamados")) return "Chamado";
  return "Abrir";
}

function isAppPath(value: string): boolean {
  return /^\/(leads|imoveis|esteira|contratos|financeiro|documentos|mensagens|agenda|relatorios|parceiros|locacao|manutencao|chamados)(\/|$)/i.test(value);
}

function isImageKey(key: string): boolean {
  const k = key.toLowerCase();
  return k === "image" || k === "avatar" || k === "avatar_url" || k === "photo" || k === "picture" || k === "img" || k === "icon";
}

function headerLabel(key: string): string {
  return HEADER_LABELS[key] ?? key.replace(/_/g, " ").replace(/^\w/, (char) => char.toUpperCase());
}

function getOpenPath(item: Record<string, unknown>): string | undefined {
  const value = item.open_path ?? item.path;
  if (typeof value !== "string") return undefined;
  const normalized = normalizeAiPath(value);
  return isAppPath(normalized) ? normalized : undefined;
}

export function JsonTable({ content }: JsonTableProps) {
  const navigate = useNavigate();

  let parsed: unknown;
  try {
    parsed = JSON.parse(content);
  } catch {
    return null;
  }

  if (!Array.isArray(parsed) || parsed.length === 0 || typeof parsed[0] !== "object" || parsed[0] === null) {
    return null;
  }

  const items = parsed as Record<string, unknown>[];
  const allKeys = new Set<string>();
  for (const item of items) {
    for (const key of Object.keys(item)) allKeys.add(key);
  }

  const priorityKeys = ["title", "name", "nome", "subtitle", "description", "descricao"];
  const keys = Array.from(allKeys).sort((a, b) => {
    const aImg = isImageKey(a);
    const bImg = isImageKey(b);
    if (aImg && !bImg) return 1;
    if (!aImg && bImg) return -1;
    const aIdx = priorityKeys.indexOf(a);
    const bIdx = priorityKeys.indexOf(b);
    if (aIdx !== -1 && bIdx !== -1) return aIdx - bIdx;
    if (aIdx !== -1) return -1;
    if (bIdx !== -1) return 1;
    return a.localeCompare(b);
  });

  const imageKeys = keys.filter(isImageKey);
  const tableKeys = keys.filter((key) => !imageKeys.includes(key) && !HIDDEN_COLUMNS.has(key));
  const hasActions = items.some((item) => Boolean(getOpenPath(item)));
  const titleOf = (item: Record<string, unknown>) => String(item.title ?? item.name ?? item.nome ?? "");
  const getImage = (item: Record<string, unknown>) => {
    for (const key of imageKeys) {
      const value = item[key];
      if (typeof value === "string" && value.startsWith("http")) return value;
    }
    return undefined;
  };

  const renderActions = (item: Record<string, unknown>) => {
    const path = getOpenPath(item);
    if (!path) return null;
    return (
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="h-7 gap-1 px-2 text-caption"
        onClick={() => navigate(path)}
      >
        <ExternalLink className="h-3.5 w-3.5" />
        {friendlyEntity(path)}
      </Button>
    );
  };

  return (
    <div className="@container my-3 min-w-0">
      <div className="hidden overflow-x-auto rounded-lg border border-border/60 @[700px]:block">
        <table className="w-full table-fixed border-collapse text-xs">
          <caption className="sr-only">Resultados da consulta</caption>
          <thead>
            <tr className="bg-muted/50">
              {imageKeys.length > 0 && <th className="w-8 px-3 py-2 text-left font-semibold text-muted-foreground" />}
              {tableKeys.map((key) => (
                <th key={key} className="break-words px-3 py-2 text-left font-semibold text-muted-foreground">
                  {headerLabel(key)}
                </th>
              ))}
              {hasActions && <th className="px-3 py-2 text-right font-semibold text-muted-foreground">Ação</th>}
            </tr>
          </thead>
          <tbody>
            {items.map((item, index) => (
              <tr key={index} className="border-t border-border/40 transition-colors hover:bg-muted/20">
                {imageKeys.length > 0 && (
                  <td className="px-3 py-2">
                    {getImage(item) ? (
                      <UserAvatar
                        name={titleOf(item)}
                        avatarUrl={getImage(item)}
                        className="h-6 w-6"
                        fallbackClassName="bg-secondary text-secondary-foreground text-caption"
                      />
                    ) : (
                      <div className="h-6 w-6" />
                    )}
                  </td>
                )}
                {tableKeys.map((key) => (
                  <td key={key} className="max-w-[280px] px-3 py-2 align-top text-foreground">
                    <CellValue value={item[key]} columnKey={key} navigate={navigate} />
                  </td>
                ))}
                {hasActions && <td className="px-3 py-2 text-right">{renderActions(item)}</td>}
              </tr>
            ))}
          </tbody>
        </table>
        <div className="border-t border-border/40 bg-muted/20 px-3 py-1.5 text-caption text-muted-foreground/70">
          {items.length} {items.length === 1 ? "registro" : "registros"}
        </div>
      </div>

      <div className="space-y-2 @[700px]:hidden">
        {items.map((item, index) => {
          const title = titleOf(item);
          const details = tableKeys.filter((key) => !["title", "name", "nome", "subtitle"].includes(key));
          return (
            <div key={index} className="rounded-xl border border-border/60 bg-background/70 p-3">
              <div className="flex items-start gap-2">
                {getImage(item) && (
                  <UserAvatar
                    name={title}
                    avatarUrl={getImage(item)}
                    className="h-7 w-7"
                    fallbackClassName="bg-secondary text-secondary-foreground text-caption"
                  />
                )}
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold text-foreground">{title || `Registro ${index + 1}`}</p>
                  {typeof item.subtitle === "string" && item.subtitle && (
                    <p className="mt-0.5 text-xs text-muted-foreground">{item.subtitle}</p>
                  )}
                </div>
              </div>
              <div className="mt-2 space-y-1.5">
                {details.map((key) => (
                  <div key={key} className="grid grid-cols-[minmax(84px,32%)_1fr] gap-2 text-xs">
                    <span className="font-semibold text-muted-foreground">{headerLabel(key)}</span>
                    <span className="min-w-0 break-words text-foreground">
                      <CellValue value={item[key]} columnKey={key} navigate={navigate} />
                    </span>
                  </div>
                ))}
              </div>
              {renderActions(item) && <div className="mt-3 flex justify-end">{renderActions(item)}</div>}
            </div>
          );
        })}
        <p className="px-1 text-caption text-muted-foreground/70">
          {items.length} {items.length === 1 ? "registro" : "registros"}
        </p>
      </div>
    </div>
  );
}

function CellValue({
  value,
  columnKey,
  navigate,
}: {
  value: unknown;
  columnKey: string;
  navigate: ReturnType<typeof useNavigate>;
}) {
  if (value === null || value === undefined) return <span className="text-muted-foreground/40">—</span>;

  if (typeof value === "boolean") {
    return <span className={value ? "font-medium text-status-ok" : "text-muted-foreground"}>{value ? "Sim" : "Não"}</span>;
  }

  if (typeof value === "number") {
    if ((value > 1_000_000_000 && value < 2_000_000_000) || (value > 1_000_000_000_000 && value < 2_000_000_000_000)) {
      const date = new Date(value > 1_000_000_000_000 ? value : value * 1000);
      if (!Number.isNaN(date.getTime())) return <span>{date.toLocaleDateString("pt-BR")}</span>;
    }
    return <span>{value.toLocaleString("pt-BR")}</span>;
  }

  if (typeof value === "string") {
    if (columnKey === "status") {
      const label = STATUS_LABELS[value] ?? value;
      return <span className="inline-flex rounded-full bg-muted px-2 py-0.5 text-caption font-semibold text-foreground">{label}</span>;
    }
    const normalized = normalizeAiPath(value);
    if (isAppPath(normalized)) {
      return (
        <button type="button" className="font-medium text-primary hover:underline" onClick={() => navigate(normalized)}>
          {friendlyEntity(normalized)}
        </button>
      );
    }
    if (/^[a-f0-9-]{32,}$/i.test(value.trim())) {
      return <span className="break-all font-mono text-muted-foreground" title={value}>{value.slice(0, 8)}…</span>;
    }
    if (/^\d{4}-\d{2}-\d{2}/.test(value)) {
      const date = new Date(value);
      if (!Number.isNaN(date.getTime())) {
        return <span>{date.toLocaleDateString("pt-BR", { timeZone: "UTC" })}</span>;
      }
    }
    if (value.startsWith("http://") || value.startsWith("https://")) {
      return (
        <a href={value} target="_blank" rel="noopener noreferrer" className="break-all text-primary hover:underline">
          {value.length > 50 ? `${value.slice(0, 47)}…` : value}
        </a>
      );
    }
    return <span className="break-words">{value}</span>;
  }

  try {
    return <span className="break-all font-mono text-caption text-muted-foreground">{JSON.stringify(value)}</span>;
  } catch {
    return <span className="text-muted-foreground">—</span>;
  }
}

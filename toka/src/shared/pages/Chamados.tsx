import { useCallback, useEffect, useMemo, useState } from "react";
import { BadgeCheck, CircleUser, Info, Play, X } from "lucide-react";

import { useAuth } from "../context/AuthContext";
import { usePermissionsContext } from "../../lib/permissions/PermissionsContext";
import { ticketService, type Ticket } from "../services/ticket.service";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "../ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "../ui/table";
import { useWsSubscription } from "../ws/useWsSubscription";
import { MediaImage } from "../components/MediaImage";
import { getMediaUrl } from "../media";
import {
  badgeVariantForTicketPriority,
  badgeVariantForTicketStatus,
  ticketCategoryLabel,
  ticketCommentAuthorLabel,
  ticketPriorityLabel,
  ticketStatusLabel,
  ticketTypeLabel,
} from "../lib/tickets/presentation";

export default function Chamados() {
  const { user } = useAuth();
  const { can } = usePermissionsContext();
  const canWrite = can("tickets:write");
  const [tickets, setTickets] = useState<Ticket[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [status, setStatus] = useState<string>("all");
  const [type, setType] = useState<string>("all");
  const [search, setSearch] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selected, setSelected] = useState<Ticket | null>(null);
  const [comment, setComment] = useState("");

  const loadList = useCallback(async () => {
    setIsLoading(true);
    try {
      const res = await ticketService.list({
        status: status !== "all" ? status : undefined,
        type: type !== "all" ? type : undefined,
      });
      setTickets(res.data.data || []);
    } finally {
      setIsLoading(false);
    }
  }, [status, type]);

  const loadSelected = useCallback(async () => {
    if (!selectedId) {
      setSelected(null);
      return;
    }
    const latest = await ticketService.getById(selectedId);
    setSelected(latest.data.data);
  }, [selectedId]);

  useEffect(() => {
    loadList();
  }, [loadList]);

  useEffect(() => {
    loadSelected().catch(() => {});
  }, [loadSelected]);

  useWsSubscription("ticket_update", () => {
    loadList();
    loadSelected().catch(() => {});
  });
  useWsSubscription("ticket_comment", () => {
    loadList();
    loadSelected().catch(() => {});
  });

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return tickets;
    return tickets.filter((t) => (t.subject || "").toLowerCase().includes(q) || (t.category || "").toLowerCase().includes(q) || (t.description || "").toLowerCase().includes(q));
  }, [tickets, search]);

  const counts = useMemo(() => {
    const out: Record<string, number> = {};
    for (const t of tickets) out[t.status] = (out[t.status] || 0) + 1;
    return out;
  }, [tickets]);

  const onSelect = (t: Ticket) => {
    setSelectedId(t.id);
  };

  const onAssignToMe = async () => {
    if (!selected || !user?.id) return;
    const displayName = user.fullname || `${user.firstname || ""} ${user.lastname || ""}`.trim() || user.email;
    await ticketService.assign(selected.id, user.id, displayName);
    await loadList();
    await loadSelected();
  };

  const onStatusChange = async (next: string) => {
    if (!selected) return;
    await ticketService.updateStatus(selected.id, next);
    await loadList();
    await loadSelected();
  };

  const onAddComment = async () => {
    if (!selected || !user?.id || !comment.trim()) return;
    const displayName = user.fullname || `${user.firstname || ""} ${user.lastname || ""}`.trim() || user.email;
    await ticketService.addComment(selected.id, { author_id: user.id, author_name: displayName, author_type: "operator", content: comment.trim() });
    setComment("");
    await loadList();
    await loadSelected();
  };

  return (
    <div className="p-6">
      <div className="flex items-start justify-between gap-4 mb-6">
        <div>
          <h1 className="text-2xl font-bold">Chamados</h1>
          <p className="text-sm text-muted-foreground mt-1">Acompanhe e responda chamados do portal em tempo real.</p>
        </div>
        <div className="flex items-center gap-2">
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger className="w-[220px]">
              <SelectValue placeholder="Status" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Todos</SelectItem>
              <SelectItem value="open">Aberto ({counts.open || 0})</SelectItem>
              <SelectItem value="assigned">Atribuído ({counts.assigned || 0})</SelectItem>
              <SelectItem value="in_progress">Em andamento ({counts.in_progress || 0})</SelectItem>
              <SelectItem value="awaiting_client">Aguardando cliente ({counts.awaiting_client || 0})</SelectItem>
              <SelectItem value="resolved">Resolvido ({counts.resolved || 0})</SelectItem>
              <SelectItem value="closed">Fechado ({counts.closed || 0})</SelectItem>
            </SelectContent>
          </Select>

          <Select value={type} onValueChange={setType}>
            <SelectTrigger className="w-[200px]">
              <SelectValue placeholder="Tipo" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Todos</SelectItem>
              <SelectItem value="support">Suporte</SelectItem>
              <SelectItem value="maintenance">Manutenção</SelectItem>
              <SelectItem value="billing">Financeiro</SelectItem>
              <SelectItem value="documentation">Documentação</SelectItem>
              <SelectItem value="general">Geral</SelectItem>
            </SelectContent>
          </Select>
          <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Buscar..." className="w-56" />
        </div>
      </div>

      <div className="grid grid-cols-12 gap-4">
        <Card className="col-span-12 xl:col-span-7 overflow-hidden">
          <CardHeader className="border-b">
            <CardTitle className="text-base">Lista</CardTitle>
          </CardHeader>
          <CardContent className="px-0">
            {isLoading ? (
              <div className="p-8 text-center text-muted-foreground">Carregando...</div>
            ) : filtered.length === 0 ? (
              <div className="p-8 text-center text-muted-foreground">Nenhum chamado encontrado.</div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Assunto</TableHead>
                    <TableHead>Tipo</TableHead>
                    <TableHead>Prioridade</TableHead>
                    <TableHead>Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filtered.map((t) => (
                    <TableRow key={t.id} className="cursor-pointer" onClick={() => onSelect(t)}>
                      <TableCell className="font-medium">
                        <div className="flex flex-col">
                          <span>{t.subject}</span>
                          <span className="text-xs text-muted-foreground">{t.category}</span>
                        </div>
                      </TableCell>
                      <TableCell>{ticketTypeLabel(t.type)}</TableCell>
                      <TableCell>
                        <Badge variant={badgeVariantForTicketPriority(t.priority)}>{ticketPriorityLabel(t.priority)}</Badge>
                      </TableCell>
                      <TableCell>
                        <Badge variant={badgeVariantForTicketStatus(t.status)}>{ticketStatusLabel(t.status)}</Badge>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        <Card className="col-span-12 xl:col-span-5">
          <CardHeader className="border-b">
            <CardTitle className="text-base">Detalhes</CardTitle>
          </CardHeader>
          <CardContent>
            {!selected ? (
              <div className="text-sm text-muted-foreground">Selecione um chamado para ver detalhes.</div>
            ) : (
              <div className="flex flex-col h-full">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="text-lg font-semibold">{selected.subject}</div>
                    <div className="text-sm text-muted-foreground mt-1">{ticketCategoryLabel(selected.category)} · {ticketTypeLabel(selected.type)}</div>
                  </div>
                  <div className="flex flex-col items-end gap-2">
                    <Badge variant={badgeVariantForTicketPriority(selected.priority)}>{ticketPriorityLabel(selected.priority)}</Badge>
                    <Badge variant={badgeVariantForTicketStatus(selected.status)}>{ticketStatusLabel(selected.status)}</Badge>
                  </div>
                </div>

                <div className="mt-3 text-sm text-foreground whitespace-pre-wrap">{selected.description || "-"}</div>
                <div className="mt-3 text-xs text-muted-foreground">
                  Solicitante: {selected.requester_name || selected.requester_email || selected.requester_account_id || "-"}
                  {selected.assigned_to_name ? ` · Atribuído: ${selected.assigned_to_name}` : ""}
                </div>

                {canWrite && (
                  <div className="mt-4 flex flex-wrap gap-2">
                    <Button size="sm" variant="outline" onClick={onAssignToMe} disabled={!user?.id}>
                      <CircleUser />
                      Atribuir a mim
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => onStatusChange("in_progress")}>
                      <Play />
                      Iniciar
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => onStatusChange("awaiting_client")}>
                      <Info />
                      Solicitar info
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => onStatusChange("resolved")}>
                      <BadgeCheck />
                      Resolver
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => onStatusChange("closed")}>
                      <X />
                      Fechar
                    </Button>
                  </div>
                )}

                <div className="mt-4 border-t pt-4 flex-1 overflow-auto">
                  <div className="text-sm font-semibold mb-2">Comentários</div>
                  <div className="space-y-3">
                    {(selected.comments || []).map((c) => (
                      <div key={c.id} className="rounded-lg border bg-muted/30 p-3">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2">
                            <Badge variant={c.author_type === "client" ? "secondary" : "outline"}>{c.author_type === "client" ? "Cliente" : c.author_id === user?.id ? "Você" : "Operador"}</Badge>
                            <div className="text-sm font-semibold">{ticketCommentAuthorLabel(c, { currentUserId: user?.id })}</div>
                          </div>
                          <div className="text-xs text-muted-foreground">{new Date(c.created_at).toLocaleString("pt-BR")}</div>
                        </div>
                        <div className="mt-2 text-sm text-foreground whitespace-pre-wrap">{c.content}</div>
                        {c.attachments && c.attachments.length > 0 && (
                          <div className="mt-3 grid grid-cols-2 gap-2">
                            {c.attachments.map((attachment, index) => {
                              const url = getMediaUrl(attachment);
                              if (!url) return null;
                              return (
                                <a key={attachment} href={url} target="_blank" rel="noreferrer" className="overflow-hidden rounded-xl border bg-card">
                                  <MediaImage src={url} alt={`Arquivo enviado ${index + 1}`} className="h-24 w-full object-cover" fallback={<div className="flex h-24 items-center justify-center text-xs text-muted-foreground">Arquivo indisponível</div>} />
                                </a>
                              );
                            })}
                          </div>
                        )}
                      </div>
                    ))}
                    {(selected.comments || []).length === 0 && <div className="text-sm text-muted-foreground">Nenhum comentário.</div>}
                  </div>
                </div>

                <div className="mt-4 flex gap-2">
                  <Input
                    value={comment}
                    onChange={(e) => setComment(e.target.value)}
                    placeholder={canWrite ? "Escreva uma resposta..." : "Sem permissão para comentar"}
                    disabled={!canWrite}
                  />
                  <Button onClick={onAddComment} disabled={!comment.trim() || !user?.id || !canWrite}>
                    Enviar
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

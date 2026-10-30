import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { ArrowLeft, Plus, Wrench } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Badge } from "../../../shared/ui/badge";
import { Button } from "../../../shared/ui/button";
import { Card, CardContent } from "../../../shared/ui/card";
import { EmptyState } from "../../../shared/ui/EmptyState";
import { Input } from "../../../shared/ui/input";
import { Skeleton } from "../../../shared/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "../../../shared/ui/tabs";
import { Textarea } from "../../../shared/ui/textarea";
import { DealSwitcher } from "../components/DealSwitcher";
import { PortalError } from "../components/PortalState";
import { MediaImage } from "../../../shared/components/MediaImage";
import { getMediaUrl } from "../../../shared/media";
import { portalService, type CreateMaintenanceTicketRequest, type CreateTicketRequest, type PortalTicket } from "../services/portal.service";
import { portalKeys } from "../hooks/queryKeys";
import { usePortalDealMaintenance, usePortalDealTickets } from "../hooks/useDealResources";
import { usePortalPersona } from "../hooks/usePortalPersona";
import { useAddTicketComment, useCreateDealMaintenance, useCreateDealTicket } from "../hooks/useDealMutations";
import { usePortalWebSocket } from "../hooks/usePortalWebSocket";
import { usePortalDeals } from "../context/PortalDealsContext";
import {
  badgeVariantForTicketPriority,
  badgeVariantForTicketStatus,
  ticketCategoryLabel,
  ticketPriorityLabel,
  ticketStatusLabel,
  ticketTypeLabel,
} from "../../../shared/lib/tickets/presentation";
import { PortalPageHeader } from "../shell/PortalPageHeader";
import { PortalPage } from "../shell/PortalPage";

function formatDateBR(dateStr: string) {
  try {
    return new Date(dateStr).toLocaleDateString("pt-BR");
  } catch {
    return dateStr;
  }
}

export function PortalChamados() {
  const navigate = useNavigate();
  const location = useLocation();
  const [createOpen, setCreateOpen] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [tab, setTab] = useState<"all" | "support" | "maintenance" | "billing">("all");
  const [query, setQuery] = useState("");
  const [selectedTicketId, setSelectedTicketId] = useState<string | null>(null);
  const [selectedMaintenanceId, setSelectedMaintenanceId] = useState<string | null>(null);
  const [newComment, setNewComment] = useState("");
  const [commentFiles, setCommentFiles] = useState<File[]>([]);
  const [commentPreviews, setCommentPreviews] = useState<string[]>([]);
  const [isSendingComment, setIsSendingComment] = useState(false);
  const [form, setForm] = useState<CreateTicketRequest>({
    type: "support",
    category: "general",
    priority: "medium",
    subject: "",
    description: "",
  });
  const [maintenanceForm, setMaintenanceForm] = useState<CreateMaintenanceTicketRequest>({
    type: "Manutenção",
    category: "Geral",
    priority: "Media",
    description: "",
  });
  const [maintenanceFiles, setMaintenanceFiles] = useState<File[]>([]);
  const [maintenancePreviews, setMaintenancePreviews] = useState<string[]>([]);
  const { onMessage } = usePortalWebSocket();
  const { deals, activeDeal, activeDealId, isLoading, setActiveDealId } = usePortalDeals();

  const { copy } = usePortalPersona();
  const dealId = activeDeal?.id;
  const queryClient = useQueryClient();
  const ticketsQuery = usePortalDealTickets(dealId);
  const maintenanceQuery = usePortalDealMaintenance(dealId);
  const tickets = useMemo(() => ticketsQuery.data ?? [], [ticketsQuery.data]);
  const maintenanceTickets = useMemo(() => maintenanceQuery.data ?? [], [maintenanceQuery.data]);
  const isTicketsLoading = ticketsQuery.isPending && Boolean(dealId);
  const isMaintenanceLoading = maintenanceQuery.isPending && Boolean(dealId);
  const createMaintenance = useCreateDealMaintenance(dealId);
  const createTicket = useCreateDealTicket(dealId);
  const addComment = useAddTicketComment(selectedTicketId ?? undefined, dealId);
  const selectedTicketQuery = useQuery<PortalTicket>({
    queryKey: selectedTicketId ? portalKeys.ticket(selectedTicketId) : [...portalKeys.all(), "ticket", ""],
    queryFn: () => portalService.getTicketById(selectedTicketId!),
    enabled: Boolean(selectedTicketId),
  });
  const selectedTicket = selectedTicketQuery.data ?? null;

  useEffect(() => {
    return () => {
      for (const u of commentPreviews) URL.revokeObjectURL(u);
    };
  }, [commentPreviews]);

  useEffect(() => {
    return () => {
      for (const u of maintenancePreviews) URL.revokeObjectURL(u);
    };
  }, [maintenancePreviews]);

  useEffect(() => {
    setSelectedTicketId(null);
    setSelectedMaintenanceId(null);
    setCreateOpen(false);
    setNewComment("");
    setCommentFiles([]);
    setCommentPreviews([]);
    setMaintenanceFiles([]);
    setMaintenancePreviews([]);
  }, [dealId]);

  const reload = () => {
    if (!dealId) return;
    queryClient.invalidateQueries({ queryKey: portalKeys.dealTickets(dealId) });
  };

  const reloadMaintenance = () => {
    if (!dealId) return;
    queryClient.invalidateQueries({ queryKey: portalKeys.dealMaintenance(dealId) });
  };

  const ticketsError = ticketsQuery.isError;
  const maintenanceError = maintenanceQuery.isError;

  useEffect(() => {
    const raw = new URLSearchParams(location.search).get("tab");
    if (raw === "all" || raw === "support" || raw === "maintenance" || raw === "billing") {
      setTab(raw);
      if (raw === "support" || raw === "billing") setForm((p) => ({ ...p, type: raw }));
    }
  }, [location.search]);

  useEffect(() => {
    return onMessage((type, payload) => {
      if (type === "ticket_update" || type === "ticket_comment") {
        if (payload?.deal_id && payload.deal_id !== dealId) return;
        if (dealId) queryClient.invalidateQueries({ queryKey: portalKeys.dealTickets(dealId) });
        if (selectedTicketId) {
          queryClient.invalidateQueries({ queryKey: portalKeys.ticket(selectedTicketId) });
        }
      }
    });
  }, [onMessage, dealId, selectedTicketId, queryClient]);

  useEffect(() => {
    if (tab === "maintenance") {
      setSelectedTicketId(null);
      setCreateOpen(false);
    }
    if (tab === "support" || tab === "billing") {
      setForm((p) => ({ ...p, type: tab }));
    }
    if (tab !== "maintenance") {
      setSelectedMaintenanceId(null);
    }
  }, [tab]);

  const selectedMaintenance = useMemo(() => {
    if (!selectedMaintenanceId) return null;
    return maintenanceTickets.find((t) => t.id === selectedMaintenanceId) || null;
  }, [maintenanceTickets, selectedMaintenanceId]);

  const maintenanceStatusLabel = (status?: string) => {
    const s = (status || "").toLowerCase();
    if (s === "em análise" || s === "em analise") return "Em análise";
    if (s === "em andamento") return "Em andamento";
    if (s === "concluído" || s === "concluido") return "Concluído";
    if (s === "cancelado") return "Cancelado";
    return status || "-";
  };

  const maintenanceTimelineIndex = (status?: string) => {
    const s = (status || "").toLowerCase();
    if (s === "cancelado") return 0;
    if (s === "concluído" || s === "concluido") return 3;
    if (s === "em andamento") return 2;
    if (s === "em análise" || s === "em analise") return 1;
    return 0;
  };

  const onCreate = async () => {
    if (!dealId) return;
    setIsSubmitting(true);
    try {
      if (tab === "maintenance") {
        if (!maintenanceForm.description.trim()) {
          toast.error("Descreva o problema para solicitar manutenção.");
          return;
        }
        const keys: string[] = [];
        if (maintenanceFiles.length > 0) {
          for (const f of maintenanceFiles) {
            const urlData = await portalService.getDealMaintenanceAttachmentUploadURL(dealId, {
              file_name: f.name,
              content_type: f.type || "application/octet-stream",
              file_size: f.size,
            });
            const putResp = await fetch(urlData.upload_url, {
              method: "PUT",
              body: f,
              headers: { "Content-Type": f.type || "application/octet-stream" },
            });
            if (!putResp.ok) throw new Error(`PUT failed: ${putResp.status}`);
            await portalService.completeDealMaintenanceAttachmentUpload(dealId, { storage_key: urlData.storage_key });
            keys.push(urlData.storage_key);
          }
        }
        await createMaintenance.mutateAsync({
          type: maintenanceForm.type,
          category: maintenanceForm.category,
          priority: maintenanceForm.priority,
          description: maintenanceForm.description.trim(),
          attachments: keys,
        });
        toast.success("Solicitação de manutenção enviada!");
        setMaintenanceForm((prev) => ({ ...prev, description: "" }));
        for (const u of maintenancePreviews) URL.revokeObjectURL(u);
        setMaintenanceFiles([]);
        setMaintenancePreviews([]);
        setCreateOpen(false);
        return;
      }

      if (!form.subject.trim() || !form.description.trim()) {
        toast.error("Preencha assunto e descrição para abrir o chamado.");
        return;
      }
      const created = await createTicket.mutateAsync(form);
      toast.success("Chamado aberto com sucesso!");
      setForm((prev) => ({ ...prev, subject: "", description: "" }));
      setSelectedTicketId(created.id);
      setCreateOpen(false);
    } catch {
      toast.error("Não foi possível abrir o chamado.");
    } finally {
      setIsSubmitting(false);
    }
  };

  const filtered = useMemo(() => {
    const byType = (t: PortalTicket) => {
      if (tab === "all") return true;
      if (tab === "billing") return t.type === "billing";
      return t.type === tab;
    };
    const q = query.trim().toLowerCase();
    const byQuery = (t: PortalTicket) => {
      if (!q) return true;
      return (
        (t.subject || "").toLowerCase().includes(q) ||
        (t.category || "").toLowerCase().includes(q) ||
        (t.description || "").toLowerCase().includes(q)
      );
    };
    return tickets.filter((t) => byType(t) && byQuery(t));
  }, [tickets, tab, query]);

  const filteredMaintenance = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return maintenanceTickets;
    return maintenanceTickets.filter((t) => {
      return (
        (t.category || "").toLowerCase().includes(q) ||
        (t.type || "").toLowerCase().includes(q) ||
        (t.description || "").toLowerCase().includes(q) ||
        (t.status || "").toLowerCase().includes(q)
      );
    });
  }, [maintenanceTickets, query]);

  const sorted = useMemo(() => [...filtered].sort((a, b) => (a.created_at < b.created_at ? 1 : -1)), [filtered]);
  const sortedMaintenance = useMemo(
    () => [...filteredMaintenance].sort((a, b) => (a.created_at < b.created_at ? 1 : -1)),
    [filteredMaintenance],
  );

  const counts = useMemo(() => {
    const openTicketStatuses = new Set(["open", "assigned", "in_progress", "awaiting_client"]);
    const openTickets = tickets.filter((t) => openTicketStatuses.has((t.status || "").toLowerCase())).length;
    const openSupport = tickets.filter((t) => t.type === "support" && openTicketStatuses.has((t.status || "").toLowerCase())).length;
    const openBilling = tickets.filter((t) => t.type === "billing" && openTicketStatuses.has((t.status || "").toLowerCase())).length;
    const openMaintenance = maintenanceTickets.filter(
      (t) => (t.status || "").toLowerCase() !== "concluído" && (t.status || "").toLowerCase() !== "concluido",
    ).length;
    return {
      all: openTickets + openMaintenance,
      support: openSupport,
      billing: openBilling,
      maintenance: openMaintenance,
    };
  }, [tickets, maintenanceTickets]);

  const onSendComment = async () => {
    if (!selectedTicketId || isSendingComment) return;
    if (!newComment.trim()) return;
    const ticketId = selectedTicketId;
    const clientRequestId =
      typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
        ? crypto.randomUUID()
        : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    setIsSendingComment(true);
    try {
      const keys: string[] = [];
      if (commentFiles.length > 0) {
        for (const f of commentFiles) {
          const urlData = await portalService.getTicketAttachmentUploadURL(ticketId, {
            file_name: f.name,
            content_type: f.type || "application/octet-stream",
            file_size: f.size,
          });
          const putResp = await fetch(urlData.upload_url, {
            method: "PUT",
            body: f,
            headers: { "Content-Type": f.type || "application/octet-stream" },
          });
          if (!putResp.ok) throw new Error(`PUT failed: ${putResp.status}`);
          await portalService.completeTicketAttachmentUpload(ticketId, { storage_key: urlData.storage_key });
          keys.push(urlData.storage_key);
        }
      }
      await addComment.mutateAsync({
        content: newComment.trim(),
        attachments: keys,
        clientRequestId,
        targetTicketId: ticketId,
        targetDealId: dealId,
      });
      setNewComment("");
      for (const u of commentPreviews) URL.revokeObjectURL(u);
      setCommentFiles([]);
      setCommentPreviews([]);
    } catch {
      toast.error("Não foi possível enviar o comentário.");
    } finally {
      setIsSendingComment(false);
    }
  };

  if (isLoading) {
    return (
      <PortalPage className="flex flex-col gap-3 p-5">
        <Skeleton className="h-24 w-full rounded-2xl" />
        <Skeleton className="h-16 w-full rounded-xl" />
        <Skeleton className="h-16 w-full rounded-xl" />
      </PortalPage>
    );
  }

  if (!activeDeal) {
    return (
      <PortalPage className="px-5 pt-8">
        <h1 className="text-section font-bold text-foreground tracking-tight">Chamados</h1>
        <div className="mt-8">
          <EmptyState icon={Wrench} title="Nenhum negócio encontrado" message="Você ainda não tem negócios ativos para abrir chamados." />
        </div>
      </PortalPage>
    );
  }

  return (
    <PortalPage>
      <PortalPageHeader title={tab === "maintenance" ? "Manutenção" : copy.ticketsTitle} />

      <div className="px-5 mt-2">
        {deals.length > 1 && (
          <div className="mb-4">
            <DealSwitcher
              deals={deals}
              activeDealId={activeDealId}
              onSelect={setActiveDealId}
              title="Selecionar transação (chamados)"
            />
          </div>
        )}

        <Tabs value={tab} onValueChange={(v) => setTab(v as any)} className="w-full">
          <TabsList className="w-full justify-start overflow-x-auto">
            <TabsTrigger value="all" className="flex-none px-4">
              Todos <Badge variant="secondary">{counts.all}</Badge>
            </TabsTrigger>
            <TabsTrigger value="support" className="flex-none px-4">
              Suporte <Badge variant="secondary">{counts.support}</Badge>
            </TabsTrigger>
            <TabsTrigger value="maintenance" className="flex-none px-4">
              Manutenção <Badge variant="secondary">{counts.maintenance}</Badge>
            </TabsTrigger>
            <TabsTrigger value="billing" className="flex-none px-4">
              Financeiro <Badge variant="secondary">{counts.billing}</Badge>
            </TabsTrigger>
          </TabsList>
        </Tabs>

        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={tab === "maintenance" ? "Buscar solicitações..." : "Buscar chamados..."}
          className="mt-3 h-11"
        />

        <Button
          type="button"
          onClick={() => setCreateOpen((v) => !v)}
          disabled={isSubmitting || (tab === "maintenance" ? isMaintenanceLoading : isTicketsLoading)}
          className="mt-3 w-full"
          size="lg"
        >
          <Plus className="w-4 h-4" />
          {tab === "maintenance" ? "Nova solicitação" : "Novo chamado"}
        </Button>

        {createOpen && (
          <Card className="mt-4">
            <CardContent className="p-5">
              {tab === "maintenance" ? (
                <>
                  <div className="grid grid-cols-2 gap-3">
                    <Input
                      value={maintenanceForm.type || ""}
                      onChange={(e) => setMaintenanceForm((p) => ({ ...p, type: e.target.value }))}
                      placeholder="Tipo (ex.: Hidráulica)"
                      className="h-11"
                    />
                    <Input
                      value={maintenanceForm.category || ""}
                      onChange={(e) => setMaintenanceForm((p) => ({ ...p, category: e.target.value }))}
                      placeholder="Categoria (ex.: Banheiro)"
                      className="h-11"
                    />
                    <select
                      value={maintenanceForm.priority || "Media"}
                      onChange={(e) => setMaintenanceForm((p) => ({ ...p, priority: e.target.value }))}
                      className="h-11 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                    >
                      <option value="Baixa">Baixa</option>
                      <option value="Media">Média</option>
                      <option value="Alta">Alta</option>
                    </select>
                  </div>
                  <Textarea
                    value={maintenanceForm.description}
                    onChange={(e) => setMaintenanceForm((p) => ({ ...p, description: e.target.value }))}
                    placeholder="Descreva o problema e, se possível, quando percebeu..."
                    className="mt-3 min-h-[96px] resize-none"
                  />
                  <div className="mt-3">
                    <input
                      type="file"
                      accept="image/*"
                      multiple
                      onChange={(e) => {
                        const all = Array.from(e.target.files || []);
                        const files = all.slice(0, 5);
                        if (all.length > 5) toast.error("Envie no máximo 5 fotos por solicitação.");
                        for (const u of maintenancePreviews) URL.revokeObjectURL(u);
                        setMaintenanceFiles(files);
                        setMaintenancePreviews(files.map((f) => URL.createObjectURL(f)));
                      }}
                      className="block w-full text-sm text-muted-foreground file:mr-3 file:rounded-md file:border-0 file:bg-primary file:px-4 file:py-2 file:text-sm file:font-semibold file:text-primary-foreground"
                    />
                    {maintenancePreviews.length > 0 && (
                      <div className="mt-3 grid grid-cols-3 gap-2">
                        {maintenancePreviews.map((u) => (
                          <div key={u} className="overflow-hidden rounded-xl border bg-card">
                            <MediaImage src={u} alt="" className="w-full h-20 object-cover" />
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </>
              ) : (
                <>
                  <div className="grid grid-cols-2 gap-3">
                    {tab === "all" ? (
                      <select
                        value={form.type}
                        onChange={(e) => setForm((p) => ({ ...p, type: e.target.value }))}
                        className="h-11 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                      >
                        <option value="support">Suporte</option>
                        <option value="billing">Financeiro</option>
                        <option value="documentation">Documentação</option>
                        <option value="general">Geral</option>
                      </select>
                    ) : (
                      <div className="h-11 rounded-md border border-input px-3 text-sm text-foreground bg-muted flex items-center">
                        {tab === "billing" ? "Financeiro" : "Suporte"}
                      </div>
                    )}
                    <select
                      value={form.category}
                      onChange={(e) => setForm((p) => ({ ...p, category: e.target.value }))}
                      className="h-11 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                    >
                      <option value="general">Geral</option>
                      <option value="documentation">Documentação</option>
                      <option value="billing">Cobrança</option>
                      <option value="technical">Técnico</option>
                    </select>
                    <select
                      value={form.priority}
                      onChange={(e) => setForm((p) => ({ ...p, priority: e.target.value }))}
                      className="h-11 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                    >
                      <option value="low">Baixa</option>
                      <option value="medium">Média</option>
                      <option value="high">Alta</option>
                      <option value="urgent">Urgente</option>
                    </select>
                  </div>

                  <Input
                    value={form.subject}
                    onChange={(e) => setForm((p) => ({ ...p, subject: e.target.value }))}
                    placeholder="Assunto"
                    className="mt-3 h-11"
                  />

                  <Textarea
                    value={form.description}
                    onChange={(e) => setForm((p) => ({ ...p, description: e.target.value }))}
                    placeholder="Descreva o problema..."
                    className="mt-3 min-h-[96px] resize-none"
                  />
                </>
              )}

              <Button type="button" onClick={onCreate} disabled={isSubmitting} className="mt-4 w-full" size="lg">
                {isSubmitting ? "Enviando..." : tab === "maintenance" ? "Solicitar manutenção" : "Abrir chamado"}
              </Button>
            </CardContent>
          </Card>
        )}
      </div>

      {tab !== "maintenance" && selectedTicketId && selectedTicketQuery.isPending && (
        <section className="px-5 mt-6 space-y-3" aria-busy="true" aria-live="polite">
          <Skeleton className="h-28 w-full rounded-2xl" />
          <Skeleton className="h-40 w-full rounded-2xl" />
        </section>
      )}

      {tab !== "maintenance" && selectedTicket && (
        <section className="px-5 mt-6">
          <Card>
            <CardContent className="p-5">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="text-[16px] font-bold text-foreground">{selectedTicket.subject}</div>
                  <div className="text-xs text-muted-foreground mt-1">
                    {ticketCategoryLabel(selectedTicket.category)} · {ticketTypeLabel(selectedTicket.type)}
                  </div>
                  <div className="text-xs text-muted-foreground mt-1">Aberto em {formatDateBR(selectedTicket.created_at)}</div>
                </div>
                <div className="flex flex-col items-end gap-2">
                  <Badge variant={badgeVariantForTicketStatus(selectedTicket.status)}>{ticketStatusLabel(selectedTicket.status)}</Badge>
                  <Badge variant={badgeVariantForTicketPriority(selectedTicket.priority)}>{ticketPriorityLabel(selectedTicket.priority)}</Badge>
                </div>
              </div>

              <div className="mt-3 text-sm text-foreground whitespace-pre-wrap">{selectedTicket.description}</div>

              <div className="mt-4 border-t pt-4">
                <div className="text-label font-bold text-foreground">Comentários</div>
                <div className="mt-3 space-y-3">
                  {(selectedTicket.comments || []).map((c) => (
                    <div key={c.id} className="border rounded-xl p-3 bg-muted/30">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <Badge variant={c.author_type === "client" ? "secondary" : "outline"}>
                            {c.author_type === "client" ? "Cliente" : "Operador"}
                          </Badge>
                          <div className="text-xs font-semibold text-foreground">
                            {c.author_type === "client" ? "Você" : c.author_name || "Operador"}
                          </div>
                        </div>
                        <div className="text-caption text-muted-foreground">{new Date(c.created_at).toLocaleString("pt-BR")}</div>
                      </div>
                      <div className="mt-2 text-label text-foreground whitespace-pre-wrap">{c.content}</div>
                      {c.attachments && c.attachments.length > 0 && (
                        <div className="mt-3 grid grid-cols-3 gap-2">
                          {c.attachments.map((u) => (
                            <a key={u} href={getMediaUrl(u)} target="_blank" rel="noreferrer" className="block overflow-hidden rounded-xl border bg-card">
                              <MediaImage src={u} alt="" className="w-full h-20 object-cover" />
                            </a>
                          ))}
                        </div>
                      )}
                    </div>
                  ))}
                  {(selectedTicket.comments || []).length === 0 && (
                    <div className="text-label text-muted-foreground mt-3">Nenhum comentário ainda.</div>
                  )}
                </div>

                <div className="mt-4 flex gap-2">
                  <Input
                    value={newComment}
                    onChange={(e) => setNewComment(e.target.value)}
                    placeholder="Escreva um comentário..."
                    className="flex-1 h-11"
                    disabled={isSendingComment}
                  />
                  <Button
                    type="button"
                    onClick={onSendComment}
                    disabled={!newComment.trim()}
                    loading={isSendingComment}
                    loadingText="Enviando..."
                    className="h-11"
                  >
                    Enviar
                  </Button>
                </div>
                <div className="mt-3">
                  <input
                    type="file"
                    accept="image/*"
                    multiple
                    disabled={isSendingComment}
                    onChange={(e) => {
                      const files = Array.from(e.target.files || []);
                      for (const u of commentPreviews) URL.revokeObjectURL(u);
                      setCommentFiles(files);
                      setCommentPreviews(files.map((f) => URL.createObjectURL(f)));
                    }}
                    className="block w-full text-sm text-muted-foreground file:mr-3 file:rounded-md file:border-0 file:bg-primary file:px-4 file:py-2 file:text-sm file:font-semibold file:text-primary-foreground"
                  />
                  {commentPreviews.length > 0 && (
                    <div className="mt-3 grid grid-cols-3 gap-2">
                      {commentPreviews.map((u) => (
                        <div key={u} className="overflow-hidden rounded-xl border bg-card">
                          <MediaImage src={u} alt="" className="w-full h-20 object-cover" />
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>
        </section>
      )}

      {tab === "maintenance" && selectedMaintenance && (
        <section className="px-5 mt-6">
          <Card>
            <CardContent className="p-5">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="text-[16px] font-bold text-foreground">{selectedMaintenance.category || "Manutenção"}</div>
                  <div className="text-xs text-muted-foreground mt-1">
                    {selectedMaintenance.type} · {selectedMaintenance.priority}
                  </div>
                  <div className="text-xs text-muted-foreground mt-1">Aberto em {formatDateBR(selectedMaintenance.created_at)}</div>
                </div>
                <div className="flex flex-col items-end gap-2">
                  <Badge variant="outline">{maintenanceStatusLabel(selectedMaintenance.status)}</Badge>
                  <Button type="button" variant="ghost" size="sm" onClick={() => setSelectedMaintenanceId(null)}>
                    Fechar
                  </Button>
                </div>
              </div>

              <div className="mt-4">
                <div className="flex items-center justify-between">
                  {["Abertura", "Análise", "Agendamento", "Conclusão"].map((label, idx) => {
                    const current = maintenanceTimelineIndex(selectedMaintenance.status);
                    const done = idx <= current && maintenanceStatusLabel(selectedMaintenance.status) !== "Cancelado";
                    const active = idx === current && maintenanceStatusLabel(selectedMaintenance.status) !== "Cancelado";
                    return (
                      <div key={label} className="flex-1 flex flex-col items-center">
                        <div className={`w-3 h-3 rounded-full ${done ? "bg-primary" : "bg-muted"}`} />
                        <div className={`mt-2 text-caption font-semibold ${active ? "text-foreground" : "text-muted-foreground"}`}>
                          {label}
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>

              <div className="mt-4 text-sm text-foreground whitespace-pre-wrap">{selectedMaintenance.description}</div>

              {selectedMaintenance.attachments && selectedMaintenance.attachments.length > 0 && (
                <div className="mt-4 border-t pt-4">
                  <div className="text-label font-bold text-foreground">Fotos</div>
                  <div className="mt-3 grid grid-cols-3 gap-2">
                    {selectedMaintenance.attachments.map((u) => (
                      <a key={u} href={getMediaUrl(u)} target="_blank" rel="noreferrer" className="block overflow-hidden rounded-xl border bg-card">
                        <MediaImage src={u} alt="" className="w-full h-24 object-cover" />
                      </a>
                    ))}
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </section>
      )}

      <section className="px-5 mt-6 space-y-3">
        {tab === "maintenance" ? (
          <>
            {maintenanceError && (
              <PortalError
                title="Não foi possível carregar"
                description="Não foi possível carregar suas solicitações. Tente novamente."
                onRetry={reloadMaintenance}
              />
            )}

            {!maintenanceError && isMaintenanceLoading && (
              <div className="space-y-3">
                <Skeleton className="h-24 w-full rounded-2xl" />
                <Skeleton className="h-24 w-full rounded-2xl" />
              </div>
            )}

            {!maintenanceError &&
              !isMaintenanceLoading &&
              sortedMaintenance.map((t) => (
                <Card
                  key={t.id}
                  onClick={() => setSelectedMaintenanceId(t.id)}
                  className={`cursor-pointer ${selectedMaintenanceId === t.id ? "ring-2 ring-ring" : ""}`}
                >
                  <CardContent className="p-4 flex items-start justify-between gap-3">
                    <div>
                      <p className="text-sm font-bold text-foreground">{t.category || "Manutenção"}</p>
                      <p className="text-xs text-muted-foreground mt-1">
                        {t.type} · {t.priority} · Aberto em {formatDateBR(t.created_at)}
                      </p>
                      <p className="text-label text-foreground mt-2 whitespace-pre-wrap">{t.description}</p>
                      {t.attachments && t.attachments.length > 0 && (
                        <p className="text-xs text-muted-foreground mt-2">{t.attachments.length} foto(s)</p>
                      )}
                    </div>
                    <Badge variant="outline">{maintenanceStatusLabel(t.status)}</Badge>
                  </CardContent>
                </Card>
              ))}

            {!maintenanceError && !isMaintenanceLoading && sortedMaintenance.length === 0 && (
              <div className="mt-10">
                <EmptyState
                  icon={Wrench}
                  title="Nenhuma solicitação"
                  message={query.trim() ? "Nenhuma solicitação encontrada para esta busca." : "Nenhuma solicitação de manutenção encontrada."}
                />
              </div>
            )}
          </>
        ) : (
          <>
            {ticketsError && (
              <PortalError
                title="Não foi possível carregar"
                description="Não foi possível carregar seus chamados. Tente novamente."
                onRetry={reload}
              />
            )}

            {!ticketsError && isTicketsLoading && (
              <div className="space-y-3">
                <Skeleton className="h-20 w-full rounded-2xl" />
                <Skeleton className="h-20 w-full rounded-2xl" />
              </div>
            )}

            {!ticketsError &&
              !isTicketsLoading &&
              sorted.map((t) => (
                <Card
                  key={t.id}
                  onClick={() => {
                    if (!isSendingComment) setSelectedTicketId(t.id);
                  }}
                  aria-disabled={isSendingComment}
                  className={`${isSendingComment ? "cursor-wait opacity-70" : "cursor-pointer"} ${
                    selectedTicketId === t.id ? "ring-2 ring-ring" : ""
                  }`}
                >
                  <CardContent className="p-4 flex items-center justify-between gap-3">
                    <div>
                      <p className="text-sm font-bold text-foreground">{t.subject}</p>
                      <p className="text-xs text-muted-foreground mt-1">
                        {ticketCategoryLabel(t.category)} · {ticketTypeLabel(t.type)} · Aberto em {formatDateBR(t.created_at)}
                      </p>
                    </div>
                    <Badge variant={badgeVariantForTicketStatus(t.status)}>{ticketStatusLabel(t.status)}</Badge>
                  </CardContent>
                </Card>
              ))}

            {!ticketsError && !isTicketsLoading && sorted.length === 0 && (
              <div className="mt-10">
                <EmptyState
                  icon={Wrench}
                  title="Nenhum chamado"
                  message={query.trim() ? "Nenhum chamado encontrado para esta busca." : "Nenhum chamado encontrado."}
                />
              </div>
            )}
          </>
        )}
      </section>
    </PortalPage>
  );
}

import { useEffect, useMemo, useState } from "react";
import { ExpandableSidePanel } from "../ExpandableSidePanel";
import { Alert, AlertDescription, AlertTitle } from "../../ui/alert";
import { Badge } from "../../ui/badge";
import { Button } from "../../ui/button";
import { Checkbox } from "../../ui/checkbox";
import { Input, inputClassNames } from "../../ui/input";
import { PhoneInput } from "../../ui/phone-input";
import { DocumentInput } from "../../ui/document-input";
import { Label } from "../../ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../../ui/select";
import {
  AlertCircle,
  FileText,
  FolderOpen,
  Info,
  ListOrdered,
  Loader2,
  Paperclip,
  ShieldCheck,
  Trash2,
  Upload,
  Users,
} from "lucide-react";
import { SigningGroupBadge } from "./SigningGroupBadge";
import type { Contract, SendForSignatureRequest, Signatory, SignerAuthMethod } from "../../contracts/types";
import { formatCpf, formatPhoneBR, parseCpf, parsePhoneBR } from "../../utils/masks";
import { documentService, type DocumentDTO } from "../../services/document.service";

const AUTH_METHODS: { value: SignerAuthMethod; label: string; hint?: string }[] = [
  { value: "email", label: "E-mail (token)", hint: "Padrão. Sem custo extra." },
  { value: "whatsapp", label: "WhatsApp (token)", hint: "Útil para clientes que pouco abrem e-mail." },
  { value: "sms", label: "SMS (token)", hint: "Pode ter custo adicional pelo plano Clicksign." },
  { value: "selfie", label: "Selfie", hint: "Captura biometria pela câmera." },
  { value: "selfie+document", label: "Selfie + Documento oficial", hint: "Maior garantia jurídica. Custo extra (R$1,50–R$4,50)." },
];

const ROLE_LABEL: Record<string, string> = {
  inquilino: "Inquilino",
  proprietario: "Proprietário",
  fiador: "Fiador",
  locador: "Locador",
  locatario: "Locatário",
  comprador: "Comprador",
  vendedor: "Vendedor",
  avalista: "Avalista",
  testemunha: "Testemunha",
};

const METHODS_REQUIRING_CPF_AND_BIRTHDAY: SignerAuthMethod[] = ["selfie+document", "icp_brasil"];
const METHODS_REQUIRING_PHONE: SignerAuthMethod[] = ["whatsapp", "sms"];

const MAX_ATTACHMENTS = 10;
const MAX_INLINE_BYTES = 20 * 1024 * 1024;
const GROUP_NONE = 0;
const GROUP_OPTIONS = [GROUP_NONE, 1, 2, 3, 4, 5];

interface Props {
  open: boolean;
  onClose: () => void;
  contract: Contract;
  onSubmit: (req: SendForSignatureRequest, patches: Record<string, SignerPatch>) => Promise<void>;
}

export interface SignerPatch {
  documentation?: string;
  birthday?: string;
  phone_number?: string;
}

interface DraftSigner {
  documentation: string;
  birthday: string;
  phone: string;
}

interface InlineAttachmentDraft {
  id: string;
  filename: string;
  sizeBytes: number;
  contentBase64: string;
}

export function SendForSignatureModal({ open, onClose, contract, onSubmit }: Props) {
  const [authBySigner, setAuthBySigner] = useState<Record<string, SignerAuthMethod[]>>({});
  const [groupBySigner, setGroupBySigner] = useState<Record<string, number>>({});
  const [draftBySigner, setDraftBySigner] = useState<Record<string, DraftSigner>>({});
  const [availableDocs, setAvailableDocs] = useState<DocumentDTO[]>([]);
  const [selectedDocIds, setSelectedDocIds] = useState<Set<string>>(new Set());
  const [inlineFiles, setInlineFiles] = useState<InlineAttachmentDraft[]>([]);
  const [loadingDocs, setLoadingDocs] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    const defaults: Record<string, SignerAuthMethod[]> = {};
    const groups: Record<string, number> = {};
    const drafts: Record<string, DraftSigner> = {};
    for (const sig of contract.signatories) {
      defaults[sig.id] = sig.auth_methods?.length ? (sig.auth_methods as SignerAuthMethod[]) : ["email"];
      groups[sig.id] = sig.group ?? GROUP_NONE;
      drafts[sig.id] = {
        documentation: sig.documentation ? formatCpf(sig.documentation) : "",
        birthday: sig.birthday ?? "",
        phone: sig.phone_number ? formatPhoneBR(sig.phone_number) : "",
      };
    }
    setAuthBySigner(defaults);
    setGroupBySigner(groups);
    setDraftBySigner(drafts);
    setSelectedDocIds(new Set());
    setInlineFiles([]);
    setError(null);
    setSubmitting(false);
  }, [open, contract.signatories]);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    (async () => {
      setLoadingDocs(true);
      try {
        const fromContract = await documentService
          .getByContract(contract.id)
          .then((r) => r.data ?? [])
          .catch(() => []);
        const filtered = fromContract.filter(
          (d) => d.file_type === "application/pdf" && !(d.storage_key?.includes(`contrato-${contract.id}`) ?? false),
        );
        if (!cancelled) setAvailableDocs(filtered);
      } finally {
        if (!cancelled) setLoadingDocs(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, contract.id]);

  const toggleMethod = (signerId: string, method: SignerAuthMethod) => {
    setAuthBySigner((prev) => {
      const current = prev[signerId] ?? [];
      const next = current.includes(method) ? current.filter((m) => m !== method) : [...current, method];
      return { ...prev, [signerId]: next };
    });
  };

  const setGroup = (signerId: string, group: number) => {
    setGroupBySigner((prev) => ({ ...prev, [signerId]: group }));
  };

  const toggleAttachment = (docId: string) => {
    setSelectedDocIds((prev) => {
      const next = new Set(prev);
      if (next.has(docId)) {
        next.delete(docId);
      } else {
        next.add(docId);
      }
      return next;
    });
  };

  const updateDraft = (signerId: string, patch: Partial<DraftSigner>) => {
    setDraftBySigner((prev) => ({
      ...prev,
      [signerId]: { ...prev[signerId], ...patch },
    }));
  };

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? []);
    e.target.value = "";
    if (files.length === 0) return;
    const totalSlots = MAX_ATTACHMENTS - selectedDocIds.size - inlineFiles.length;
    if (files.length > totalSlots) {
      setError(`Limite de ${MAX_ATTACHMENTS} anexos por envelope.`);
      return;
    }
    const drafts: InlineAttachmentDraft[] = [];
    for (const f of files) {
      if (!f.name.toLowerCase().endsWith(".pdf") || f.type !== "application/pdf") {
        setError(`Apenas PDFs são aceitos: ${f.name}`);
        return;
      }
      if (f.size > MAX_INLINE_BYTES) {
        setError(`${f.name} excede 20 MB.`);
        return;
      }
      const base64 = await fileToBase64(f);
      drafts.push({
        id: `${f.name}-${f.size}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        filename: f.name,
        sizeBytes: f.size,
        contentBase64: base64,
      });
    }
    setError(null);
    setInlineFiles((prev) => [...prev, ...drafts]);
  };

  const removeInline = (id: string) => {
    setInlineFiles((prev) => prev.filter((f) => f.id !== id));
  };

  const totalAttachments = selectedDocIds.size + inlineFiles.length;

  const validation = useMemo(() => {
    const errors: Record<string, string[]> = {};
    for (const sig of contract.signatories) {
      const methods = authBySigner[sig.id] ?? [];
      const draft = draftBySigner[sig.id] ?? { documentation: "", birthday: "", phone: "" };
      const errs: string[] = [];
      if (methods.length === 0) errs.push("Selecione ao menos 1 método.");
      const requiresCpfBday = methods.some((m) => METHODS_REQUIRING_CPF_AND_BIRTHDAY.includes(m));
      if (requiresCpfBday) {
        if (parseCpf(draft.documentation).length !== 11) errs.push("Informe um CPF válido (11 dígitos).");
        if (!/^\d{4}-\d{2}-\d{2}$/.test(draft.birthday)) errs.push("Informe a data de nascimento (YYYY-MM-DD).");
      }
      const requiresPhone = methods.some((m) => METHODS_REQUIRING_PHONE.includes(m));
      if (requiresPhone && parsePhoneBR(draft.phone).length < 10) {
        errs.push("Informe um telefone válido (DDD + número).");
      }
      if (errs.length > 0) errors[sig.id] = errs;
    }
    return errors;
  }, [contract.signatories, authBySigner, draftBySigner]);

  const allValid = Object.keys(validation).length === 0;

  const groupSummary = useMemo(() => {
    const buckets: Record<number, Signatory[]> = {};
    for (const sig of contract.signatories) {
      const g = groupBySigner[sig.id] ?? GROUP_NONE;
      (buckets[g] ??= []).push(sig);
    }
    return buckets;
  }, [contract.signatories, groupBySigner]);

  const orderedGroupKeys = Object.keys(groupSummary)
    .map((k) => Number(k))
    .sort((a, b) => a - b);
  const hasOrdering = orderedGroupKeys.some((k) => k > 0);

  const handleSubmit = async () => {
    if (!allValid) return;
    if (totalAttachments > MAX_ATTACHMENTS) {
      setError(`Limite de ${MAX_ATTACHMENTS} anexos por envelope.`);
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const patches: Record<string, SignerPatch> = {};
      for (const sig of contract.signatories) {
        const draft = draftBySigner[sig.id];
        if (!draft) continue;
        const cpf = parseCpf(draft.documentation);
        const phone = parsePhoneBR(draft.phone);
        const patch: SignerPatch = {};
        if (cpf && cpf !== sig.documentation) patch.documentation = cpf;
        if (draft.birthday && draft.birthday !== sig.birthday) patch.birthday = draft.birthday;
        if (phone && phone !== sig.phone_number) patch.phone_number = phone;
        if (Object.keys(patch).length > 0) patches[sig.id] = patch;
      }
      const signerGroups: Record<string, number> = {};
      for (const [id, group] of Object.entries(groupBySigner)) {
        if (group && group > 0) signerGroups[id] = group;
      }
      await onSubmit(
        {
          signer_auth: authBySigner,
          signer_groups: Object.keys(signerGroups).length > 0 ? signerGroups : undefined,
          attachment_document_ids: selectedDocIds.size > 0 ? [...selectedDocIds] : undefined,
          inline_attachments:
            inlineFiles.length > 0 ? inlineFiles.map((f) => ({ filename: f.filename, content_base64: f.contentBase64 })) : undefined,
        },
        patches,
      );
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Falha ao enviar para assinatura");
    } finally {
      setSubmitting(false);
    }
  };

  const totalAttachmentsLabel = totalAttachments === 0 ? "Nenhum anexo selecionado" : `${totalAttachments} de ${MAX_ATTACHMENTS} anexos`;

  const noSigners = contract.signatories.length === 0;

  return (
    <ExpandableSidePanel
      open={open}
      onClose={onClose}
      defaultSize="2xl"
      expandedSize="full"
      zIndex={70}
      renderHeader={({ expandButton, closeButton }) => (
        <div className="flex-none px-6 py-5 border-b border-border bg-card">
          <div className="flex items-start justify-between gap-3">
            <div className="flex items-start gap-3 min-w-0">
              <div className="p-2.5 rounded-xl bg-primary-50 dark:bg-primary-900/30 shrink-0">
                <ShieldCheck className="w-5 h-5 text-primary-600 dark:text-primary-400" />
              </div>
              <div className="min-w-0">
                <h2 className="text-lg font-bold text-foreground">Enviar contrato para assinatura</h2>
                <p className="text-sm text-muted-foreground mt-0.5">
                  Configure autenticação, ordem de assinatura e documentos antes de despachar o envelope pela Clicksign.
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2 shrink-0">
              {expandButton}
              {closeButton}
            </div>
          </div>
        </div>
      )}
    >
      <div className="flex-1 min-h-0 overflow-y-auto px-6 py-5 space-y-5 bg-muted/20">
        {noSigners ? (
          <Alert variant="warning">
            <AlertCircle />
            <AlertTitle>Nenhum signatário cadastrado</AlertTitle>
            <AlertDescription>Adicione signatários ao contrato antes de enviá-lo para assinatura.</AlertDescription>
          </Alert>
        ) : (
          <>
            <section className="rounded-2xl border border-border bg-card p-5">
              <header className="flex items-start justify-between gap-3 mb-4">
                <div className="flex items-start gap-3">
                  <div className="p-2 rounded-xl bg-primary-50 dark:bg-primary-900/30">
                    <Users className="w-4 h-4 text-primary-600 dark:text-primary-400" />
                  </div>
                  <div>
                    <h3 className="text-sm font-bold text-foreground">Signatários</h3>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      Métodos de autenticação e ordem de assinatura por signatário.
                    </p>
                  </div>
                </div>
                <Badge variant="secondary">
                  {contract.signatories.length} pessoa{contract.signatories.length === 1 ? "" : "s"}
                </Badge>
              </header>

              {hasOrdering && (
                <div className="mb-4 rounded-xl border border-status-info/20 bg-status-info-soft px-3 py-2 flex items-start gap-2">
                  <Info className="w-3.5 h-3.5 text-status-info mt-0.5 shrink-0" />
                  <div className="text-xs text-status-info">
                    <div className="font-bold">Assinatura sequencial ativa.</div>
                    <div className="text-status-info/85 mt-0.5">
                      Grupos com o mesmo número assinam em paralelo. Grupos maiores só são notificados após o anterior concluir.
                    </div>
                  </div>
                </div>
              )}

              <div className="space-y-3">
                {contract.signatories.map((sig: Signatory) => {
                  const selected = authBySigner[sig.id] ?? [];
                  const draft = draftBySigner[sig.id] ?? { documentation: "", birthday: "", phone: "" };
                  const errs = validation[sig.id] ?? [];
                  const requiresCpfBday = selected.some((m) => METHODS_REQUIRING_CPF_AND_BIRTHDAY.includes(m));
                  const requiresPhone = selected.some((m) => METHODS_REQUIRING_PHONE.includes(m));
                  const group = groupBySigner[sig.id] ?? GROUP_NONE;

                  return (
                    <article key={sig.id} className="rounded-xl border border-border bg-card overflow-hidden">
                      <header className="px-4 py-3 bg-muted/30 border-b border-border flex items-center justify-between gap-3">
                        <div className="min-w-0">
                          <div className="text-sm font-bold text-foreground truncate">{sig.name || sig.email}</div>
                          <div className="text-xs text-muted-foreground truncate mt-0.5 flex items-center gap-2">
                            <span className="truncate">{sig.email}</span>
                            <span className="text-border">•</span>
                            <Badge variant="outline" className="font-medium">
                              {ROLE_LABEL[sig.role] ?? sig.role}
                            </Badge>
                          </div>
                        </div>
                        <SigningGroupBadge group={group} />
                      </header>

                      <div className="grid gap-4 p-4 sm:grid-cols-[1fr_1.4fr]">
                        <div className="space-y-2">
                          <Label htmlFor={`group-${sig.id}`} className="flex items-center gap-1.5">
                            <ListOrdered className="w-3.5 h-3.5 text-muted-foreground" />
                            Ordem de assinatura
                          </Label>
                          <Select value={String(group)} onValueChange={(v) => setGroup(sig.id, Number(v))}>
                            <SelectTrigger id={`group-${sig.id}`} className="w-full">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {GROUP_OPTIONS.map((g) => (
                                <SelectItem key={g} value={String(g)}>
                                  {g === 0 ? "Sem ordem (paralelo)" : `Grupo ${g}`}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                          <p className="text-xs text-muted-foreground">
                            {group === 0 ? "Será notificado junto com os demais sem ordem." : `Assina junto com outros do Grupo ${group}.`}
                          </p>
                        </div>

                        <div className="space-y-2">
                          <Label className="flex items-center gap-1.5">
                            <ShieldCheck className="w-3.5 h-3.5 text-muted-foreground" />
                            Métodos de autenticação
                          </Label>
                          <div className="grid gap-1.5">
                            {AUTH_METHODS.map((m) => {
                              const id = `auth-${sig.id}-${m.value}`;
                              const checked = selected.includes(m.value);
                              return (
                                <label
                                  key={m.value}
                                  htmlFor={id}
                                  className={[
                                    "flex cursor-pointer items-start gap-2 rounded-lg border px-3 py-2 text-sm transition-colors",
                                    checked
                                      ? "border-primary-200 bg-primary-50/60 dark:border-primary-700 dark:bg-primary-900/20"
                                      : "border-border bg-card hover:bg-muted/40",
                                  ].join(" ")}
                                >
                                  <Checkbox id={id} checked={checked} onCheckedChange={() => toggleMethod(sig.id, m.value)} />
                                  <div className="min-w-0">
                                    <div className="font-medium text-foreground">{m.label}</div>
                                    {m.hint && <p className="text-xs text-muted-foreground mt-0.5">{m.hint}</p>}
                                  </div>
                                </label>
                              );
                            })}
                          </div>
                        </div>
                      </div>

                      {(requiresCpfBday || requiresPhone) && (
                        <div className="border-t border-dashed border-border px-4 py-3 bg-muted/20">
                          <p className="text-xs text-muted-foreground mb-2 flex items-center gap-1.5">
                            <Info className="w-3.5 h-3.5" />
                            Os métodos selecionados exigem dados adicionais — eles serão salvos no contrato antes do envio.
                          </p>
                          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                            {requiresCpfBday && (
                              <>
                                <div className="space-y-1">
                                  <Label htmlFor={`cpf-${sig.id}`}>CPF</Label>
                                  <DocumentInput
                                    id={`cpf-${sig.id}`}
                                    mode="cpf"
                                    value={draft.documentation}
                                    onChange={(v) => updateDraft(sig.id, { documentation: formatCpf(v) })}
                                    placeholder="000.000.000-00"
                                    className={inputClassNames}
                                  />
                                </div>
                                <div className="space-y-1">
                                  <Label htmlFor={`bday-${sig.id}`}>Data de nascimento</Label>
                                  <Input
                                    id={`bday-${sig.id}`}
                                    type="date"
                                    value={draft.birthday}
                                    onChange={(e) => updateDraft(sig.id, { birthday: e.target.value })}
                                  />
                                </div>
                              </>
                            )}
                            {requiresPhone && (
                              <div className="space-y-1">
                                <Label htmlFor={`phone-${sig.id}`}>Telefone</Label>
                                <PhoneInput
                                  id={`phone-${sig.id}`}
                                  value={draft.phone}
                                  onChange={(v) => updateDraft(sig.id, { phone: formatPhoneBR(v) })}
                                  placeholder="(11) 99999-0000"
                                  className={inputClassNames}
                                />
                              </div>
                            )}
                          </div>
                        </div>
                      )}

                      {errs.length > 0 && (
                        <div className="border-t border-status-critical/20 bg-status-critical-soft px-4 py-2">
                          <ul className="space-y-0.5 text-xs text-status-critical">
                            {errs.map((e) => (
                              <li key={e}>• {e}</li>
                            ))}
                          </ul>
                        </div>
                      )}
                    </article>
                  );
                })}
              </div>
            </section>

            <section className="rounded-2xl border border-border bg-card p-5">
              <header className="flex items-start justify-between gap-3 mb-4">
                <div className="flex items-start gap-3">
                  <div className="p-2 rounded-xl bg-primary-50 dark:bg-primary-900/30">
                    <Paperclip className="w-4 h-4 text-primary-600 dark:text-primary-400" />
                  </div>
                  <div>
                    <h3 className="text-sm font-bold text-foreground">Documentos do envelope</h3>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      O contrato principal já é incluído. Adicione procurações, RGs ou comprovantes se necessário.
                    </p>
                  </div>
                </div>
                <Badge variant={totalAttachments === 0 ? "outline" : "secondary"}>{totalAttachmentsLabel}</Badge>
              </header>

              <div className="grid gap-4 lg:grid-cols-2">
                <div className="rounded-xl border border-border bg-muted/20 p-4">
                  <div className="flex items-center gap-2 mb-3">
                    <FolderOpen className="w-4 h-4 text-muted-foreground" />
                    <h4 className="text-xs font-bold text-foreground">Da pasta do contrato</h4>
                  </div>
                  {loadingDocs ? (
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      <Loader2 className="w-3.5 h-3.5 animate-spin" /> Carregando documentos…
                    </div>
                  ) : availableDocs.length === 0 ? (
                    <p className="text-xs text-muted-foreground">Nenhum PDF adicional cadastrado neste contrato.</p>
                  ) : (
                    <ul className="space-y-1.5">
                      {availableDocs.map((d) => {
                        const id = `attach-${d.id}`;
                        const checked = selectedDocIds.has(d.id);
                        return (
                          <li key={d.id}>
                            <label
                              htmlFor={id}
                              className={[
                                "flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 transition-colors",
                                checked
                                  ? "border-primary-200 bg-primary-50/60 dark:border-primary-700 dark:bg-primary-900/20"
                                  : "border-border bg-card hover:bg-muted/40",
                              ].join(" ")}
                            >
                              <Checkbox id={id} checked={checked} onCheckedChange={() => toggleAttachment(d.id)} />
                              <FileText className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
                              <div className="min-w-0 flex-1">
                                <div className="text-sm font-medium text-foreground truncate">{d.name}</div>
                                <div className="text-xs text-muted-foreground">{formatFileSize(d.file_size)}</div>
                              </div>
                            </label>
                          </li>
                        );
                      })}
                    </ul>
                  )}
                </div>

                <div className="rounded-xl border border-border bg-muted/20 p-4">
                  <div className="flex items-center gap-2 mb-3">
                    <Upload className="w-4 h-4 text-muted-foreground" />
                    <h4 className="text-xs font-bold text-foreground">Enviar do computador</h4>
                  </div>
                  <label
                    className={[
                      "flex flex-col items-center justify-center gap-1.5 rounded-lg border border-dashed border-border bg-card px-4 py-6 text-center cursor-pointer transition-colors hover:bg-muted/40",
                      totalAttachments >= MAX_ATTACHMENTS && "opacity-50 cursor-not-allowed",
                    ]
                      .filter(Boolean)
                      .join(" ")}
                  >
                    <Upload className="w-5 h-5 text-muted-foreground" />
                    <span className="text-sm font-medium text-foreground">Selecionar PDFs</span>
                    <span className="text-xs text-muted-foreground">Até 20 MB cada, máx. {MAX_ATTACHMENTS} anexos</span>
                    <input
                      type="file"
                      accept="application/pdf"
                      multiple
                      className="hidden"
                      onChange={handleFileChange}
                      disabled={totalAttachments >= MAX_ATTACHMENTS}
                    />
                  </label>

                  {inlineFiles.length > 0 && (
                    <ul className="mt-3 space-y-1.5">
                      {inlineFiles.map((f) => (
                        <li key={f.id} className="flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2">
                          <FileText className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
                          <div className="min-w-0 flex-1">
                            <div className="text-sm font-medium text-foreground truncate">{f.filename}</div>
                            <div className="text-xs text-muted-foreground">{formatFileSize(f.sizeBytes)}</div>
                          </div>
                          <button
                            type="button"
                            onClick={() => removeInline(f.id)}
                            className="p-1.5 rounded-md text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft transition-colors"
                            aria-label={`Remover ${f.filename}`}
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </div>
            </section>

            {error && (
              <Alert variant="critical">
                <AlertCircle />
                <AlertTitle>Não foi possível enviar</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
          </>
        )}
      </div>

      <div className="flex-none px-6 py-4 border-t border-border bg-card">
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground">
            {noSigners
              ? "Adicione signatários para liberar o envio."
              : allValid
                ? "Pronto para enviar pela Clicksign."
                : "Resolva as pendências dos signatários para continuar."}
          </p>
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={onClose} disabled={submitting}>
              Cancelar
            </Button>
            <Button onClick={handleSubmit} disabled={!allValid || submitting || noSigners}>
              {submitting ? <Loader2 className="mr-2 w-4 h-4 animate-spin" /> : null}
              Enviar pela Clicksign
            </Button>
          </div>
        </div>
      </div>
    </ExpandableSidePanel>
  );
}

function formatFileSize(bytes: number): string {
  if (!bytes) return "—";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error ?? new Error("read failed"));
    reader.onload = () => {
      const result = typeof reader.result === "string" ? reader.result : "";
      const comma = result.indexOf(",");
      resolve(comma >= 0 ? result.slice(comma + 1) : result);
    };
    reader.readAsDataURL(file);
  });
}


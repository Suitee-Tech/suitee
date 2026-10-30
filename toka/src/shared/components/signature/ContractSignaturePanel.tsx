import { useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  Ban,
  CheckCircle2,
  Clock,
  Eye,
  FileSignature,
  Hourglass,
  Pencil,
  Plus,
  RefreshCw,
  Send,
  X,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import {
  ContractStatus,
  type Contract,
  type SignatureEnvelope,
  type SignatureEnvelopeSigner,
} from "../../contracts/types";
import type { Signer } from "../../services/deal.service";
import { useContractSignatureEnvelope } from "../../../features/esteira/components/esteira/useContractSignatureEnvelope";
import { SignerFormModal, type SignerFormValues } from "./SignerFormModal";
import { SigningGroupBadge } from "./SigningGroupBadge";
import {
  activeGroupLabel,
  signatureProgress,
  signatureProgressLabel,
  signerTurnState,
  waitingTurnLabel,
} from "./signingProgress";
import { Button } from "../../ui/button";
import { signedContractFilename, useSignedContractDocument } from "./useSignedContractDocument";

/**
 * Painel de assinatura do contrato.
 *
 * A tela de contrato já criava o envelope (`SendForSignatureModal`), mas depois
 * disso não oferecia nada: não dava para ver o estado de cada signatário, nem
 * reenviar, sincronizar, cancelar ou corrigir quem assina. O aditivo tornava
 * isso visível — ele nasce em rascunho e precisa ser assinado para valer, e não
 * havia caminho na própria tela do aditivo.
 *
 * Reusa o que a esteira digital já tinha para envelope de contrato
 * (`useContractSignatureEnvelope`) em vez de recriar a lógica de ciclo de vida.
 */
interface ContractSignaturePanelProps {
  contract: Contract;
  envelope: SignatureEnvelope | null;
  loading?: boolean;
  onEnvelopeChange: (envelope: SignatureEnvelope | null) => void;
  onContractChange: (contract: Contract) => void;
  onRefresh: () => void;
}

const STATUS_META: Record<string, { label: string; className: string; Icon: typeof CheckCircle2 }> = {
  SIGNED: { label: "Assinou", className: "text-status-ok", Icon: CheckCircle2 },
  VIEWED: { label: "Visualizou — aguardando", className: "text-status-info", Icon: Eye },
  PENDING: { label: "Aguardando assinatura", className: "text-status-warning", Icon: Clock },
  // Grupo posterior: pendente, mas ainda NÃO convidado (o provedor só convida o
  // grupo da vez). Antes caía no mesmo "Aguardando assinatura" de quem já
  // recebeu o pedido.
  WAITING_TURN: { label: "Aguardando a vez", className: "text-muted-foreground", Icon: Clock },
  REFUSED: { label: "Recusou assinar", className: "text-status-critical", Icon: X },
};

const ENVELOPE_STATUS_LABEL: Record<string, string> = {
  PENDING: "Preparando",
  DRAFT: "Rascunho",
  RUNNING: "Aguardando assinatura",
  PARTIALLY_SIGNED: "Parcialmente assinado",
  FINISHED: "Assinado",
  REFUSED: "Recusado",
  CANCELED: "Cancelado",
  EXPIRED: "Expirado",
  ERROR: "Erro",
};

export function ContractSignaturePanel({
  contract,
  envelope,
  loading = false,
  onEnvelopeChange,
  onContractChange,
  onRefresh,
}: ContractSignaturePanelProps) {
  const [signerEditing, setSignerEditing] = useState<SignatureEnvelopeSigner | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [cancelConfirmOpen, setCancelConfirmOpen] = useState(false);

  // O controlador da esteira espera `Signer[]`. O contrato guarda `signatories`,
  // que tem os mesmos campos de identidade — a conversão evita depender do deal.
  const dealSigners = useMemo<Signer[]>(
    () =>
      (contract.signatories ?? []).map((signatory) => ({
        id: signatory.id,
        name: signatory.name,
        email: signatory.email,
        role: signatory.role,
        status: (signatory.signed_at || signatory.signature ? "SIGNED" : "PENDING") as Signer["status"],
        signed_at: signatory.signed_at,
      })),
    [contract.signatories],
  );

  const handleContractChange = useCallback(
    (next: Contract) => onContractChange(next),
    [onContractChange],
  );

  const signature = useContractSignatureEnvelope({
    contract,
    dealSigners,
    onContractChange: handleContractChange,
    onRefresh,
  });

  // Mantém a página informada quando o envelope muda (cancelar, sincronizar,
  // editar signatário), para o cabeçalho e a lista de partes acompanharem.
  // Em efeito, não no render: notificar durante o render é efeito colateral em
  // fase de render e pode re-renderizar em laço.
  const envelopeFromHook = signature.envelope;
  const activeEnvelope = envelopeFromHook ?? envelope;
  useEffect(() => {
    if (envelopeFromHook && envelopeFromHook !== envelope) {
      onEnvelopeChange(envelopeFromHook);
    }
  }, [envelopeFromHook, envelope, onEnvelopeChange]);

  const { envelopeLoading, envelopeLoadFailed } = signature;
  const hasEnvelope = Boolean(activeEnvelope?.external_envelope_id);
  const isFinished = activeEnvelope?.status === "FINISHED" || contract.status === "SIGNED";
  // Estado parcial: quem já assinou, quem está com a vez e quem ainda nem foi
  // convidado (grupo posterior). Sem isso a tela mostrava "Aguardando
  // assinatura" para todo mundo, incluindo quem não recebeu pedido nenhum.
  const progress = useMemo(() => signatureProgress(activeEnvelope), [activeEnvelope]);
  const grupoAtivo = activeGroupLabel(progress);
  const canCancel = hasEnvelope && !isFinished && activeEnvelope?.status !== "CANCELED";

  // O documento assinado SÓ existe quando a assinatura terminou.
  //
  // A URL do contrato é um campo próprio e pode estar preenchida por um estado
  // anterior (o contrato pode ter sido regenerado, ou uma tentativa encerrada
  // pode ter deixado o campo). Confiar nele fazia o painel anunciar "Documento
  // assinado disponível" e oferecer download num envelope que ainda aguardava
  // assinatura — contradizendo o próprio selo de status ao lado. O estado do
  // envelope manda.
  const assinaturaConcluida = activeEnvelope
    ? activeEnvelope.status === "FINISHED"
    : contract.status === ContractStatus.SIGNED;

  // A URL do PDF assinado NÃO vem do contrato/envelope: aquele campo guarda um
  // link pré-assinado com validade de ~5 minutos, e reusá-lo depois devolve o
  // XML de "Request has expired" do storage. O que decide se o documento existe
  // é o estado da assinatura; o link é pedido na hora do clique.
  const signedDocumentUrl = assinaturaConcluida ? "fresh" : undefined;
  const signedDocument = useSignedContractDocument(signedDocumentUrl ? contract.id : null);

  // Um contrato já assinado pode não ter o registro do envelope acessível pela
  // API (o envelope terminou antes, ou o registro não é devolvido). Nesses casos
  // "nenhum envelope criado" é mentira: o documento assinado e as assinaturas
  // estão ali ao lado. A mensagem só aparece quando de fato não há nada.
  const showNoEnvelopeNotice =
    !activeEnvelope && !envelopeLoading && !signedDocumentUrl && !isFinished;

  const signersById = useMemo(() => {
    const map = new Map<string, SignatureEnvelopeSigner>();
    for (const signer of activeEnvelope?.signers ?? []) {
      if (signer.signer_id) map.set(signer.signer_id, signer);
    }
    return map;
  }, [activeEnvelope?.signers]);

  const signersByEmail = useMemo(() => {
    const map = new Map<string, SignatureEnvelopeSigner>();
    for (const signer of activeEnvelope?.signers ?? []) {
      if (signer.email) map.set(signer.email.toLowerCase(), signer);
    }
    return map;
  }, [activeEnvelope?.signers]);

  const handleEditSubmit = async (values: SignerFormValues) => {
    await signature.handleEditSignerSubmit(values);
    setSignerEditing(null);
  };

  const handleAddSubmit = async (values: SignerFormValues) => {
    await signature.handleAddSignerToEnvelope(values);
    setAddOpen(false);
  };

  return (
    <div className="space-y-6">
      <div className="bg-card rounded-xl border border-border p-6 shadow-sm">
        <div className="mb-5 flex flex-wrap items-start justify-between gap-4">
          <div className="flex items-start gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary-50">
              <FileSignature className="h-5 w-5 text-primary-600" />
            </div>
            <div>
              <h3 className="text-lg font-bold text-foreground">Assinatura</h3>
              <p className="text-sm text-muted-foreground">
                Envelope de assinatura eletrônica e situação de cada signatário.
              </p>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {activeEnvelope && (
              <span
                className={clsx(
                  "inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-bold",
                  isFinished
                    ? "border-status-ok/20 bg-status-ok-soft text-status-ok"
                    : activeEnvelope.status === "ERROR" || activeEnvelope.status === "REFUSED"
                      ? "border-status-critical/20 bg-status-critical-soft text-status-critical"
                      : activeEnvelope.status === "PARTIALLY_SIGNED"
                        ? "border-status-info/20 bg-status-info-soft text-status-info"
                        : "border-border bg-muted text-muted-foreground",
                )}
              >
                {isFinished ? <CheckCircle2 className="h-3.5 w-3.5" /> : <Hourglass className="h-3.5 w-3.5" />}
                {ENVELOPE_STATUS_LABEL[activeEnvelope.status] ?? activeEnvelope.status}
                {!isFinished && progress.total > 0 && ` · ${signatureProgressLabel(progress)}`}
              </span>
            )}
            <Button
              variant="outline"
              size="sm"
              onClick={signature.handleReconcile}
              disabled={!hasEnvelope || signature.reconcileBusy || envelopeLoading}
            >
              <RefreshCw className={clsx("mr-1.5 h-3.5 w-3.5", signature.reconcileBusy && "animate-spin")} />
              Sincronizar
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={signature.handleResendAll}
              disabled={!hasEnvelope || signature.resendAllBusy || isFinished}
            >
              <Send className="mr-1.5 h-3.5 w-3.5" />
              Reenviar
            </Button>
            {canCancel && (
              <Button variant="outline" size="sm" onClick={() => setCancelConfirmOpen(true)}>
                <Ban className="mr-1.5 h-3.5 w-3.5" />
                Cancelar
              </Button>
            )}
          </div>
        </div>

        {envelopeLoadFailed && (
          <div className="mb-4 flex items-center gap-2 rounded-lg border border-status-warning/30 bg-status-warning-soft p-3 text-sm text-status-warning">
            <AlertTriangle className="h-4 w-4 shrink-0" />
            Não foi possível carregar o envelope agora.
          </div>
        )}

        {activeEnvelope?.last_error && (
          <div className="mb-4 flex items-start gap-2 rounded-lg border border-status-critical/30 bg-status-critical-soft p-3 text-sm text-status-critical">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            <span>{activeEnvelope.last_error}</span>
          </div>
        )}

        {showNoEnvelopeNotice && (
          <div className="mb-4 rounded-lg border border-border bg-muted p-4 text-sm text-muted-foreground">
            Nenhum envelope de assinatura para este contrato. Em contratos em rascunho, o envio às partes fica na aba{" "}
            <strong>Documentos</strong>.
          </div>
        )}

        {signedDocumentUrl && (
          <div className="mb-4 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-status-ok/30 bg-status-ok-soft p-3 text-sm">
            <span className="font-medium text-status-ok">Documento assinado disponível.</span>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-auto p-0 font-medium text-status-ok underline hover:bg-transparent"
              loading={signedDocument.busy === "download"}
              onClick={() => void signedDocument.download(signedContractFilename(contract))}
            >
              Baixar PDF assinado
            </Button>
          </div>
        )}

        {(progress.total > 0 || progress.hasSigningOrder) && (
          <div className="mb-4 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
            <span>{progress.total > 0 ? `Progresso: ${signatureProgressLabel(progress)}` : "Nenhum signatário no envelope."}</span>
            {grupoAtivo && (
              <span className="flex items-center gap-2">
                <span>Assinatura sequencial:</span>
                {progress.activeGroup > 0 ? (
                  <SigningGroupBadge group={progress.activeGroup} active suffix="em andamento" />
                ) : (
                  <span className="font-bold text-status-ok">{grupoAtivo}</span>
                )}
              </span>
            )}
          </div>
        )}

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {(contract.signatories ?? []).map((signatory) => {
            const envelopeSigner =
              signersById.get(signatory.id) ?? signersByEmail.get((signatory.email ?? "").toLowerCase());
            const rawStatus = envelopeSigner?.status ?? (signatory.signed_at || signatory.signature ? "SIGNED" : "PENDING");
            const signerGroup = envelopeSigner?.group && envelopeSigner.group > 0 ? envelopeSigner.group : (signatory.group ?? 0);
            const turn = signerTurnState(activeEnvelope, { status: rawStatus, group: signerGroup });
            const meta = STATUS_META[turn] ?? STATUS_META[rawStatus] ?? STATUS_META.PENDING;
            const label = turn === "WAITING_TURN" ? waitingTurnLabel(signerGroup, progress.activeGroup) : meta.label;
            const { Icon } = meta;
            return (
              <div key={signatory.id} className="rounded-lg border border-border bg-muted p-4">
                <div className="mb-2 flex items-start justify-between gap-2">
                  <span className="inline-block rounded bg-primary-50 px-2 py-1 text-xs font-semibold uppercase tracking-wider text-primary-600">
                    {signatory.role}
                  </span>
                  {envelopeSigner && !isFinished && (
                    <button
                      type="button"
                      onClick={() => setSignerEditing(envelopeSigner)}
                      className="text-muted-foreground transition-colors hover:text-foreground"
                      title="Editar dados do signatário"
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </button>
                  )}
                </div>
                <p className="text-lg font-bold text-foreground">{signatory.name}</p>
                <p className="text-sm text-muted-foreground">{signatory.email}</p>
                {signerGroup > 0 && (
                  <div className="mt-2">
                    <SigningGroupBadge
                      group={signerGroup}
                      active={turn === "SIGNING"}
                      suffix={turn === "WAITING_TURN" ? "aguardando" : undefined}
                    />
                  </div>
                )}
                <div className="mt-3 border-t border-border pt-3">
                  <p className={clsx("flex items-center gap-1 text-xs font-medium", meta.className)}>
                    <Icon className="h-3 w-3" />
                    {label}
                    {signatory.signed_at && turn === "SIGNED" ? ` em ${new Date(signatory.signed_at).toLocaleString("pt-BR")}` : ""}
                  </p>
                  {turn === "WAITING_TURN" && (
                    <p className="mt-1 text-xs text-muted-foreground">
                      Só é notificado quando o Grupo {Math.max(progress.activeGroup, 1)} concluir.
                    </p>
                  )}
                </div>
              </div>
            );
          })}
        </div>

        {hasEnvelope && !isFinished && (
          <button
            type="button"
            onClick={() => setAddOpen(true)}
            className="mt-4 inline-flex items-center gap-1.5 text-sm font-medium text-primary-600 hover:underline"
          >
            <Plus className="h-4 w-4" />
            Adicionar signatário ao envelope
          </button>
        )}
      </div>

      <SignerFormModal
        mode="edit"
        open={Boolean(signerEditing)}
        onClose={() => setSignerEditing(null)}
        onSubmit={handleEditSubmit}
        initial={
          signerEditing
            ? {
                name: signerEditing.name,
                email: signerEditing.email,
                phone_number: signerEditing.phone_number,
                documentation: signerEditing.documentation,
                birthday: signerEditing.birthday,
              }
            : undefined
        }
      />

      <SignerFormModal
        mode="add"
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onSubmit={handleAddSubmit}
      />

      {cancelConfirmOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="w-full max-w-md rounded-2xl border border-border bg-card p-6 shadow-xl">
            <h3 className="text-lg font-bold text-foreground">Cancelar assinatura</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              O envelope será cancelado e os signatários não poderão mais assinar. Esta ação não pode ser desfeita.
            </p>
            <div className="mt-6 flex justify-end gap-3">
              <Button variant="outline" onClick={() => setCancelConfirmOpen(false)}>
                Voltar
              </Button>
              <Button
                variant="destructive"
                onClick={async () => {
                  await signature.handleCancel();
                  setCancelConfirmOpen(false);
                }}
                disabled={signature.cancelBusy}
              >
                Cancelar assinatura
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

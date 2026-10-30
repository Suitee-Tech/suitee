import { useParams, useNavigate, useSearchParams } from "react-router";
import { motion, AnimatePresence } from "motion/react";
import {
  AlertCircle,
  ArrowLeft,
  ChevronRight,
  FileText,
  User,
  Calendar,
  DollarSign,
  Download,
  Eye,
  CheckCircle2,
  Clock,
  Briefcase,
  Percent,
  History,
  PenTool,
  Plus,
  Upload,
  X,
  ShieldAlert,
  TrendingUp,
  Building2,
  FileSignature,
  Crown,
} from "lucide-react";
import { useMemo, useState, useEffect } from "react";
import { clsx } from "clsx";
import { contractService } from "../services/contract.service";
import { signatureService } from "../services/signature.service";
import { documentService, DocumentDTO, CreateDocumentDTO, DocumentCategory } from "../services/document.service";
import { unwrapApiData } from "../services/api";
import { ContractStatus, type Contract, type SignatureEnvelope, type SignatureSignerStatus } from "../contracts/types";
import { SendForSignatureModal } from "../components/signature/SendForSignatureModal";
import { ContractSignaturePanel } from "../components/signature/ContractSignaturePanel";
import {
  isEnvelopeTerminal,
  isSignatureInFlight,
  signatureProgress,
  signatureProgressLabel,
} from "../components/signature/signingProgress";
import { SuperDocEditor } from "@superdoc-dev/react";
import "@superdoc-dev/react/style.css";
import { base64ToFile } from "../utils/docx";
import { toast } from "sonner";
import { invoiceService } from "../services/invoice.service";
import { chargeService, type Charge } from "../services/charge.service";
import type { Invoice } from "../finance/types";
import { formatISODateBR } from "../utils/format";
import { normalizeChargeStatus } from "../utils/charge";
import { buildContractPlanRows, summarizeContractPlan, type ContractPlanRow } from "../lib/finance/contractPlan";
import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/button";
import {
  signedContractFilename,
  useSignedContractDocument,
} from "../components/signature/useSignedContractDocument";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "../ui/dialog";
import { Spinner } from "../ui/spinner";
import { useFocusMode } from "../context/FocusModeContext";
import { useSubscriptionAccess } from "../context/SubscriptionAccessContext";
import { TerminateContractModal } from "../components/ContractLifecycleModals";
import { ReadjustContractPanel } from "../components/ReadjustContractPanel";
import { AmendDocumentPanel } from "../components/AmendDocumentPanel";
import { ContractRentalTab } from "../components/ContractRentalTab";
import { BillingPlanWizard } from "../components/BillingPlanWizard";

export function ContractDetails() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { focusMode } = useFocusMode();
  const { hasFeature } = useSubscriptionAccess();
  const canUseSignature = hasFeature("electronic_signature");

  const [contract, setContract] = useState<Contract | null>(null);
  // O PDF assinado é sempre pedido na hora do clique: o campo
  // `signed_document_url` guarda um link pré-assinado que expira em minutos.
  const signedDocument = useSignedContractDocument(contract?.id);
  const [deal, setDeal] = useState<any>(null);
  const [invoices, setInvoices] = useState<Invoice[]>([]);
  const [charges, setCharges] = useState<Charge[]>([]);
  const [planLoading, setPlanLoading] = useState(false);
  const [documents, setDocuments] = useState<DocumentDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchParams] = useSearchParams();
  const [activeTab, setActiveTab] = useState<"geral" | "locacao" | "financeiro" | "assinatura" | "documentos">(() => searchParams.get("aba") === "locacao" ? "locacao" : "geral");
  const [showBillingPlanWizard, setShowBillingPlanWizard] = useState(false);
  const [showAddDoc, setShowAddDoc] = useState(false);
  const [addingDoc, setAddingDoc] = useState(false);
  const [newDoc, setNewDoc] = useState<CreateDocumentDTO>({
    contract_id: "",
    name: "",
    category: "outros",
    file_type: "application/pdf",
  });
  const [showSendModal, setShowSendModal] = useState(false);
  const [envelope, setEnvelope] = useState<SignatureEnvelope | null>(null);
  const [signedFileUrl, setSignedFileUrl] = useState<string | null>(null);
  const [showTerminate, setShowTerminate] = useState(false);
  const [showReadjust, setShowReadjust] = useState(false);
  const [showEditDocument, setShowEditDocument] = useState(false);
  const [showAmend, setShowAmend] = useState(false);

  const openSignatureFlow = () => {
    if (canUseSignature) setShowSendModal(true);
    else navigate("/assinatura");
  };

  useEffect(() => {
    if (id) {
      loadContract(id);
    }
  }, [id]);

  // Progresso do envelope (quem já assinou, de quantos). O contrato só diz
  // "parcial"; o número por signatário está no envelope, que é o que a página
  // passa a acompanhar durante todo o estado parcial.
  const progresso = useMemo(() => signatureProgress(envelope), [envelope]);

  useEffect(() => {
    // A assinatura em andamento inclui o estado PARCIAL: o contrato vira
    // PARTIALLY_SIGNED no primeiro signatário e continua assim até o último. A
    // condição antiga (só PENDING_SIGNATURE) parava o polling exatamente aí — a
    // tela congelava em "1 de 2" e nunca via a assinatura concluir.
    if (!contract || !isSignatureInFlight(contract.status)) return;
    let cancelled = false;
    const tick = async () => {
      try {
        const env = await signatureService.getByContract(contract.id);
        if (cancelled) return;
        setEnvelope(env);
        if (isEnvelopeTerminal(env?.status)) {
          loadContract(contract.id);
        }
      } catch (err) {
        console.warn("envelope poll failed", err);
      }
    };
    tick();
    const interval = window.setInterval(tick, 15000);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, [contract?.id, contract?.status]);

  const loadContract = async (contractId: string) => {
    try {
      setLoading(true);
      const data = await contractService.getById(contractId);
      setContract(data);

      contractService
        .getSignedFileUrl(contractId)
        .then((res) => setSignedFileUrl(res?.url ?? null))
        .catch(() => setSignedFileUrl(null));

      if (data.deal_id) {
        import("../services/deal.service").then(({ dealService }) => {
          dealService
            .getById(data.deal_id as string)
            .then((res) => setDeal(res.data.data))
            .catch(console.error);
        });

        invoiceService.list(undefined, data.deal_id as string).then(setInvoices).catch(console.error);
      }

      // Locação é cobrada por `charges` vinculadas ao contrato — não por
      // invoices do negócio. Sem esta leitura o plano financeiro da locação
      // aparecia vazio mesmo com cobranças emitidas e pagas.
      const rental = (data.type || "").toLowerCase() === "locacao";
      if (rental) {
        setPlanLoading(true);
        chargeService
          .getAll({ contract_id: contractId, limit: 200 })
          .then((res) => setCharges(((res as { data?: Charge[] })?.data ?? []) as Charge[]))
          .catch((error) => {
            console.error("Falha ao carregar as cobranças da locação", error);
            setCharges([]);
          })
          .finally(() => setPlanLoading(false));
      } else {
        setCharges([]);
      }

      try {
        const docRes = await documentService.getByContract(contractId);
        const docs = unwrapApiData<DocumentDTO[]>(docRes) ?? [];
        setDocuments(Array.isArray(docs) ? docs : []);
      } catch {
        setDocuments([]);
      }
    } catch (error) {
      console.error("Failed to load contract", error);
      toast.error("Erro ao carregar contrato");
    } finally {
      setLoading(false);
    }
  };

  const handleAddDocument = async () => {
    if (!contract || !newDoc.name) return;
    setAddingDoc(true);
    try {
      const res = await documentService.create({ ...newDoc, contract_id: contract.id });
      const created = unwrapApiData<DocumentDTO>(res);
      if (created) {
        setDocuments((prev) => [...prev, created]);
        toast.success("Documento adicionado!");
      }
      setShowAddDoc(false);
      setNewDoc({ contract_id: "", name: "", category: "outros", file_type: "application/pdf" });
    } catch {
      toast.error("Erro ao adicionar documento.");
    } finally {
      setAddingDoc(false);
    }
  };

  const handleDeleteDocument = async (docId: string) => {
    try {
      await documentService.delete(docId);
      setDocuments((prev) => prev.filter((d) => d.id !== docId));
      toast.success("Documento removido.");
    } catch {
      toast.error("Erro ao remover documento.");
    }
  };

  const formatFileSize = (bytes: number) => {
    if (bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
  };

  const CATEGORY_LABELS: Record<string, string> = {
    contrato: "Contrato",
    vistoria: "Vistoria",
    financeiro: "Financeiro",
    juridico: "Jurídico",
    outros: "Outros",
  };

  const STATUS_COLORS: Record<string, { bg: string; text: string; border: string }> = {
    pendente: { bg: "bg-status-warning-soft", text: "text-status-warning", border: "border-status-warning/30" },
    assinado: { bg: "bg-status-ok-soft", text: "text-status-ok", border: "border-status-ok/30" },
    expirado: { bg: "bg-muted", text: "text-muted-foreground", border: "border-border" },
    cancelado: { bg: "bg-status-critical-soft", text: "text-status-critical", border: "border-status-critical/30" },
  };

  const handleSendForSignature = async (
    req: { signer_auth: Record<string, any[]>; locale?: string },
    patches: Record<string, { documentation?: string; birthday?: string; phone_number?: string }>,
  ) => {
    if (!contract) return;
    if (Object.keys(patches).length > 0) {
      const updatedSignatories = contract.signatories.map((s) => {
        const p = patches[s.id];
        if (!p) return s;
        return {
          ...s,
          documentation: p.documentation ?? s.documentation,
          birthday: p.birthday ?? s.birthday,
          phone_number: p.phone_number ?? s.phone_number,
        };
      });
      await contractService.update(contract.id, { signatories: updatedSignatories } as any);
    }
    try {
      await contractService.sendForSignature(contract.id, req as any);
      toast.success("Contrato enviado para Clicksign — aguardando assinaturas.");
      loadContract(contract.id);
    } catch (err: unknown) {
      // O envio é recusado com 409 quando o contrato já foi enviado
      // (`signature_locked_at` preenchido) ou saiu de rascunho. Antes a promise
      // rejeitava sem tratamento e o operador só via "409 (Conflict)" no
      // console, sem saber que o contrato já estava em assinatura.
      const status = (err as { response?: { status?: number } })?.response?.status;
      const message = (err as { response?: { data?: { message?: string } } })?.response?.data?.message;
      if (status === 409) {
        toast.error(
          message || "Este contrato já foi enviado para assinatura. Cancele o envelope antes de enviar novamente.",
        );
        // Recarrega para a tela refletir o estado real (envelope em andamento).
        loadContract(contract.id);
      } else {
        toast.error(message || "Não foi possível enviar para assinatura.");
      }
    }
  };

  // Derivados do contrato precisam ficar antes dos returns condicionais:
  // um hook depois de um `return` muda a ordem das chamadas entre renders.
  const isRentalContract = (contract?.type || "").toLowerCase() === "locacao";
  // Locação lê `charges` do contrato; venda lê `invoices` do negócio.
  const planRows = useMemo<ContractPlanRow[]>(
    () =>
      buildContractPlanRows({
        source: isRentalContract ? "rent" : "sale",
        charges,
        invoices,
      }),
    [isRentalContract, charges, invoices],
  );

  if (loading) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center p-8 bg-muted">
        <Spinner className="size-8 text-primary mb-4" />
        <p className="text-muted-foreground">Carregando contrato...</p>
      </div>
    );
  }

  if (!contract) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center p-8 bg-muted text-muted-foreground">
        <FileText className="w-16 h-16 mb-4 text-muted-foreground/60" />
        <h2 className="text-xl font-bold text-foreground mb-2">Contrato não encontrado</h2>
        <p className="mb-6">O contrato que você está procurando não existe ou foi removido.</p>
        <Button onClick={() => navigate(-1)}>
          <ArrowLeft className="w-4 h-4" />
          Voltar
        </Button>
      </div>
    );
  }

  const formatCurrency = (val: number) => {
    return new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(val);
  };

  const contractValue = deal?.value || 0;
  const commissionPct = deal?.commission_pct || 0;
  const commissionValue = (contractValue * commissionPct) / 100;
  const clientName =
    contract.signatories.find((s) =>
      ["comprador", "inquilino", "locatario", "cliente", "client"].includes((s.role || "").toLowerCase()),
    )?.name || "Cliente";

  const handleExportPdf = async () => {
    try {
      const signed = await contractService.getSignedFileUrl(contract.id);
      if (!signed?.url) {
        toast.error("O PDF assinado fica disponível após a conclusão da assinatura.");
        return;
      }
      window.open(signed.url, "_blank", "noopener");
    } catch {
      toast.error("Não foi possível exportar o PDF do contrato.");
    }
  };

  const StatusBadge = ({ status }: { status: string }) => {
    const isAtivo = status === ContractStatus.ACTIVE || status === ContractStatus.SIGNED;
    const isEncerrado = status === ContractStatus.TERMINATED || status === ContractStatus.EXPIRED;
    // O parcial é meio caminho: já houve assinatura, falta o resto. Ficava com o
    // tom de alerta do "aguardando" e sem rótulo (a tela mostrava o enum cru).
    const isParcial = status === ContractStatus.PARTIALLY_SIGNED;

    let colorClass = "bg-status-warning-soft text-status-warning border-status-warning/30";
    let dotClass = "bg-status-warning";

    const STATUS_LABELS: Record<string, string> = {
      DRAFT: "Rascunho",
      PENDING_SIGNATURE: "Aguardando Assinatura",
      PARTIALLY_SIGNED: "Parcialmente assinado",
      SIGNED: "Assinado",
      ACTIVE: "Vigente",
      EXPIRED: "Expirado",
      TERMINATED: "Encerrado",
    };

    if (isAtivo) {
      colorClass = "bg-status-ok-soft text-status-ok border-status-ok/30";
      dotClass = "bg-status-ok";
    } else if (isParcial) {
      colorClass = "bg-status-info-soft text-status-info border-status-info/30";
      dotClass = "bg-status-info";
    } else if (isEncerrado) {
      colorClass = "bg-muted text-foreground border-border";
      dotClass = "bg-muted-foreground/30";
    }

    return (
      <span
        className={clsx(
          "px-2.5 py-1 text-xs font-semibold rounded-full border flex items-center gap-1.5 w-fit",
          colorClass,
        )}
      >
        <span className={clsx("w-1.5 h-1.5 rounded-full", dotClass)}></span>
        {STATUS_LABELS[status] ?? status}
      </span>
    );
  };

  return (
    <div className="flex-1 overflow-auto bg-muted font-sans">
      <div className="max-w-7xl mx-auto p-6 lg:p-8 space-y-6">
        <div className="flex flex-col md:flex-row md:items-start justify-between gap-4">
          <div className="flex items-start gap-4">
            <Button variant="outline" size="icon" onClick={() => navigate(-1)} className="mt-1">
              <ArrowLeft className="w-5 h-5" />
            </Button>
            <div>
              <div className="flex items-center gap-3 mb-1 flex-wrap">
                <h1 className="text-2xl md:text-3xl font-bold tracking-tight text-foreground">
                  Contrato {contract.display_id || contract.id.substring(0, 8)}
                </h1>
                <StatusBadge status={contract.status} />
                {contract.amends_contract_id && (
                  <span className="px-2.5 py-1 text-xs font-semibold rounded-full border bg-primary-50 text-primary-700 border-primary-200 flex items-center gap-1.5">
                    <FileSignature className="w-3 h-3" />
                    Aditivo
                  </span>
                )}
              </div>
              <p className="text-muted-foreground text-sm flex items-center gap-2 flex-wrap">
                <Calendar className="w-4 h-4" />
                Criado em {new Date(contract.created_at).toLocaleDateString("pt-BR")}
                {contract.amends_contract_id && (
                  <>
                    {" · "}
                    <a
                      href={`/contratos/${contract.amends_contract_id}`}
                      className="text-primary-600 hover:text-primary-700 underline"
                    >
                      Ver contrato original
                    </a>
                  </>
                )}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <Button variant="outline" onClick={handleExportPdf}>
              <Download className="w-4 h-4" />
              Exportar PDF
            </Button>
            {contract.status === ContractStatus.DRAFT && (
              <Button onClick={openSignatureFlow} variant={canUseSignature ? "default" : "outline"}>
                {canUseSignature ? <PenTool className="w-4 h-4" /> : <Crown className="w-4 h-4" />}
                {canUseSignature ? "Enviar para Assinatura" : "Assinatura digital Pro"}
              </Button>
            )}
            {isSignatureInFlight(contract.status) && contract.status !== ContractStatus.DRAFT && (
              <span className="px-4 py-2 bg-status-warning-soft text-status-warning border border-status-warning/30 rounded-lg font-medium flex items-center gap-2">
                <Clock className="w-4 h-4" />
                Aguardando assinaturas
                {progresso.total > 0 && ` · ${signatureProgressLabel(progresso)}`}
              </span>
            )}
            {(contract.status === ContractStatus.SIGNED || contract.status === ContractStatus.ACTIVE) && (
              <>
                {isRentalContract && (
                  <Button variant="outline" onClick={() => setShowReadjust(true)} title="Aplicar reajuste no valor mensal">
                    <TrendingUp className="w-4 h-4 text-primary-600" />
                    Reajustar
                  </Button>
                )}
                {!contract.amends_contract_id && (
                  <Button
                    variant="outline"
                    onClick={() => setShowAmend(true)}
                    title="Criar aditivo (novo contrato com termos atualizados)"
                  >
                    <FileSignature className="w-4 h-4 text-primary-600" />
                    Aditivo
                  </Button>
                )}
                <Button
                  variant="outline"
                  onClick={() => setShowTerminate(true)}
                  className="border-status-critical/30 text-status-critical hover:bg-status-critical-soft hover:text-status-critical"
                  title="Encerrar contrato"
                >
                  <ShieldAlert className="w-4 h-4" />
                  Encerrar
                </Button>
              </>
            )}
            {contract.status === ContractStatus.TERMINATED && (
              <span className="px-4 py-2 bg-muted text-muted-foreground border border-border rounded-lg font-medium flex items-center gap-2">
                <ShieldAlert className="w-4 h-4" />
                Encerrado
              </span>
            )}
          </div>
        </div>

        <AnimatePresence>
          {!focusMode && (
            <motion.div
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: "auto" }}
              exit={{ opacity: 0, height: 0 }}
              transition={{ duration: 0.3, ease: "easeInOut" }}
            >
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
                <div className="bg-card rounded-xl border border-border p-5 shadow-sm">
                  <div className="flex items-center gap-3 mb-2">
                    <div className="p-2 bg-primary-50 text-primary-600 rounded-lg">
                      <DollarSign className="w-5 h-5" />
                    </div>
                    <h3 className="text-sm font-medium text-muted-foreground">Valor do Negócio</h3>
                  </div>
                  <p className="text-2xl font-bold text-foreground">{formatCurrency(contractValue)}</p>
                </div>
                <div className="bg-card rounded-xl border border-border p-5 shadow-sm">
                  <div className="flex items-center gap-3 mb-2">
                    <div className="p-2 bg-status-ok-soft text-status-ok rounded-lg">
                      <Percent className="w-5 h-5" />
                    </div>
                    <h3 className="text-sm font-medium text-muted-foreground">Comissão ({commissionPct}%)</h3>
                  </div>
                  <p className="text-2xl font-bold text-foreground">{formatCurrency(commissionValue)}</p>
                </div>
                <div className="bg-card rounded-xl border border-border p-5 shadow-sm">
                  <div className="flex items-center gap-3 mb-2">
                    <div className="p-2 bg-status-info-soft text-status-info rounded-lg">
                      <User className="w-5 h-5" />
                    </div>
                    <h3 className="text-sm font-medium text-muted-foreground">Cliente Principal</h3>
                  </div>
                  <p className="text-lg font-bold text-foreground truncate">{clientName}</p>
                </div>
                <div className="bg-card rounded-xl border border-border p-5 shadow-sm">
                  <div className="flex items-center gap-3 mb-2">
                    <div className="p-2 bg-stage-contract/10 text-stage-contract rounded-lg">
                      <Briefcase className="w-5 h-5" />
                    </div>
                    <h3 className="text-sm font-medium text-muted-foreground">Deal ID</h3>
                  </div>
                  <p
                    className="text-lg font-bold text-foreground truncate"
                    title={contract.deal_id}
                  >
                    {contract.deal_id ? contract.deal_id.substring(0, 8) + "..." : "N/A"}
                  </p>
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>

        <div className="flex items-center gap-6 border-b border-border px-2 overflow-x-auto">
          {[
            { id: "geral", label: "Visão Geral", icon: Eye },
            { id: "locacao", label: "Locação", icon: Building2 },
            { id: "financeiro", label: "Financeiro", icon: DollarSign },
            { id: "assinatura", label: "Assinatura", icon: FileSignature },
            { id: "documentos", label: "Documentos", icon: FileText },
          ]
            .filter((tab) => tab.id !== "locacao" || isRentalContract)
            .map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id as any)}
                className={clsx(
                  "py-3 font-medium text-sm flex items-center gap-2 border-b-2 transition-colors",
                  activeTab === tab.id
                    ? "border-primary-600 text-primary-600"
                    : "border-transparent text-muted-foreground hover:text-foreground hover:border-border",
                )}
              >
                <tab.icon className="w-4 h-4" />
                {tab.label}
              </button>
            ))}
        </div>

        <div className="pt-2">
          {activeTab === "geral" && (
            <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              <div className="lg:col-span-2 space-y-6">
                <div className="bg-card rounded-xl border border-border p-6 shadow-sm">
                  <h3 className="text-lg font-bold text-foreground mb-4 flex items-center gap-2">
                    <FileText className="w-5 h-5 text-primary-600" />
                    Conteúdo do Contrato
                  </h3>
                  {contract.status === ContractStatus.DRAFT && !contract.signature_locked_at && (
                    <Button variant="outline" className="mb-4" onClick={() => setShowEditDocument(true)}>
                      <PenTool className="w-4 h-4" /> Editar documento
                    </Button>
                  )}
                  <div className="border border-border rounded-lg overflow-auto h-[520px]">
                    {signedFileUrl ? (
                      <iframe src={signedFileUrl} title="Contrato assinado" className="w-full h-full" />
                    ) : contract.content ? (
                      <SuperDocEditor
                        document={base64ToFile(contract.content, "contrato.docx")}
                        user={{ name: "Operador", email: "operador@suitee.com" }}
                        isInternal
                        hideToolbar
                        documentMode="viewing"
                      />
                    ) : (
                      <p className="p-4 text-sm text-muted-foreground">Conteúdo do contrato indisponível.</p>
                    )}
                  </div>
                </div>

                <div className="bg-card rounded-xl border border-border p-6 shadow-sm">
                  <h3 className="text-lg font-bold text-foreground mb-4 flex items-center gap-2">
                    <User className="w-5 h-5 text-primary-600" />
                    Partes Envolvidas (Signatários)
                  </h3>
                  {envelope && (
                    <div className="mb-4 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-primary-100 bg-primary-50 p-3 text-sm">
                      <div>
                        <span className="font-medium text-primary-900">Clicksign · {envelope.environment}</span>
                        <span className="ml-2 text-primary-800">Status do envelope: {envelope.status}</span>
                        {envelope.last_error && <span className="ml-2 text-red-700">· {envelope.last_error}</span>}
                      </div>
                      {/*
                        Só oferece o PDF assinado quando a assinatura terminou. O
                        campo do contrato pode estar preenchido por estado anterior
                        (regeneração, tentativa encerrada) e oferecer o download num
                        envelope em andamento contradiz o status exibido ao lado.
                      */}
                      {envelope.status === "FINISHED" ? (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-auto p-0 text-primary-700 underline hover:bg-transparent"
                          loading={signedDocument.busy === "download"}
                          onClick={() => void signedDocument.download(signedContractFilename(contract))}
                        >
                          Baixar PDF assinado
                        </Button>
                      ) : null}
                    </div>
                  )}
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    {contract.signatories.map((sig) => {
                      const envSigner = envelope?.signers.find((s) => s.signer_id === sig.id);
                      const status: SignatureSignerStatus | undefined = envSigner?.status;
                      return (
                        <div key={sig.id} className="p-4 border border-border rounded-lg bg-muted">
                          <span className="text-xs font-semibold text-primary-600 bg-primary-50 px-2 py-1 rounded uppercase tracking-wider mb-2 inline-block">
                            {sig.role}
                          </span>
                          <p className="font-bold text-foreground text-lg">{sig.name}</p>
                          <p className="text-sm text-muted-foreground">Email: {sig.email}</p>
                          {sig.auth_methods && sig.auth_methods.length > 0 && (
                            <p className="mt-1 text-xs text-muted-foreground">Auth: {sig.auth_methods.join(", ")}</p>
                          )}
                          <div className="mt-3 pt-3 border-t border-border">
                            {sig.signed_at || status === "SIGNED" ? (
                              <p className="text-xs text-status-ok flex items-center gap-1 font-medium">
                                <CheckCircle2 className="w-3 h-3" />
                                Assinado{sig.signed_at ? ` em ${new Date(sig.signed_at).toLocaleString("pt-BR")}` : ""}
                              </p>
                            ) : status === "REFUSED" ? (
                              <p className="text-xs text-status-critical flex items-center gap-1 font-medium">
                                <X className="w-3 h-3" />
                                Recusou assinar
                              </p>
                            ) : (
                              <p className="text-xs text-status-warning flex items-center gap-1 font-medium">
                                <Clock className="w-3 h-3" />
                                {status === "VIEWED" ? "Visualizou — aguardando assinatura" : "Aguardando assinatura"}
                              </p>
                            )}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>

              <div className="lg:col-span-1">
                <div className="bg-card rounded-xl border border-border p-6 shadow-sm sticky top-6">
                  <h3 className="text-lg font-bold text-foreground mb-6 flex items-center gap-2">
                    <History className="w-5 h-5 text-primary-600" />
                    Histórico do Contrato
                  </h3>
                  <div className="space-y-6">
                    <div className="relative pl-6 border-l-2 border-border last:border-transparent">
                      <div className="absolute -left-[9px] top-0 w-4 h-4 rounded-full border-2 border-card bg-primary-500" />
                      <div className="-mt-1.5">
                        <p className="text-sm font-medium text-foreground">Contrato Criado</p>
                        <p className="text-xs text-muted-foreground mt-1 flex items-center gap-1">
                          <Clock className="w-3 h-3" />
                          {new Date(contract.created_at).toLocaleString("pt-BR")}
                        </p>
                      </div>
                    </div>
                    {contract.signatories
                      .filter((s) => s.signed_at)
                      .map((s) => (
                        <div key={s.id} className="relative pl-6 border-l-2 border-border last:border-transparent">
                          <div className="absolute -left-[9px] top-0 w-4 h-4 rounded-full border-2 border-card bg-status-ok" />
                          <div className="-mt-1.5">
                            <p className="text-sm font-medium text-foreground">Assinado por {s.name}</p>
                            <p className="text-xs text-muted-foreground mt-1 flex items-center gap-1">
                              <Clock className="w-3 h-3" />
                              {s.signed_at && new Date(s.signed_at).toLocaleString("pt-BR")}
                            </p>
                          </div>
                        </div>
                      ))}
                  </div>
                </div>
              </div>
            </motion.div>
          )}

          {activeTab === "locacao" && <ContractRentalTab contract={contract} />}

          {activeTab === "financeiro" && (
            <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
              {contract.termination && (
                <div className="rounded-xl border border-status-critical/30 bg-status-critical-soft p-5">
                  <div className="flex items-start gap-3">
                    <ShieldAlert className="w-5 h-5 text-status-critical mt-0.5 flex-shrink-0" />
                    <div className="flex-1 min-w-0 space-y-1 text-sm">
                      <p className="font-bold text-status-critical">
                        Contrato encerrado em {new Date(contract.termination.terminated_at).toLocaleDateString("pt-BR")}
                      </p>
                      {contract.termination.reason && <p className="text-foreground">Motivo: {contract.termination.reason}</p>}
                      <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mt-2">
                        {(contract.termination.early_termination_fee || 0) > 0 && (
                          <div>
                            <p className="text-xs text-muted-foreground uppercase tracking-wider">Multa</p>
                            <p className="font-bold text-foreground">{formatCurrency(contract.termination.early_termination_fee || 0)}</p>
                          </div>
                        )}
                        {(contract.termination.security_deposit_return || 0) > 0 && (
                          <div>
                            <p className="text-xs text-muted-foreground uppercase tracking-wider">Caução devolvida</p>
                            <p className="font-bold text-foreground">{formatCurrency(contract.termination.security_deposit_return || 0)}</p>
                          </div>
                        )}
                        {(contract.termination.security_deposit_deductions || 0) > 0 && (
                          <div>
                            <p className="text-xs text-muted-foreground uppercase tracking-wider">Descontos</p>
                            <p className="font-bold text-foreground">{formatCurrency(contract.termination.security_deposit_deductions || 0)}</p>
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              )}

              <div className="bg-card rounded-xl border border-border shadow-sm overflow-hidden">
                <div className="p-6 border-b border-border bg-muted flex items-center justify-between gap-3">
                  <h3 className="text-lg font-bold text-foreground flex items-center gap-2">
                    <DollarSign className="w-5 h-5 text-primary-600" />
                    {isRentalContract ? "Plano financeiro da locação" : "Condições de Pagamento"}
                  </h3>
                  <div className="flex flex-wrap items-center gap-2">
                    {isRentalContract && (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => navigate(`/locacao?contract=${contract.id}&tab=financeiro&setup=1`)}
                      >
                        Configurar plano-base
                      </Button>
                    )}
                    <Button variant="outline" size="sm" onClick={() => setShowBillingPlanWizard(true)}>
                      {isRentalContract ? "Composição mensal" : "Configurar plano"}
                    </Button>
                  </div>
                </div>
                {isRentalContract && charges.length > 0 && <RentalPlanSummary charges={charges} />}

                {planRows.length === 0 ? (
                  planLoading ? (
                    <div className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
                      <Spinner className="w-4 h-4" /> Carregando o plano financeiro…
                    </div>
                  ) : (
                    <EmptyState
                      icon={DollarSign}
                      title={isRentalContract ? "Nenhuma cobrança emitida" : "Nenhuma parcela gerada"}
                      message={isRentalContract
                        ? "Configure calendário, primeiro aluguel e composição antes de emitir as cobranças da locação."
                        : "Ainda não foram geradas parcelas para este contrato."}
                      className="border-none bg-transparent my-6"
                    />
                  )
                ) : (
                  <div className="overflow-x-auto">
                    <table className="w-full text-left text-sm whitespace-nowrap">
                      <thead className="bg-card text-muted-foreground border-b border-border">
                        <tr>
                          <th className="px-6 py-4 font-medium">{isRentalContract ? "Competência" : "Parcela"}</th>
                          <th className="px-6 py-4 font-medium">Vencimento</th>
                          <th className="px-6 py-4 font-medium">Valor</th>
                          <th className="px-6 py-4 font-medium">Status</th>
                          <th className="px-6 py-4 font-medium text-right">
                            <span className="sr-only">Abrir</span>
                          </th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-border bg-card">
                        {planRows.map((row) => {
                          const meta = planStatusMeta(row.status);
                          const StatusIcon = meta.icon;
                          return (
                            <tr
                              key={row.id}
                              onClick={() => navigate("/financeiro/cobrancas")}
                              className="cursor-pointer hover:bg-muted/50 transition-colors"
                              title="Abrir em Cobranças"
                            >
                              <td className="px-6 py-4 font-medium text-foreground">{row.label}</td>
                              <td className="px-6 py-4 text-muted-foreground">{formatPlanDate(row.dueDate)}</td>
                              <td className="px-6 py-4 font-medium text-foreground">{formatCurrency(row.amount)}</td>
                              <td className="px-6 py-4">
                                <span
                                  className={clsx(
                                    "inline-flex items-center gap-1.5 py-1 px-2.5 rounded-full text-xs font-medium border",
                                    meta.className,
                                  )}
                                >
                                  <StatusIcon className="w-3 h-3" />
                                  {meta.label}
                                </span>
                              </td>
                              <td className="px-6 py-4 text-right">
                                <ChevronRight className="w-4 h-4 text-muted-foreground inline" aria-hidden="true" />
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            </motion.div>
          )}

          {activeTab === "assinatura" && contract && (
            <ContractSignaturePanel
              contract={contract}
              envelope={envelope}
              onEnvelopeChange={setEnvelope}
              onContractChange={setContract}
              onRefresh={() => loadContract(contract.id)}
            />
          )}

          {activeTab === "documentos" && (
            <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
              <div className="bg-card rounded-xl border border-border shadow-sm overflow-hidden">
                <div className="p-6 border-b border-border flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                  <h3 className="text-lg font-bold text-foreground flex items-center gap-2">
                    <FileText className="w-5 h-5 text-primary-600" />
                    Documentação Anexada
                    <span className="text-sm font-normal text-muted-foreground">({documents.length})</span>
                  </h3>
                  <Button variant="outline" onClick={() => setShowAddDoc(true)}>
                    <Plus className="w-4 h-4" />
                    Adicionar Documento
                  </Button>
                </div>

                {documents.length === 0 ? (
                  <EmptyState
                    icon={Upload}
                    title="Nenhum documento anexado"
                    message="Clique em 'Adicionar Documento' para enviar arquivos."
                    className="border-none bg-transparent my-6"
                  />
                ) : (
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4 p-6">
                    {documents.map((doc) => {
                      const sc = STATUS_COLORS[doc.status] || STATUS_COLORS.pendente;
                      return (
                        <div
                          key={doc.id}
                          className="flex items-center p-4 border border-border rounded-lg hover:border-primary-300 transition-colors group bg-muted hover:bg-card"
                        >
                          <div className="w-10 h-10 rounded bg-primary-100 text-primary-600 flex items-center justify-center shrink-0">
                            <FileText className="w-5 h-5" />
                          </div>
                          <div className="ml-4 flex-1 min-w-0">
                            <p className="font-medium text-foreground text-sm truncate">{doc.name}</p>
                            <div className="flex items-center gap-2 mt-1">
                              <span className="text-xs text-muted-foreground">
                                {doc.file_size ? formatFileSize(doc.file_size) : doc.file_type} •{" "}
                                {new Date(doc.upload_date || doc.created_at).toLocaleDateString("pt-BR")}
                              </span>
                              <span
                                className={clsx(
                                  "text-micro font-bold uppercase px-1.5 py-0.5 rounded border",
                                  sc.bg,
                                  sc.text,
                                  sc.border,
                                )}
                              >
                                {doc.status}
                              </span>
                              <span className="text-micro text-muted-foreground bg-muted px-1.5 py-0.5 rounded">
                                {CATEGORY_LABELS[doc.category] || doc.category}
                              </span>
                            </div>
                          </div>
                          <div className="ml-4 flex items-center gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                            <Button
                              variant="outline"
                              size="icon"
                              className="size-9 rounded-full text-muted-foreground hover:text-primary-600"
                            >
                              <Eye className="w-4 h-4" />
                            </Button>
                            <Button
                              variant="outline"
                              size="icon"
                              onClick={() => handleDeleteDocument(doc.id)}
                              className="size-9 rounded-full text-muted-foreground hover:text-status-critical"
                            >
                              <X className="w-4 h-4" />
                            </Button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>

              <Dialog open={showAddDoc} onOpenChange={(next) => { if (!next) setShowAddDoc(false); }}>
                <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-md">
                  <div className="px-6 py-4 border-b border-border pr-12">
                    <DialogTitle className="text-lg font-bold text-foreground flex items-center gap-2">
                      <Upload className="w-5 h-5 text-primary-600" />
                      Novo Documento
                    </DialogTitle>
                    <DialogDescription className="sr-only">Adicione um novo documento ao contrato</DialogDescription>
                  </div>
                  <div className="p-6 space-y-4">
                    <div>
                      <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">
                        Nome do Documento
                      </label>
                      <input
                        type="text"
                        value={newDoc.name}
                        onChange={(e) => setNewDoc((prev) => ({ ...prev, name: e.target.value }))}
                        placeholder="Ex: contrato_locacao.pdf"
                        className="w-full px-4 py-2.5 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none"
                      />
                    </div>
                    <div className="grid grid-cols-2 gap-4">
                      <div>
                        <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">
                          Categoria
                        </label>
                        <select
                          value={newDoc.category}
                          onChange={(e) => setNewDoc((prev) => ({ ...prev, category: e.target.value as DocumentCategory }))}
                          className="w-full px-4 py-2.5 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none appearance-none"
                        >
                          <option value="contrato">Contrato</option>
                          <option value="vistoria">Vistoria</option>
                          <option value="financeiro">Financeiro</option>
                          <option value="juridico">Jurídico</option>
                          <option value="outros">Outros</option>
                        </select>
                      </div>
                      <div>
                        <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">
                          Tipo de Arquivo
                        </label>
                        <select
                          value={newDoc.file_type}
                          onChange={(e) => setNewDoc((prev) => ({ ...prev, file_type: e.target.value }))}
                          className="w-full px-4 py-2.5 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none appearance-none"
                        >
                          <option value="application/pdf">PDF</option>
                          <option value="image/jpeg">JPEG</option>
                          <option value="image/png">PNG</option>
                          <option value="application/zip">ZIP</option>
                          <option value="application/msword">DOC</option>
                        </select>
                      </div>
                    </div>
                    <div>
                      <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">Observações</label>
                      <input
                        type="text"
                        value={newDoc.notes || ""}
                        onChange={(e) => setNewDoc((prev) => ({ ...prev, notes: e.target.value }))}
                        placeholder="Opcional"
                        className="w-full px-4 py-2.5 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none"
                      />
                    </div>
                  </div>
                  <div className="px-6 py-4 border-t border-border flex justify-end gap-3">
                    <Button variant="ghost" onClick={() => setShowAddDoc(false)}>
                      Cancelar
                    </Button>
                    <Button onClick={handleAddDocument} loading={addingDoc} loadingText="Adicionando..." disabled={!newDoc.name}>
                      <Plus className="w-4 h-4" />
                      Adicionar
                    </Button>
                  </div>
                </DialogContent>
              </Dialog>
            </motion.div>
          )}
        </div>
      </div>

      {contract && (
        <SendForSignatureModal
          open={showSendModal}
          onClose={() => setShowSendModal(false)}
          contract={contract}
          onSubmit={handleSendForSignature}
        />
      )}
      {contract && (
        <TerminateContractModal open={showTerminate} onClose={() => setShowTerminate(false)} contract={contract} onCompleted={(updated) => setContract(updated)} />
      )}
      {contract && (
        <ReadjustContractPanel
          open={showReadjust}
          onClose={() => setShowReadjust(false)}
          contract={contract}
          onCompleted={(updated) => setContract(updated)}
        />
      )}
      {contract && (
        <AmendDocumentPanel
          key={`${contract.id}-${showEditDocument ? "edit" : "create"}`}
          mode={showEditDocument ? "edit" : "create"}
          open={showAmend || showEditDocument}
          onClose={() => { setShowAmend(false); setShowEditDocument(false); }}
          contract={contract}
          onCompleted={(updated, reviewSignature) => {
            setContract(updated);
            setEnvelope(null);
            setSignedFileUrl(null);
            if (reviewSignature) {
              setActiveTab("assinatura");
              openSignatureFlow();
            }
          }}
        />
      )}
      {contract && showBillingPlanWizard && (
        <BillingPlanWizard
          contractId={contract.id}
          contractType={isRentalContract ? "RENT" : "SALE"}
          // O negócio vinculado alimenta a composição econômica da venda
          // (repasse ao proprietário + comissões) nas cobranças geradas.
          dealId={contract.deal_id || undefined}
          defaultMonthlyAmount={isRentalContract ? contract.value : undefined}
          defaultTotalAmount={!isRentalContract ? contract.value : undefined}
          onClose={() => setShowBillingPlanWizard(false)}
          onSaved={() => {}}
        />
      )}
    </div>
  );
}

interface PlanStatusMeta {
  label: string;
  className: string;
  icon: typeof CheckCircle2;
}

/** Vocabulário de cobrança (charge) e de parcela (invoice) em um só lugar. */
function planStatusMeta(status: string): PlanStatusMeta {
  switch (normalizeChargeStatus(status)) {
    case "PAID":
      return { label: "Pago", className: "bg-status-ok-soft text-status-ok border-status-ok/30", icon: CheckCircle2 };
    case "PARTIALLY_PAID":
      return { label: "Parcial", className: "bg-status-info-soft text-status-info border-status-info/30", icon: Clock };
    case "OVERDUE":
      return { label: "Em atraso", className: "bg-status-critical-soft text-status-critical border-status-critical/30", icon: AlertCircle };
    case "CANCELLED":
      return { label: "Cancelada", className: "bg-muted text-muted-foreground border-border", icon: X };
    case "REVERSED":
      return { label: "Reversada", className: "bg-muted text-muted-foreground border-border", icon: X };
    default:
      return { label: "Pendente", className: "bg-status-warning-soft text-status-warning border-status-warning/30", icon: Clock };
  }
}

function formatPlanDate(iso?: string): string {
  if (!iso) return "—";
  return formatISODateBR(iso) || "—";
}

/**
 * Resumo do plano emitido. Existe para que o painel nunca pareça vazio
 * quando há cobranças: o número de emitidas e o que já entrou ficam visíveis
 * acima da tabela.
 */
function RentalPlanSummary({ charges }: { charges: Charge[] }) {
  const totals = useMemo(() => summarizeContractPlan(charges), [charges]);

  if (totals.count === 0) return null;

  const items = [
    { label: "Cobranças emitidas", value: String(totals.count), tone: "text-foreground" },
    { label: "Valor emitido", value: formatMoneyValue(totals.emitted), tone: "text-foreground" },
    { label: "Recebido", value: formatMoneyValue(totals.received), tone: "text-status-ok" },
    {
      label: "Em aberto",
      value: formatMoneyValue(totals.open),
      tone: totals.open > 0 ? "text-status-warning" : "text-foreground",
    },
  ];

  return (
    <div className="border-b border-border bg-muted px-6 py-3">
      <dl className="grid grid-cols-2 gap-x-6 gap-y-2 sm:grid-cols-4">
        {items.map((item) => (
          <div key={item.label} className="min-w-0">
            <dt className="truncate text-micro font-bold uppercase tracking-wide text-muted-foreground">{item.label}</dt>
            <dd className={clsx("truncate text-sm font-bold tabular-nums", item.tone)}>{item.value}</dd>
          </div>
        ))}
      </dl>
      {totals.overdue > 0 && (
        <p className="mt-2 text-caption font-bold text-status-critical">
          {formatMoneyValue(totals.overdue)} em atraso
        </p>
      )}
    </div>
  );
}

function formatMoneyValue(value: number): string {
  return new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(value);
}

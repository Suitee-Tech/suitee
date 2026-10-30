import { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { AnimatePresence, motion } from "motion/react";
import { AlertCircle, Check, CheckCircle2, ChevronsUpDown, Clock, CreditCard, Download, Eye, ExternalLink, FileCheck, FileSignature, FileText, Loader2, Lock, Maximize2, Minimize2, Plus, RefreshCw, Send, User, UserPlus, X } from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { logger } from "../../../../lib/logger";
import { Popover, PopoverContent, PopoverTrigger } from "../../../../shared/ui/popover";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, CommandSeparator } from "../../../../shared/ui/command";
import { PreviewVariableForm } from "../../../../shared/components/contract-editor/PreviewVariableForm";
import { useDealsContext } from "../../../../shared/context/DealsContext";
import { useAuthUser } from "../../../../shared/context/AuthUserContext";
import { useOptionalAuth } from "../../../../shared/context/AuthContext";
import { useDealContractMutationsContext } from "../../../../shared/context/DealContractMutationsContext";
import { useDealChecklistsContext } from "../../../../shared/context/DealChecklistsContext";
import { dealService, DealDTO } from "../../../../shared/services/deal.service";
import { leadService, ApiLead } from "../../../../shared/services/lead.service";
import { contractService } from "../../../../shared/services/contract.service";
import type { Contract, CreateContractRequest, SendForSignatureRequest } from "../../../../shared/contracts/types";
import { SignatureStatusBadge } from "../../../../shared/components/signature/SignatureStatusBadge";
import { SignedDocumentsSection } from "../../../../shared/components/signature/SignedDocumentsSection";
import { SendForSignatureModal, type SignerPatch } from "../../../../shared/components/signature/SendForSignatureModal";
import { rentalChargeConfigService } from "../../../../shared/services/rentalChargeConfig.service";
import { saleBillingPlanService, SaleBillingPlanDraft } from "../../../../shared/services/saleBillingPlan.service";
import { tenantFinancialSettingsService } from "../../../../shared/services/tenantFinancialSettings.service";
import { BillingPlanWizard } from "../../../../shared/components/BillingPlanWizard";
import { propertyService, PropertyDTO } from "../../../../shared/services/property.service";
import { brokerService, BrokerDTO } from "../../../../shared/services/broker.service";
import { templateService, ContractTemplate } from "../../../../shared/services/template.service";
import type { FirstChargeType } from "../../../../shared/rentals/types";
import { unwrapApiData } from "../../../../shared/services/api";
import { notifyApiError } from "../../../../shared/services/errorToast";
import {
  base64ToFile,
  extractDocxTemplateVariables,
  fillDocxTemplate,
} from "../../../../shared/utils/docx";
import { formatPhoneBR } from "../../../../shared/utils/masks";
import { SuperDocEditor, SuperDocRef } from "@superdoc-dev/react";
import "@superdoc-dev/react/style.css";
import { flags } from "../../../../shared/config/flags";
import { Button } from "../../../../shared/ui/button";
import { ExpandableSidePanel } from "../../../../shared/components/ExpandableSidePanel";
import { LAYER } from "../../../../shared/ui/layers";
import { RequirementCard } from "./RequirementCard";
import { ContractSignatureSection } from "./ContractSignatureSection";
import { buildContractVariableData } from "../../../../shared/utils/contractVariableData";
import { useContractSignatureEnvelope } from "./useContractSignatureEnvelope";
import { FinancialPlanRestricted, isRentalPreContractPlanReady, PreContractFinancialSetup } from "./PreContractFinancialSetup";
import { usePermissionsContext } from "../../../../shared/context/PermissionsContext";
import {
  addMonthsClamped,
  DEFAULT_LEASE_TERM_MONTHS,
  MAX_LEASE_TERM_MONTHS,
  MIN_LEASE_TERM_MONTHS,
  wholeMonthsBetween,
} from "../../../../shared/lib/finance/rentalProration";
import {
  type CachedRentalPreContractPlan,
  rentalPlanCacheKey as buildRentalPlanCacheKey,
} from "../../../../shared/lib/finance/preContractPlanCache";

type TabType = "proposta" | "partes" | "documentacao" | "analise" | "contrato" | "financeiro";

/**
 * Texto do botão desabilitado por plano financeiro incompleto.
 *
 * Quem não tem acesso financeiro não pode resolver o que está pendente — dizer
 * "conclua o plano" para esse perfil é um beco sem saída. Nesse caso o texto
 * aponta quem resolve.
 */
function financialReadyBlockedHint(canSeeFinancialPlan: boolean, dealType?: DealDTO["type"]): string {
  if (!canSeeFinancialPlan) {
    return "A área financeira ainda não concluiu o plano-base desta negociação";
  }
  return dealType === "SALE"
    ? "Defina a forma de recebimento antes de gerar a minuta"
    : "Revise a vigência e o primeiro vencimento do plano-base antes de gerar a minuta";
}



const isValidISODate = (value: string): boolean =>
  Boolean(value) && !Number.isNaN(Date.parse(`${value}T00:00:00Z`));

const defaultFirstChargeDateFor = (startDate: string, dueDay: number): string => {
  if (!isValidISODate(startDate)) return "";
  const start = new Date(`${startDate}T00:00:00Z`);
  const safeDueDay = Math.min(28, Math.max(1, dueDay));
  const target = new Date(Date.UTC(start.getUTCFullYear(), start.getUTCMonth(), safeDueDay));
  if (start.getUTCDate() > safeDueDay) target.setUTCMonth(target.getUTCMonth() + 1);
  return target.toISOString().slice(0, 10);
};

const ROLE_LABEL: Record<string, string> = {
  tenant: "Inquilino",
  buyer: "Comprador",
  owner: "Proprietário",
  guarantor: "Fiador/Avalista",
  witness: "Testemunha",
  spouse: "Cônjuge",
  attorney: "Procurador",
  legal_representative: "Representante legal",
  guardian: "Tutor",
  curator: "Curador",
};

// Contract status badge config moved to SignatureStatusBadge (single source
// of truth shared with portal + DealPartiesSection). See the canonical
// mapping in toka/src/shared/components/signature/SignatureStatusBadge.tsx.

/**
 * Editor de modelos de contrato (DOCX + variáveis), a mesma tela de
 * "Modelos de Contrato", carregada sob demanda: só entra no bundle quando o
 * operador abre o cadastro pelo painel do contrato.
 */
const ContractTemplateEditor = lazy(() =>
  import("../../../../shared/pages/ContractTemplates").then((m) => ({ default: m.ContractTemplates })),
);

export function ContractTab({
  dealDetails,
  onRefresh,
  onGoToTab,
}: {
  dealDetails: DealDTO | null;
  onRefresh: () => void;
  onGoToTab: (t: TabType) => void;
}) {
  const { createContractFromDeal } = useDealContractMutationsContext();
  const { inviteSigner } = useDealsContext();
  const { dealChecklists } = useDealChecklistsContext();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [isAdding, setIsAdding] = useState(false);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [role, setRole] = useState(dealDetails?.type === "SALE" ? "comprador" : "inquilino");
  const [isFullScreen, setIsFullScreen] = useState(false);

  const [buyerLead, setBuyerLead] = useState<ApiLead | null>(null);
  const [sellerLead, setSellerLead] = useState<ApiLead | null>(null);
  const [brokersById, setBrokersById] = useState<Record<string, BrokerDTO>>({});
  const { user: me } = useAuthUser();
  const auth = useOptionalAuth();
  const currentCompany = auth?.companies.find((company) => company.id === me?.current_company_id) ?? auth?.companies[0] ?? null;
  const { can, isLoading: isLoadingPermissions } = usePermissionsContext();
  // Quem configura o plano financeiro pré-contrato: quem enxerga o módulo
  // financeiro (`finance:read`) ou quem edita cobranças/recebimento
  // (`charges:write`, o caso de Operações). Sem nenhum dos dois — corretor,
  // marketing, perfis customizados só com contratos — o painel vira o estado
  // restrito: antes ele era renderizado de qualquer jeito, mostrando plano-base,
  // taxa de administração e botões que o servidor recusaria com 403.
  const canSeeFinancialPlan = can("finance:read") || can("charges:write");
  // Criar modelo exige `templates:write` (admin/gestor). Sem essa permissão o
  // CTA viraria outro beco sem saída — o servidor recusaria o salvamento.
  const canWriteTemplates = can("templates:write");
  const [ownerLeadsById, setOwnerLeadsById] = useState<Record<string, ApiLead>>({});

  const [contract, setContract] = useState<Contract | null>(null);
  const [property, setProperty] = useState<PropertyDTO | null>(null);
  const [sendingForSignature, setSendingForSignature] = useState(false);
  const [sendForSignatureOpen, setSendForSignatureOpen] = useState(false);

  // FASE C — billing plan draft (SALE pré-assinatura). O draft é
  // gravado pelo BillingPlanWizard em mode='draft' e consumido pelo
  // handler contract.signed para materializar o SaleBillingPlan real.
  // A escolha é explícita: sem draft ativo o financeiro não fabrica uma
  // cobrança à vista e a UX não permite enviar a minuta para assinatura.
  const [showBillingDraftWizard, setShowBillingDraftWizard] = useState(false);
  const [billingDraft, setBillingDraft] = useState<SaleBillingPlanDraft | null>(null);

  // Carrega o draft ativo do deal (somente SALE). Re-busca sempre que o
  // deal muda ou após o wizard salvar (via refreshBillingDraft).
  const refreshBillingDraft = useMemo(() => {
    return () => {
      // O draft de recebimento é dado financeiro: sem acesso, nem busca (o
      // endpoint exige `charges:read` e devolveria 403).
      if (!dealDetails?.id || dealDetails?.type !== "SALE" || !canSeeFinancialPlan) {
        setBillingDraft(null);
        return;
      }
      saleBillingPlanService
        .getDraft(dealDetails.id)
        .then((d) => setBillingDraft(d))
        .catch(() => setBillingDraft(null));
    };
  }, [dealDetails?.id, dealDetails?.type, canSeeFinancialPlan]);

  useEffect(() => {
    refreshBillingDraft();
  }, [refreshBillingDraft]);
  const todayDate = new Date().toISOString().split("T")[0];
  const rentalPlanCacheKey = useMemo(
    () => buildRentalPlanCacheKey(dealDetails?.id),
    [dealDetails?.id],
  );
  const cachedRentalPlan = dealDetails?.id
    ? queryClient.getQueryData<CachedRentalPreContractPlan>(rentalPlanCacheKey)
    : undefined;
  // Prazo do contrato em meses: é ele que dirige o término (o antigo 30 fixo
  // virou campo). `addMonthsClamped` ancora no último dia do mês alvo — sem
  // isso 31/08 + 30 meses vazava para 03/03 e o residual do último aluguel
  // cobrava dias que não existem no contrato.
  const [rentalStartDate, setRentalStartDate] = useState<string>(cachedRentalPlan?.startDate ?? todayDate);
  const [rentalTermMonths, setRentalTermMonths] = useState<number>(
    cachedRentalPlan?.termMonths ?? DEFAULT_LEASE_TERM_MONTHS,
  );
  const [rentalEndDate, setRentalEndDate] = useState<string>(
    () => cachedRentalPlan?.endDate ?? addMonthsClamped(todayDate, DEFAULT_LEASE_TERM_MONTHS),
  );
  // Enquanto o usuário não editar o TÉRMINO à mão, ele acompanha o prazo. Ao
  // editar direto, o prazo passa a ser derivado das datas (meses inteiros) e o
  // excedente aparece como residual no último aluguel.
  const rentalEndUserEdited = useRef(Boolean(cachedRentalPlan?.endDate));
  const dueDayUserEdited = useRef(Boolean(cachedRentalPlan?.defaultDueDay));
  // Prazo escolhido à mão (ou já gravado no rascunho) não é sobrescrito pelo
  // padrão da imobiliária, que chega depois na query de parâmetros.
  const termUserEdited = useRef(Boolean(cachedRentalPlan?.termMonths));
  const firstChargeDateUserEdited = useRef(Boolean(cachedRentalPlan?.firstChargeDate));
  const [defaultDueDay, setDefaultDueDay] = useState<number>(cachedRentalPlan?.defaultDueDay ?? 7);
  const [firstChargeType, setFirstChargeType] = useState<FirstChargeType>(
    cachedRentalPlan?.firstChargeType ?? "same_month_pro_rata",
  );
  const [firstChargeDate, setFirstChargeDate] = useState<string>(
    () => cachedRentalPlan?.firstChargeDate ?? defaultFirstChargeDateFor(todayDate, 7),
  );

  const financialSettingsQuery = useQuery({
    queryKey: ["financial-settings"],
    queryFn: () => tenantFinancialSettingsService.get(),
    // Sem acesso financeiro a consulta nem sai: o servidor responderia 403 e a
    // tela ficaria com um erro de rede que o usuário não pode resolver.
    enabled: dealDetails?.type === "RENT" && canSeeFinancialPlan,
    staleTime: 5 * 60_000,
  });

  const computedEndDate = addMonthsClamped(rentalStartDate, rentalTermMonths);

  useEffect(() => {
    if (rentalEndUserEdited.current) return;
    if (!computedEndDate) return;
    setRentalEndDate(computedEndDate);
  }, [computedEndDate]);

  useEffect(() => {
    const configuredDay = financialSettingsQuery.data?.default_due_day;
    if (!configuredDay || dueDayUserEdited.current) return;
    setDefaultDueDay(Math.min(28, Math.max(1, configuredDay)));
  }, [financialSettingsQuery.data?.default_due_day]);

  // Prazo padrão da imobiliária (Configurações financeiras → Padrões). Vale
  // como sugestão inicial: quem já mexeu no prazo ou tem rascunho mantém o que
  // escolheu.
  const configuredTermMonths = financialSettingsQuery.data?.default_lease_term_months;
  useEffect(() => {
    if (termUserEdited.current) return;
    if (!configuredTermMonths || configuredTermMonths < MIN_LEASE_TERM_MONTHS) return;
    const months = Math.min(MAX_LEASE_TERM_MONTHS, configuredTermMonths);
    setRentalTermMonths(months);
    const derived = addMonthsClamped(rentalStartDate, months);
    if (derived) setRentalEndDate(derived);
  }, [configuredTermMonths, rentalStartDate]);

  useEffect(() => {
    if (firstChargeDateUserEdited.current) return;
    setFirstChargeDate(defaultFirstChargeDateFor(rentalStartDate, defaultDueDay));
  }, [rentalStartDate, defaultDueDay]);

  useEffect(() => {
    if (!dealDetails?.id || dealDetails.type !== "RENT") return;
    queryClient.setQueryData<CachedRentalPreContractPlan>(rentalPlanCacheKey, {
      startDate: rentalStartDate,
      endDate: rentalEndDate,
      firstChargeType,
      firstChargeDate,
      defaultDueDay,
      termMonths: rentalTermMonths,
    });
  }, [
    dealDetails?.id,
    dealDetails?.type,
    defaultDueDay,
    firstChargeDate,
    firstChargeType,
    queryClient,
    rentalEndDate,
    rentalTermMonths,
    rentalPlanCacheKey,
    rentalStartDate,
  ]);

  useEffect(() => {
    if (!dealDetails) return;
    if (dealDetails.customer_id) {
      leadService
        .getById(dealDetails.customer_id)
        .then((data) => setBuyerLead(data))
        .catch(() => logger.debug("Could not fetch buyer lead details"));
    }
  }, [dealDetails]);

  useEffect(() => {
    if (!dealDetails) return;
    if (dealDetails.owner_id) {
      leadService
        .getById(dealDetails.owner_id)
        .then((data) => setSellerLead(data))
        .catch(() => logger.debug("Could not fetch seller lead details"));
    }
  }, [dealDetails]);

  useEffect(() => {
    const ownerIds = property?.owners?.map((o) => o.id).filter(Boolean) || [];
    if (ownerIds.length === 0) {
      setOwnerLeadsById({});
      return;
    }

    const uniqueOwnerIds = Array.from(new Set(ownerIds));
    Promise.all(uniqueOwnerIds.map((id) => leadService.getById(id).then((data) => ({ id, data })).catch(() => null))).then((results) => {
      const next: Record<string, ApiLead> = {};
      for (const r of results) {
        if (!r?.data) continue;
        next[r.id] = r.data;
      }
      setOwnerLeadsById(next);
    });
  }, [property?.id, property?.owners?.length]);

  useEffect(() => {
    const ids = new Set<string>();
    if (dealDetails?.broker_id) ids.add(dealDetails.broker_id);
    for (const p of dealDetails?.broker_participations || []) ids.add(p.broker_id);
    for (const b of property?.capturing_brokers || []) ids.add(b.id);

    const missing = Array.from(ids).filter((id) => id && !brokersById[id]);
    if (missing.length === 0) return;

    Promise.all(missing.map((id) => brokerService.getById(id).then((res) => ({ id, data: unwrapApiData<BrokerDTO>(res) })).catch(() => null))).then(
      (results) => {
        const next = { ...brokersById };
        let changed = false;
        for (const r of results) {
          if (!r?.data) continue;
          next[r.id] = r.data;
          changed = true;
        }
        if (changed) setBrokersById(next);
      },
    );
  }, [dealDetails?.id, property?.id, property?.capturing_brokers?.length, dealDetails?.broker_participations?.length, brokersById]);

  // Garante que o próprio corretor logado esteja sempre em brokersById, mesmo
  // sem permissão de leitura (brokers:read), para o template de contrato
  // resolver o nome do captador/visitador/fechamento quando for o próprio.
  useEffect(() => {
    const mid = me?.id;
    if (!mid) return;
    setBrokersById((prev) => {
      if (prev[mid]) return prev;
      return {
        ...prev,
        [mid]: {
          id: mid,
          name: me.fullname || me.email || "Você",
          role: me.role || "corretor",
          avatar: me.avatar_url || "",
          active: true,
          tenant_id: "",
          created_at: "",
          updated_at: "",
        },
      };
    });
  }, [me?.id, me?.fullname, me?.avatar_url]);

  const [loadingContract, setLoadingContract] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [regenerating, setRegenerating] = useState(false);
  const [preparingRegeneration, setPreparingRegeneration] = useState(false);
  const generationBusy = useRef(false);
  const [templates, setTemplates] = useState<ContractTemplate[]>([]);
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>("");
  // Cadastro de modelo aberto DENTRO do painel do contrato. Sem nenhum modelo
  // cadastrado não há minuta a gerar: o estado vazio do seletor abre o mesmo
  // editor da tela "Modelos de Contrato" (embutido), em vez de mandar o
  // operador procurar a tela e perder o negócio aberto.
  const [creatingTemplate, setCreatingTemplate] = useState(false);
  const [showPreview, setShowPreview] = useState(false);
  const [previewFile, setPreviewFile] = useState<File | null>(null);
  const [showVariableForm, setShowVariableForm] = useState(false);
  const [autoFilledData, setAutoFilledData] = useState<Record<string, any>>({});
  const [previewTemplateId, setPreviewTemplateId] = useState("");
  const [previewSourceFingerprint, setPreviewSourceFingerprint] = useState("");
  const [previewFormRevision, setPreviewFormRevision] = useState(0);
  const [previewVariableKeys, setPreviewVariableKeys] = useState<string[]>([]);
  const editorRef = useRef<SuperDocRef>(null);

  const signers = dealDetails?.signers || [];
  const proposals = dealDetails?.proposals || [];
  const latestProposal = proposals.length > 0 ? proposals[proposals.length - 1] : null;
  const parties = dealDetails?.parties ?? [];
  const customerRole = dealDetails?.type === "RENT" ? "tenant" : "buyer";
  const primaryCustomer = parties.find((p) => p.role === customerRole && p.is_primary);
  const primaryOwner = parties.find((p) => p.role === "owner" && p.is_primary);
  // A minuta guarda um snapshot dos signatários. Quando a esteira corrige o
  // e-mail/nome da parte, o contrato ainda mostrava o valor antigo no modal de
  // envio (o backend já sincroniza antes de criar o envelope). Refletimos a
  // correção aqui para a revisão do operador bater com o que será enviado.
  useEffect(() => {
    if (!contract || contract.status === "SIGNED") return;
    const byId = new Map(signers.filter((signer) => signer.id).map((signer) => [signer.id, signer]));
    let changed = false;
    const next = contract.signatories.map((signatory) => {
      if (signatory.signed_at) return signatory;
      const current = byId.get(signatory.id);
      if (!current) return signatory;
      const name = (current.name ?? "").trim();
      const email = (current.email ?? "").trim();
      if ((!name || name === signatory.name) && (!email || email === signatory.email)) return signatory;
      changed = true;
      return {
        ...signatory,
        name: name || signatory.name,
        email: email || signatory.email,
      };
    });
    if (changed) {
      setContract((previous) => previous ? { ...previous, signatories: next } : previous);
    }
  }, [contract, signers]);

  const acceptedBy = latestProposal?.accepted_by ?? [];
  const isAcceptedBy = (partyId?: string) => !!partyId && acceptedBy.some((a) => a.party_id === partyId);
  // Required = configured on the party (signature_required) or fallback to is_primary,
  // limited to the customer/owner roles that actually accept proposals.
  const requiredPartyIDs = parties
    .filter((p) => (p.signature_required ?? p.is_primary))
    .filter((p) => p.role === customerRole || p.role === "owner")
    .map((p) => p.id);
  const requiredAccepted = requiredPartyIDs.length > 0 && requiredPartyIDs.every((id) => isAcceptedBy(id));
  // Sem nenhuma parte obrigatória não há aceite a exigir: a geração do contrato
  // segue liberada, como os "Parâmetros da Proposta Digital" anunciam ("Sem partes
  // obrigatórias, o deal avança sem aceite formal da proposta"). A exceção exige
  // configuração EXPLÍCITA (alguma parte com `signature_required` definido) para
  // não afrouxar negócios antigos, em que o campo ausente significa parte primária.
  const acceptanceNotRequired = requiredPartyIDs.length === 0
    && parties.some((party) => party.signature_required !== undefined);
  const canProceedContract = !flags.multiPartyUI || !latestProposal || latestProposal.status === "ACCEPTED"
    || requiredAccepted || acceptanceNotRequired;
  const saleDraftCurrent =
    Boolean(billingDraft) && billingDraft!.total_amount_cents === Math.round((dealDetails?.value || 0) * 100);
  const rentalPreContractPlan = {
    startDate: rentalStartDate,
    endDate: rentalEndDate,
    suggestedEndDate: computedEndDate,
    firstChargeType,
    firstChargeDate,
    defaultDueDay,
    termMonths: rentalTermMonths,
    adminFeePct: financialSettingsQuery.data?.admin_fee_pct,
    // Mudar o início recalcula o término pelo prazo vigente; mudar o prazo
    // recalcula o término; editar o término à mão recalcula o prazo (meses
    // inteiros). As três pontas nunca ficam incoerentes entre si.
    onStartDateChange: (value: string) => {
      rentalEndUserEdited.current = false;
      setRentalStartDate(value);
      const derived = addMonthsClamped(value, rentalTermMonths);
      if (derived) setRentalEndDate(derived);
    },
    onTermMonthsChange: (value: number) => {
      const months = Number.isFinite(value)
        ? Math.min(MAX_LEASE_TERM_MONTHS, Math.max(MIN_LEASE_TERM_MONTHS, Math.trunc(value)))
        : DEFAULT_LEASE_TERM_MONTHS;
      termUserEdited.current = true;
      rentalEndUserEdited.current = false;
      setRentalTermMonths(months);
      const derived = addMonthsClamped(rentalStartDate, months);
      if (derived) setRentalEndDate(derived);
    },
    onEndDateChange: (value: string) => {
      termUserEdited.current = true;
      rentalEndUserEdited.current = true;
      setRentalEndDate(value);
      const months = wholeMonthsBetween(rentalStartDate, value);
      if (months >= MIN_LEASE_TERM_MONTHS) setRentalTermMonths(Math.min(MAX_LEASE_TERM_MONTHS, months));
    },
    onFirstChargeTypeChange: setFirstChargeType,
    onFirstChargeDateChange: (value: string) => {
      firstChargeDateUserEdited.current = true;
      setFirstChargeDate(value);
    },
    onDefaultDueDayChange: (value: number) => {
      dueDayUserEdited.current = true;
      setDefaultDueDay(Number.isFinite(value) ? value : 0);
    },
  };
  const rentalFinancialReady = isRentalPreContractPlanReady(rentalPreContractPlan);
  const financialReady = dealDetails?.type === "SALE" ? saleDraftCurrent : rentalFinancialReady;

  const isMultiParty = !!(flags.multiPartyUI && parties.length > 0);
  const checklist = dealDetails?.id ? (dealChecklists[dealDetails.id] ?? []) : [];
  const pendingDocs = checklist.filter((i) => (i.status === "pending" || i.status === "rejected") && !i.solicited).length;
  const missingPrimaries = isMultiParty && (!primaryCustomer || !primaryOwner);

  // A lista é relida também DEPOIS da montagem: o cadastro do modelo pode
  // acontecer no painel embutido desta mesma tela (abaixo), e sem isso o
  // seletor continuaria vazio depois de o operador criar o primeiro modelo.
  const loadTemplates = useCallback(async () => {
    try {
      const data = await templateService.list();
      const list = data || [];
      setTemplates(list);
      setSelectedTemplateId((current) => {
        if (current && list.some((t) => t.id === current)) return current;
        return list[0]?.id ?? "";
      });
      return list;
    } catch (err) {
      console.error("Error loading templates", err);
      return [];
    }
  }, []);

  // Trocar de modelo invalida a minuta já pré-visualizada (o preview carrega o
  // template escolhido no momento em que foi gerado).
  const selectTemplate = useCallback((id: string) => {
    setSelectedTemplateId(id);
    setShowVariableForm(false);
    setShowPreview(false);
    setPreviewFile(null);
    setPreviewTemplateId("");
    setPreviewSourceFingerprint("");
    setPreviewVariableKeys([]);
  }, []);

  useEffect(() => {
    void loadTemplates();
  }, [loadTemplates]);

  useEffect(() => {
    if (!dealDetails?.id) return;
    setLoadingContract(true);
    contractService
      .listByDeal(dealDetails.id)
      .then((contracts) => {
        setContract(contracts?.length > 0 ? contracts[0] : null);
      })
      .catch(() => setContract(null))
      .finally(() => setLoadingContract(false));
  }, [dealDetails?.id]);

  const signatureEnvelope = useContractSignatureEnvelope({
    contract,
    dealId: dealDetails?.id,
    dealSigners: signers,
    onContractChange: setContract,
    onRefresh,
  });
  const { envelope, envelopeLoading, envelopeLoadFailed } = signatureEnvelope;
  const failedBeforeEnvelope = envelope?.status === "ERROR" && !envelope.external_envelope_id;
  // Envelope cancelado/expirado/recusado antes de qualquer assinatura é uma
  // tentativa encerrada, não um contrato assinado. O operador pode corrigir o
  // e-mail/minuta e regerar; o vínculo antigo é limpo pelo backend.
  const terminalEnvelopeWithoutSignature =
    envelope?.status === "CANCELED" ||
    envelope?.status === "EXPIRED" ||
    envelope?.status === "REFUSED";
  const canRegenerate = Boolean(contract)
    && !envelopeLoading && !envelopeLoadFailed
    && !contract?.signed_document_url
    && !contract?.signatories.some((signer) => signer.signed_at || signer.signature)
    && (contract?.status === "DRAFT" || contract?.status === "PENDING_SIGNATURE")
    && (failedBeforeEnvelope || terminalEnvelopeWithoutSignature ||
      (!contract?.external_envelope_id && !envelope && contract?.status === "DRAFT" && !contract.signature_locked_at && !contract.sent_for_signature_at));
  const canSend = contract?.status === "DRAFT" && !contract.signature_locked_at
    && !envelopeLoading && !envelopeLoadFailed
    && (!envelope || failedBeforeEnvelope || terminalEnvelopeWithoutSignature);

  const beginRegeneration = async () => {
    if (!contract || !canRegenerate || preparingRegeneration) return;
    setPreparingRegeneration(true);
    try {
      if (dealDetails?.type === "RENT") {
        const config = await rentalChargeConfigService.getByContractId(contract.id);
        rentalEndUserEdited.current = true;
        firstChargeDateUserEdited.current = true;
        dueDayUserEdited.current = true;
        setRentalStartDate(contract.start_date.slice(0, 10));
        setRentalEndDate(contract.end_date?.slice(0, 10) || "");
        if (config) {
          setFirstChargeDate(config.first_charge_date.slice(0, 10));
          setFirstChargeType(config.first_charge_type);
          setDefaultDueDay(config.default_due_day);
        }
      }
      setShowPreview(false);
      setPreviewFile(null);
      setShowVariableForm(false);
      setIsFullScreen(false);
      setRegenerating(true);
    } catch (error) {
      notifyApiError(error, { context: "Carregar dados do contrato" });
    } finally {
      setPreparingRegeneration(false);
    }
  };

  // A signed contract's legal artifact is the Clicksign PDF, not the editable
  // DOCX draft.
  const contractSigned = (contract?.status ?? "").toString().toUpperCase() === "SIGNED";
  useEffect(() => {
    if (!dealDetails?.property_id) return;
    propertyService
      .getById(dealDetails.property_id)
      .then((data) => setProperty(unwrapApiData(data)))
      .catch((err) => console.error("Error loading property", err));
  }, [dealDetails?.property_id]);

  const buildVariableData = (): Record<string, any> =>
    buildContractVariableData({
      dealDetails,
      property,
      signers,
      buyerLead,
      sellerLead,
      brokersById,
      ownerLeadsById,
      rentalStartDate,
      rentalEndDate,
      computedEndDate,
      firstChargeDate,
      firstChargeType,
      defaultDueDay,
      contract,
      preferContractDates: !!contract && !regenerating,
      company: currentCompany,
    });

  const fingerprintVariableData = (data: Record<string, any>): string => {
    // Em vendas, esse campo é criado com `new Date().toISOString()` a cada
    // chamada. O restante dos campos de data já detecta mudanças relevantes.
    const stableData = { ...data };
    delete stableData.contract_start_date_iso;
    return JSON.stringify(stableData);
  };

  const handlePreviewContract = () => {
    if (!dealDetails) return;
    const template = templates.find((t) => t.id === selectedTemplateId);
    if (!template) {
      toast.error("Selecione um template");
      return;
    }

    const data = buildVariableData();
    setAutoFilledData(data);
    setPreviewVariableKeys(Array.from(new Set([
      ...(template.variables || []),
      ...extractDocxTemplateVariables(template.content, data),
    ])));
    // Um preview anterior não pode voltar a ser considerado atual enquanto o
    // novo formulário ainda não foi confirmado.
    setShowPreview(false);
    setPreviewFile(null);
    setPreviewTemplateId("");
    setPreviewSourceFingerprint("");
    setPreviewFormRevision((revision) => revision + 1);
    setShowVariableForm(true);
  };

  const handleVariableFormConfirm = (finalData: Record<string, any>) => {
    const template = templates.find((t) => t.id === selectedTemplateId);
    if (!template) return;

    setShowVariableForm(false);

    try {
      const base64Filled = fillDocxTemplate(template.content, finalData);
      const file = base64ToFile(base64Filled, `Preview_${template.name}.docx`);
      setPreviewFile(file);
      setPreviewTemplateId(template.id);
      setPreviewSourceFingerprint(fingerprintVariableData(autoFilledData));
      setShowPreview(true);
    } catch (e: any) {
      if (e.properties && e.properties.errors) {
        const errorMessages = e.properties.errors.map((err: any) => err.properties.explanation || err.message).join(", ");
        toast.error(`Erro de formatação no template: ${errorMessages}`);
      } else {
        toast.error("Erro ao gerar preview. Verifique as variáveis do template.");
      }
      console.error(e);
    }
  };

  const fileToBase64 = (file: File | Blob): Promise<string> => {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.readAsDataURL(file);
      reader.onload = () => resolve(reader.result as string);
      reader.onerror = (error) => reject(error);
    });
  };

  const handleGenerateContract = async () => {
    if (!dealDetails || generationBusy.current) return;
    if (!regenerating && !financialReady) {
      toast.error(
        !canSeeFinancialPlan
          ? "O plano financeiro desta negociação é configurado pela área financeira. Peça a ela para concluir o plano-base antes de gerar a minuta."
          : dealDetails.type === "SALE"
            ? "Defina uma forma de recebimento compatível com o valor da venda antes de gerar a minuta."
            : "Revise a vigência e o primeiro vencimento do plano-base antes de gerar a minuta.",
      );
      return;
    }
    const currentSourceFingerprint = fingerprintVariableData(buildVariableData());
    if (
      previewTemplateId !== selectedTemplateId ||
      previewSourceFingerprint !== currentSourceFingerprint
    ) {
      toast.error("O template ou os dados do contrato mudaram. Gere o preview novamente antes de salvar.");
      handlePreviewContract();
      return;
    }
    generationBusy.current = true;
    setGenerating(true);
    try {
      let finalContent = "";

      if (showPreview && editorRef.current) {
        const superdoc = editorRef.current.getInstance();
        if (superdoc) {
          try {
            const blobs = await superdoc.exportEditorsToDOCX();
            if (blobs && blobs.length > 0) {
              finalContent = await fileToBase64(blobs[0]);
            }
          } catch (e) {
            console.error("Failed to export docx from editor, falling back to original preview file", e);
          }
        }
      }

      if (!finalContent && previewFile) {
        finalContent = await fileToBase64(previewFile);
      }

      if (!finalContent) {
        toast.error("Gere o preview antes de salvar");
        setGenerating(false);
        return;
      }

      const signatories = signers.map((s) => ({
        id: s.id || crypto.randomUUID(),
        name: s.name,
        email: s.email,
        role: s.role,
      }));

      const isRentalDeal = dealDetails.type !== "SALE";
      const startDateIso = isRentalDeal ? new Date(`${rentalStartDate}T00:00:00Z`).toISOString() : new Date().toISOString();
      const endDateBase = isRentalDeal ? rentalEndDate || computedEndDate : "";
      const endDateIso = isRentalDeal ? new Date(`${endDateBase}T00:00:00Z`).toISOString() : undefined;

      if (isRentalDeal && (!rentalStartDate || isNaN(Date.parse(`${rentalStartDate}T00:00:00Z`)))) {
        toast.error("Informe a data de início do contrato (locação).");
        setGenerating(false);
        return;
      }
      if (isRentalDeal && (!endDateBase || isNaN(Date.parse(`${endDateBase}T00:00:00Z`)))) {
        toast.error("Não foi possível calcular a data de término (30 meses). Informe manualmente.");
        setGenerating(false);
        return;
      }
      if (isRentalDeal && new Date(`${endDateBase}T00:00:00Z`) < new Date(`${rentalStartDate}T00:00:00Z`)) {
        toast.error("A data de término não pode ser menor que a data de início.");
        setGenerating(false);
        return;
      }

      const req: CreateContractRequest = {
        deal_id: dealDetails.id,
        content: finalContent,
        signatories,
        start_date: startDateIso,
        end_date: endDateIso,
        type: dealDetails.type === "SALE" ? "venda" : "locacao",
        property_title: property?.title || "",
      };

      const created = regenerating && contract
        ? await contractService.regenerate(contract.id, {
            content: finalContent,
            signatories,
            expected_updated_at: contract.updated_at,
          })
        : await createContractFromDeal(dealDetails.id, req);
      if (created) {
        if (isRentalDeal && !regenerating) {
          try {
            await rentalChargeConfigService.create({
              contract_id: created.id,
              tenant_party_id: dealDetails.customer_id,
              owner_id: dealDetails.owner_id,
              property_id: dealDetails.property_id,
              start_date: startDateIso,
              end_date: endDateIso || startDateIso,
              first_charge_date: new Date(`${firstChargeDate}T00:00:00Z`).toISOString(),
              first_charge_type: firstChargeType,
              default_due_day: defaultDueDay,
              monthly_amount: dealDetails.value || 0,
            });
            queryClient.removeQueries({ queryKey: rentalPlanCacheKey });
          } catch (e: any) {
            toast.warning(
              e?.response?.data?.error ||
                "A minuta foi gerada, mas o plano-base da locação ainda precisa ser sincronizado no financeiro.",
            );
          }
        }
        setContract(created);
        setShowPreview(false);
        setIsFullScreen(false);
        setRegenerating(false);
        toast.success(regenerating ? "Contrato regerado. Revise e envie para assinatura." : "Contrato gerado com sucesso!");
        onRefresh();
      }
    } catch (error: any) {
      notifyApiError(error, { context: "Gerar contrato" });
    } finally {
      generationBusy.current = false;
      setGenerating(false);
    }
  };

  const handleSendForSignature = async (
    req: SendForSignatureRequest,
    patches: Record<string, SignerPatch>,
  ) => {
    if (!contract) return;

    setSendingForSignature(true);
    try {
      // Persist signer patches (CPF/birthday/phone) before kicking off the
      // signature flow so Clicksign gets the complete signer payload on the
      // first try and the contract reflects the new data going forward.
      if (Object.keys(patches).length > 0) {
        const nextSignatories = contract.signatories.map((s) => {
          const p = patches[s.id];
          if (!p) return s;
          return {
            ...s,
            documentation: p.documentation ?? s.documentation,
            birthday: p.birthday ?? s.birthday,
            phone_number: p.phone_number ?? s.phone_number,
          };
        });
        await contractService.update(contract.id, { signatories: nextSignatories });
      }

      const updated = await contractService.sendForSignature(contract.id, req);
      setContract(updated);
      toast.success("Contrato enviado para Clicksign. Aguardando assinaturas.");
      onRefresh();
    } catch (error: any) {
      const message =
        error?.response?.data?.message || error?.response?.data?.error || "Erro ao enviar para assinatura.";
      toast.error(message);
      // Refresh the failed attempt and the server's unlocked draft too.
      try { setContract(await contractService.getById(contract.id)); } catch { /* Keep the saved draft on network failure. */ }
      // Re-throw so the modal can show the error inline and stay open if
      // there's still work for the operator to fix.
      throw new Error(message);
    } finally {
      setSendingForSignature(false);
    }
  };

  const handleAddSigner = async () => {
    if (!dealDetails) return;
    try {
      await dealService.addSigner(dealDetails.id, { name, email, role: role.toLowerCase() });
      toast.success("Assinante adicionado com sucesso!");
      setIsAdding(false);
      setName("");
      setEmail("");
      setSearchLeadTerm("");
      setManualEntryMode(false);
      setSelectedLead(null);
      setSuggestedLeads([]);
      onRefresh();
    } catch (error: any) {
      console.error(error);
      notifyApiError(error, { context: "Adicionar assinante" });
    }
  };

  const [searchLeadTerm, setSearchLeadTerm] = useState("");
  const [suggestedLeads, setSuggestedLeads] = useState<ApiLead[]>([]);
  const [searchingLeads, setSearchingLeads] = useState(false);
  const [manualEntryMode, setManualEntryMode] = useState(false);
  const [selectedLead, setSelectedLead] = useState<ApiLead | null>(null);
  const [leadComboOpen, setLeadComboOpen] = useState(false);

  const mapLeadRoleToSignerRole = (leadRole: string, dealType?: string): string | null => {
    switch (leadRole) {
      case "buyer":
        return dealType === "SALE" ? "comprador" : "locatario";
      case "owner":
        return dealType === "SALE" ? "vendedor" : "locador";
      case "tenant":
        return "inquilino";
      case "guarantor":
        return dealType === "SALE" ? "avalista" : "fiador";
      default:
        return null;
    }
  };

  const getAutoRoleByContext = (dealType?: string): string => {
    return dealType === "SALE" ? "comprador" : "inquilino";
  };

  const handleSelectLead = (lead: ApiLead) => {
    setSelectedLead(lead);
    setName(lead.name);
    setEmail(lead.email);
    if (lead.role) {
      const signerRole = mapLeadRoleToSignerRole(lead.role, dealDetails?.type);
      if (signerRole) {
        setRole(signerRole);
      } else {
        setRole(getAutoRoleByContext(dealDetails?.type));
      }
    } else {
      setRole(getAutoRoleByContext(dealDetails?.type));
    }
    setLeadComboOpen(false);
  };

  const handleClearSelection = () => {
    setSelectedLead(null);
    setName("");
    setEmail("");
    setManualEntryMode(false);
  };

  const handleEnterManualMode = () => {
    setManualEntryMode(true);
    setSelectedLead(null);
    setLeadComboOpen(false);
  };

  useEffect(() => {
    if (!leadComboOpen) return;

    if (!searchLeadTerm || searchLeadTerm.length < 2) {
      if (suggestedLeads.length === 0) {
        setSearchingLeads(true);
        leadService
          .getAll({ limit: 10 })
          .then((data) => setSuggestedLeads(data.data || []))
          .catch((err) => console.error("Error loading leads:", err))
          .finally(() => setSearchingLeads(false));
      }
      return;
    }

    const timer = setTimeout(() => {
      setSearchingLeads(true);
      leadService
        .getAll({ search: searchLeadTerm, limit: 10 })
        .then((data) => setSuggestedLeads(data.data || []))
        .catch((err) => console.error("Error searching leads:", err))
        .finally(() => setSearchingLeads(false));
    }, 300);

    return () => clearTimeout(timer);
  }, [searchLeadTerm, leadComboOpen]);

  if (loadingContract) {
    return (
      <div className="flex items-center justify-center py-16 text-muted-foreground">
        <Loader2 className="w-6 h-6 animate-spin mr-2" /> Carregando contrato...
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {isMultiParty && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
          <RequirementCard
            eyebrow="Partes"
            status={missingPrimaries ? "Primários pendentes" : "Primários OK"}
            hint={`Defina ${ROLE_LABEL[customerRole] ?? customerRole} primário e Proprietário primário.`}
            blocking={missingPrimaries}
            ctaLabel="Ir"
            onCta={() => onGoToTab("partes")}
          />
          <RequirementCard
            eyebrow="Documentos"
            status={pendingDocs > 0 ? `${pendingDocs} pendente(s)` : "Sem pendências"}
            hint="Solicite os itens pendentes por parte."
            blocking={pendingDocs > 0}
            ctaLabel="Ver"
            onCta={() => onGoToTab("documentacao")}
          />
          <RequirementCard
            eyebrow="Proposta"
            status={!canProceedContract ? "Aceite pendente" : acceptanceNotRequired ? "Aceite não exigido" : "Aceite OK"}
            hint={acceptanceNotRequired
              ? "Nenhuma parte precisa aceitar: os parâmetros da proposta marcaram o aceite como opcional."
              : "É necessário o aceite das partes obrigatórias."}
            blocking={!canProceedContract}
            ctaLabel="Ver"
            onCta={() => onGoToTab("proposta")}
          />
        </div>
      )}

      {!contract || regenerating ? (
        <div className="bg-card p-6 rounded-2xl border border-border shadow-sm">
          <div className="flex items-start gap-4 mb-5">
            <div className="w-14 h-14 bg-primary/10 rounded-2xl flex items-center justify-center flex-shrink-0">
              <FileText className="w-7 h-7 text-primary" />
            </div>
            <div>
              <h3 className="font-bold text-foreground text-lg">{regenerating ? "Regerar contrato" : "Gerar Contrato"}</h3>
              <p className="text-sm text-muted-foreground mt-1">Selecione o template e gere o contrato automaticamente com os dados da esteira.</p>
              {regenerating && <p className="text-sm text-muted-foreground mt-2">Revise a nova minuta antes de salvar. A versão anterior ficará no histórico do mesmo contrato. A vigência e o plano financeiro serão preservados.</p>}
            </div>
          </div>

          <div className="mb-5">
            <label className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-2 block">Template</label>
            {templates.length === 0 ? (
              <div className="p-4 border border-border rounded-lg bg-muted/40 space-y-3">
                <p className="text-sm text-muted-foreground">
                  <span className="font-medium text-foreground">Nenhum modelo de contrato cadastrado.</span>{" "}
                  Sem um modelo não há minuta para gerar nem enviar para assinatura.
                </p>
                {canWriteTemplates ? (
                  <div className="flex flex-wrap items-center gap-2">
                    <Button type="button" size="sm" onClick={() => setCreatingTemplate(true)}>
                      <Plus className="w-4 h-4" /> Criar modelo de contrato
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => void loadTemplates()}
                    >
                      <RefreshCw className="w-4 h-4" /> Já criei — atualizar lista
                    </Button>
                    <span className="text-xs text-muted-foreground">
                      O editor de modelos abre aqui mesmo; o negócio continua aberto atrás.
                    </span>
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    Peça a um administrador ou gestor para cadastrar o modelo em{" "}
                    <span className="font-medium text-foreground">Modelos de Contrato</span> — criar
                    modelo exige a permissão de editar modelos.
                  </p>
                )}
              </div>
            ) : (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                {templates.map((t) => (
                  <button
                    key={t.id}
                    onClick={() => {
                      if (t.id === selectedTemplateId) return;
                      selectTemplate(t.id);
                    }}
                    className={clsx(
                      "p-3 rounded-xl border-2 text-left transition-all text-sm font-medium",
                      selectedTemplateId === t.id ? "border-primary bg-primary/10 text-primary" : "border-border bg-card text-muted-foreground hover:border-border",
                    )}
                  >
                    <FileText className="w-4 h-4 mb-1" />
                    {t.name}
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="bg-muted rounded-xl p-4 mb-5 border border-border">
            <h4 className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-3">Dados do Contrato</h4>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm">
              <div>
                <span className="text-muted-foreground text-xs">Tipo</span>
                <p className="font-medium text-foreground">{dealDetails?.type === "SALE" ? "Venda" : "Locação"}</p>
              </div>
              <div>
                <span className="text-muted-foreground text-xs">Valor</span>
                <p className="font-medium text-foreground">{(dealDetails?.value || 0).toLocaleString("pt-BR", { style: "currency", currency: "BRL" })}</p>
              </div>
              <div>
                <span className="text-muted-foreground text-xs">Comissão</span>
                <p className="font-medium text-foreground">{dealDetails?.commission || 0}%</p>
              </div>
              <div>
                <span className="text-muted-foreground text-xs">Signatários</span>
                <p className="font-medium text-foreground">{signers.length} pessoa(s)</p>
              </div>
            </div>
            {signers.length === 0 && (
              <p className="text-xs text-status-warning mt-3 flex items-center gap-1">
                <AlertCircle className="w-3.5 h-3.5" />
                Adicione pelo menos um signatário antes de gerar o contrato.
              </p>
            )}
          </div>

          {dealDetails && !regenerating && (
            isLoadingPermissions ? (
              // Evita o "flash" do estado restrito enquanto as permissões do
              // usuário chegam: mostrar restrição por engano é pior do que um
              // instante de espera.
              <div className="mb-5 flex items-center gap-2 rounded-2xl border border-border bg-card px-5 py-4 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                Verificando acesso à área financeira...
              </div>
            ) : canSeeFinancialPlan ? (
              <PreContractFinancialSetup
                deal={dealDetails}
                latestProposal={latestProposal}
                saleDraft={billingDraft}
                saleDraftCurrent={saleDraftCurrent}
                rentalPlan={dealDetails.type === "RENT" ? rentalPreContractPlan : undefined}
                ready={financialReady}
                onConfigureSale={() => setShowBillingDraftWizard(true)}
                onOpenFinancialPreview={() => onGoToTab("financeiro")}
              />
            ) : (
              <FinancialPlanRestricted />
            )
          )}

          {showVariableForm && (
            <div className="mb-5 border border-border rounded-xl bg-card overflow-hidden shadow-sm">
              <PreviewVariableForm
                key={`${selectedTemplateId}:${previewFormRevision}`}
                templateVariables={previewVariableKeys}
                autoFilledData={autoFilledData}
                onConfirm={handleVariableFormConfirm}
                onCancel={() => setShowVariableForm(false)}
              />
            </div>
          )}

          {showPreview && previewFile && (
            <div className={clsx("flex flex-col bg-card", isFullScreen ? "fixed inset-0 z-50 p-4 bg-muted" : "mb-5 border border-border rounded-xl h-[500px]")}>
              <div className="flex justify-between items-center px-4 py-2 border-b border-border bg-card">
                <h4 className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Editor de Contrato</h4>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setIsFullScreen(!isFullScreen)}
                  className="gap-1 text-xs font-medium text-muted-foreground"
                >
                  {isFullScreen ? (
                    <>
                      <Minimize2 className="w-4 h-4" /> Sair da Tela Cheia
                    </>
                  ) : (
                    <>
                      <Maximize2 className="w-4 h-4" /> Tela Cheia
                    </>
                  )}
                </Button>
              </div>
              <div className={clsx("flex-1 overflow-hidden", isFullScreen ? "flex justify-center bg-[#f3f3f3]" : "")}>
                <div className={clsx("h-full overflow-y-auto", isFullScreen ? "w-full max-w-[850px] bg-card shadow-xl" : "w-full")}>
                  <div className="min-h-full">
                    <SuperDocEditor ref={editorRef} document={previewFile} user={{ name: "Admin", email: "admin@suitee.com" }} isInternal={true} hideToolbar={false} />
                  </div>
                </div>
              </div>
            </div>
          )}

          {flags.multiPartyUI && latestProposal && !canProceedContract && (
            <div className="p-4 bg-status-warning-soft border border-status-warning/30 rounded-xl text-sm text-status-warning">É necessário o aceite das partes obrigatórias para gerar o contrato.</div>
          )}
          <div className="flex gap-3">
            {regenerating && (
              <Button variant="outline" disabled={generating} onClick={() => {
                setRegenerating(false); setShowPreview(false); setPreviewFile(null); setShowVariableForm(false); setIsFullScreen(false);
              }}>Cancelar</Button>
            )}
            {!showPreview ? (
              <Button
                variant="outline"
                size="lg"
                onClick={handlePreviewContract}
                disabled={templates.length === 0 || signers.length === 0 || !canProceedContract}
                className="flex-1 border-primary/20 bg-primary/10 font-bold text-primary hover:bg-primary/15 hover:text-primary"
              >
                <Eye className="w-4 h-4" /> Pré-visualizar Contrato
              </Button>
            ) : (
              <Button
                size="lg"
                onClick={handleGenerateContract}
                loading={generating}
                loadingText="Gerando contrato..."
                disabled={regenerating ? !canRegenerate : !financialReady}
                className="flex-1 font-bold"
                title={!regenerating && !financialReady ? financialReadyBlockedHint(canSeeFinancialPlan, dealDetails?.type) : undefined}
              >
                <FileText className="w-4 h-4" /> {regenerating ? "Salvar nova minuta" : "Finalizar e Gerar Contrato"}
              </Button>
            )}
          </div>
        </div>
      ) : (
        <>
        <div className="bg-card p-5 rounded-2xl border border-border shadow-sm">
          <div className="flex flex-col sm:flex-row gap-5 items-start">
            <div className="w-14 h-14 bg-status-ok-soft rounded-2xl flex items-center justify-center flex-shrink-0">
              <FileCheck className="w-7 h-7 text-status-ok" />
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-3 mb-1">
                <h3 className="font-bold text-foreground text-lg">Minuta Gerada</h3>
                <SignatureStatusBadge kind="contract" status={contract.status === "DRAFT" && !contract.sent_for_signature_at ? contract.status : envelope?.status ?? contract.status} hideIcon />
              </div>
              <p className="text-sm text-muted-foreground">
                Contrato gerado em {new Date(contract.created_at).toLocaleDateString("pt-BR")} usando template com dados validados na esteira.
              </p>
              {envelope?.status === "ERROR" && (
                <div role="alert" className="mt-3 rounded-lg border border-status-critical/30 bg-status-critical-soft p-3 text-sm text-status-critical break-words">
                  <p className="font-semibold">Falha no envio para assinatura</p>
                  <p>{envelope.last_error || "O envio não foi concluído."}</p>
                  {failedBeforeEnvelope && <p className="mt-2">Corrija a configuração indicada. Você pode regerar a minuta e enviar novamente.</p>}
                </div>
              )}
              <div className="mt-3">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => navigate(`/contratos/${contract.id}`)}
                  className="gap-1.5 text-xs font-bold"
                >
                  <ExternalLink className="w-3.5 h-3.5" /> Abrir página do contrato
                </Button>
              </div>
              {!contractSigned && (
              <div className="mt-3 flex flex-wrap gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    if (!showPreview) {
                      setIsFullScreen(true);
                      setShowPreview(true);
                    } else {
                      setIsFullScreen(false);
                      setShowPreview(false);
                    }
                  }}
                  className="gap-1.5 bg-primary/10 text-xs font-bold text-primary hover:bg-primary/15 hover:text-primary"
                >
                  <Eye className="w-3.5 h-3.5" /> {showPreview ? "Ocultar Preview" : "Revisar Documento"}
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => {
                    const file = base64ToFile(contract.content, `Contrato_${dealDetails?.id}.docx`);
                    const url = URL.createObjectURL(file);
                    const a = document.createElement("a");
                    a.href = url;
                    a.download = file.name;
                    a.click();
                    URL.revokeObjectURL(url);
                  }}
                  className="gap-1.5 text-xs font-bold text-muted-foreground"
                >
                  <Download className="w-3.5 h-3.5" /> Baixar DOCX
                </Button>
                {contract.status === "DRAFT" && dealDetails?.type === "SALE" && canSeeFinancialPlan && (
                  <Button
                    variant={billingDraft ? "secondary" : "outline"}
                    size="sm"
                    onClick={() => setShowBillingDraftWizard(true)}
                    className="gap-1.5 text-xs font-bold"
                    title={billingDraft
                      ? `Configurada: ${billingDraft.mode === "INSTALLMENTS" ? `${billingDraft.installment_count}x` : "à vista"}`
                      : "Sem forma de recebimento configurada — nenhuma cobrança será fabricada automaticamente"}
                  >
                    <CreditCard className="w-3.5 h-3.5" />
                    {billingDraft ? "Revisar recebimento" : "Configurar recebimento"}
                  </Button>
                )}
                {canRegenerate && (
                  <Button variant="outline" size="sm" loading={preparingRegeneration} disabled={sendingForSignature} onClick={beginRegeneration} className="gap-1.5 text-xs font-bold">
                    <RefreshCw className="w-3.5 h-3.5" /> Regerar contrato
                  </Button>
                )}
                {canSend && (
                  <Button
                    variant="success"
                    size="sm"
                    loading={sendingForSignature}
                    loadingText="Enviando..."
                    onClick={() => setSendForSignatureOpen(true)}
                    disabled={dealDetails?.type === "SALE" && !saleDraftCurrent}
                    className="gap-1.5 text-xs font-bold"
                    title={
                      dealDetails?.type === "SALE" && !saleDraftCurrent
                        ? canSeeFinancialPlan
                          ? "Revise a forma de recebimento antes de enviar"
                          : "A área financeira ainda não concluiu o recebimento desta venda"
                        : undefined
                    }
                  >
                    <Send className="w-3.5 h-3.5" />
                    Enviar para Assinatura
                  </Button>
                )}
              </div>
              )}
              {Boolean(contract.draft_revisions?.length) && (
                <details className="mt-3 text-sm">
                  <summary className="cursor-pointer font-semibold">Versões anteriores da minuta ({contract.draft_revisions!.length})</summary>
                  <ul className="mt-2 space-y-2">
                    {contract.draft_revisions!.map((revision, index) => (
                      <li key={`${revision.replaced_at}:${index}`}>
                        <Button variant="outline" size="sm" onClick={() => {
                          const file = base64ToFile(revision.content, `Contrato_${contract.id}_versao_${index + 1}.docx`);
                          const url = URL.createObjectURL(file);
                          const link = document.createElement("a");
                          link.href = url; link.download = file.name; link.click(); URL.revokeObjectURL(url);
                        }}>
                          <Download className="w-3.5 h-3.5" /> Versão {index + 1} · substituída em {new Date(revision.replaced_at).toLocaleString("pt-BR")}
                        </Button>
                      </li>
                    ))}
                  </ul>
                </details>
              )}
              {contract.status === "DRAFT" && dealDetails?.type === "SALE" && canSeeFinancialPlan && !billingDraft && (
                <div className="mt-2 flex items-start gap-2 rounded-lg border border-status-warning/30 bg-status-warning-soft px-3 py-2 text-xs text-status-warning">
                  <AlertCircle className="w-3.5 h-3.5 flex-shrink-0 mt-0.5" />
                  <p>
                    Sem forma de recebimento configurada. O financeiro <strong>não cria uma cobrança automática</strong>{" "}
                    sem uma escolha explícita. Configure entrada, parcelas, saldo ou dação antes de enviar para assinatura.
                  </p>
                </div>
              )}
              {contract.status === "DRAFT" && dealDetails?.type === "SALE" && canSeeFinancialPlan && billingDraft && !saleDraftCurrent && (
                <div className="mt-2 flex items-start gap-2 rounded-lg border border-status-critical/30 bg-status-critical-soft px-3 py-2 text-xs text-status-critical">
                  <AlertCircle className="w-3.5 h-3.5 flex-shrink-0 mt-0.5" />
                  <p>
                    A forma de recebimento foi calculada para outro valor. Revise o plano antes de enviar a minuta para assinatura.
                  </p>
                </div>
              )}
            </div>
          </div>

          <AnimatePresence>
            {showPreview && contract.content && !contractSigned && (
              <motion.div
                initial={{ height: 0, opacity: 0 }}
                animate={{ height: "auto", opacity: 1 }}
                exit={{ height: 0, opacity: 0 }}
                className={clsx("overflow-hidden", isFullScreen ? "fixed inset-0 z-50 flex flex-col bg-muted p-4" : "")}
              >
                {isFullScreen && (
                  <div className="flex justify-between items-center px-4 py-2 mb-4 border-b border-border bg-card rounded-xl shadow-sm">
                    <h4 className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Visualização do Contrato</h4>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setIsFullScreen(false)}
                      className="gap-1 text-xs font-medium text-muted-foreground"
                    >
                      <Minimize2 className="w-4 h-4" /> Sair da Tela Cheia
                    </Button>
                  </div>
                )}

                <div className={clsx("flex-1 overflow-hidden", isFullScreen ? "flex justify-center bg-[#f3f3f3]" : "mt-4 border border-border rounded-xl h-[500px]")}>
                  <div className={clsx("h-full overflow-y-auto", isFullScreen ? "w-full max-w-[850px] bg-card shadow-xl" : "w-full")}>
                    <div className="min-h-full">
                      <SuperDocEditor document={base64ToFile(contract.content, "contrato.docx")} user={{ name: "Admin", email: "admin@suitee.com" }} isInternal={true} hideToolbar={true} />
                    </div>
                  </div>
                </div>
              </motion.div>
            )}
          </AnimatePresence>
        </div>
        {contractSigned && <SignedDocumentsSection contract={contract} />}
        </>
      )}

      <ContractSignatureSection
        dealDetails={dealDetails}
        contract={contract}
        signers={signers}
        signature={signatureEnvelope}
        inviteSigner={inviteSigner}
        newSigner={{
          isAdding,
          setIsAdding,
          name,
          setName,
          email,
          setEmail,
          role,
          setRole,
          manualEntryMode,
          setManualEntryMode,
          selectedLead,
          leadComboOpen,
          setLeadComboOpen,
          searchLeadTerm,
          setSearchLeadTerm,
          suggestedLeads,
          setSuggestedLeads,
          searchingLeads,
          onSelectLead: handleSelectLead,
          onClearSelection: handleClearSelection,
          onEnterManualMode: handleEnterManualMode,
          onCancel: () => {
            setIsAdding(false);
            setManualEntryMode(false);
            setSelectedLead(null);
            setSuggestedLeads([]);
          },
          onAddSigner: handleAddSigner,
        }}
      />
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div className="bg-foreground rounded-2xl p-5 text-white flex items-center gap-4">
          <div className="w-12 h-12 bg-card/10 rounded-full flex items-center justify-center">
            <Lock className="w-6 h-6 text-primary-300" />
          </div>
          <div>
            <h4 className="font-bold">Cofre Digital</h4>
            <p className="text-xs text-muted-foreground mt-0.5">Criptografia AES-256 ativa.</p>
          </div>
        </div>
        <div className="bg-card rounded-2xl p-5 border border-border flex items-center gap-4 shadow-sm">
          <div className={clsx("w-12 h-12 rounded-full flex items-center justify-center", financialReady || !canSeeFinancialPlan ? "bg-muted" : "bg-status-warning-soft")}>
            {!canSeeFinancialPlan ? (
              <Lock className="w-6 h-6 text-muted-foreground" />
            ) : financialReady ? (
              <CheckCircle2 className="w-6 h-6 text-status-ok" />
            ) : (
              <AlertCircle className="w-6 h-6 text-status-warning" />
            )}
          </div>
          <div>
            <h4 className="font-bold text-foreground">Materialização após assinatura</h4>
            <p className="text-xs text-muted-foreground mt-0.5">
              {!canSeeFinancialPlan
                ? "Acompanhamento financeiro restrito ao seu perfil."
                : financialReady
                  ? "Recebíveis e obrigações têm base configurada."
                  : "Plano financeiro ainda pendente."}
            </p>
          </div>
        </div>
      </div>

      {contract && (
        <SendForSignatureModal
          open={sendForSignatureOpen}
          onClose={() => setSendForSignatureOpen(false)}
          contract={contract}
          onSubmit={handleSendForSignature}
        />
      )}
      {showBillingDraftWizard && dealDetails?.type === "SALE" && dealDetails?.id && (
        <BillingPlanWizard
          mode="draft"
          dealId={dealDetails.id}
          contractId=""
          contractType="SALE"
          defaultTotalAmount={dealDetails.value}
          onClose={() => setShowBillingDraftWizard(false)}
          onSaved={() => {
            refreshBillingDraft();
            queryClient.invalidateQueries({ queryKey: ["financial-preview"] });
            setShowBillingDraftWizard(false);
          }}
        />
      )}

      {/* Cadastro do primeiro modelo de contrato, no mesmo editor da tela
          "Modelos de Contrato" (modo embutido), por cima do negócio: o painel
          do contrato e o negócio continuam abertos atrás e o modelo salvo já
          entra selecionado. */}
      <ExpandableSidePanel
        open={creatingTemplate}
        onClose={() => setCreatingTemplate(false)}
        defaultSize="full"
        expandedSize="full"
        zIndex={LAYER.panelTop}
        ariaLabel="Novo modelo de contrato"
      >
        {creatingTemplate && (
          <Suspense
            fallback={
              <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Carregando editor de modelos…
              </div>
            }
          >
            <ContractTemplateEditor
              embedded
              onExit={() => setCreatingTemplate(false)}
              onSaved={(created) => {
                setCreatingTemplate(false);
                void loadTemplates().then(() => selectTemplate(created.id));
                toast.success(`Modelo "${created.name}" criado e selecionado para gerar a minuta.`);
              }}
            />
          </Suspense>
        )}
      </ExpandableSidePanel>
    </div>
  );
}

import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { useFieldArray, useForm, type SubmitHandler } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import * as z from "zod";
import {
  ArrowLeft,
  ArrowRight,
  Briefcase,
  Building2,
  Check,
  Image as ImageIcon,
  Loader2,
  MapPin,
  Plus,
  Trash2,
  User,
  Users,
  X,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { ExpandableSidePanel } from "./ExpandableSidePanel";
import { PropertyPicker } from "./PropertyPicker";
import { LeadPicker } from "./LeadPicker";
import { DeferredMediaSection } from "./DeferredMediaSection";
import { useAgentsContext } from "../context/AgentsContext";
import { useAuth } from "../context/AuthContext";
import { useLeadsContext } from "../context/LeadsContext";
import { usePropertiesContext } from "../context/PropertiesContext";
import {
  captacaoService,
  type CaptacaoCapturingBrokerInput,
  type CaptacaoOwnerInput,
} from "../services/captacao.service";
import { propertyService } from "../services/property.service";
import { leadService } from "../services/lead.service";
import { unwrapApiData } from "../services/api";
import { notifyApiError } from "../services/errorToast";
import { mapApiLeadToLead } from "../lib/leads/mappers";
import { mergeOwnerOptions, ensureSelectedOwner } from "../lib/leads/ownerOptions";
import type { Lead } from "../data";
import type { PropertyDTO } from "../services/property.service";
import { formatCurrencyBRL0 } from "../utils/format";
import { MoneyInput } from "../ui/money-input";
import { Form } from "../ui/form";
import { CepField, PercentField, SelectField, TextareaField, TextField } from "../ui/fields";
import { lookupCep } from "../services/cep.service";
import { formatCep, onlyDigits } from "../utils/masks";
import { stateNameFromCode } from "../utils/uf";

const ORIGIN = ["novo", "existente"] as const;
const PROPERTY_TYPES = ["apartment", "house", "land", "commercial"] as const;
const LISTING_TYPES = ["sale", "rent", "exchange"] as const;
const CONDITIONS = ["new", "used", "under_construction"] as const;

const BROKERAGE_TYPES = ["captacao", "atendimento", "visita"] as const;
type BrokerageType = (typeof BROKERAGE_TYPES)[number];

const BROKERAGE_TYPE_LABELS: Record<BrokerageType, string> = {
  captacao: "Captação",
  atendimento: "Atendimento",
  visita: "Visita",
};

const BROKERAGE_TYPE_HINT: Record<BrokerageType, string> = {
  captacao: "Corretor que captou o imóvel",
  atendimento: "Corretor responsável pelo lead",
  visita: "Corretor que conduziu a visita",
};

const PROPERTY_TYPE_OPTIONS = [
  { value: "apartment", label: "Apartamento" },
  { value: "house", label: "Casa" },
  { value: "land", label: "Terreno" },
  { value: "commercial", label: "Comercial" },
];
const LISTING_TYPE_OPTIONS = [
  { value: "sale", label: "Venda" },
  { value: "rent", label: "Locação" },
  { value: "exchange", label: "Permuta" },
];
const CONDITION_OPTIONS = [
  { value: "new", label: "Novo" },
  { value: "used", label: "Usado" },
  { value: "under_construction", label: "Em construção" },
];
const BROKERAGE_TYPE_OPTIONS = BROKERAGE_TYPES.map((t) => ({ value: t, label: BROKERAGE_TYPE_LABELS[t] }));

function brokerageTypeToLegacyRole(t: BrokerageType): "captador" | "atendente" | "visitador" {
  switch (t) {
    case "captacao":
      return "captador";
    case "atendimento":
      return "atendente";
    case "visita":
      return "visitador";
  }
}

const captadorSchema = z.object({
  broker_id: z.string().min(1, "Selecione um corretor"),
  brokerage_type: z.enum(BROKERAGE_TYPES),
  commission_pct: z.number().min(0, "Comissão não pode ser negativa").max(100, "Máximo 100%"),
  creci: z.string().optional(),
});

const mediaImageSchema = z.object({
  url: z.string().optional(),
  pending_file: z.any().optional(),
  description: z.string().optional(),
});

const captacaoSchema = z
  .object({
    origin: z.enum(ORIGIN),
    owner_lead_id: z.string().min(1, "Selecione um proprietário"),
    existing_property_id: z.string().optional(),

    title: z.string().optional(),
    property_type: z.enum(PROPERTY_TYPES).optional(),
    listing_type: z.enum(LISTING_TYPES).optional(),
    condition: z.enum(CONDITIONS).optional(),
    value_cents: z.number().optional(),
    description: z.string().optional(),

    address_postal_code: z.string().optional(),
    address_street: z.string().optional(),
    address_number: z.string().optional(),
    address_complement: z.string().optional(),
    address_neighborhood: z.string().optional(),
    address_city: z.string().optional(),
    address_state: z.string().optional(),
    address_state_code: z.string().optional(),

    capturing_brokers: z.array(captadorSchema).min(1, "Adicione ao menos um captador"),
    images: z.array(mediaImageSchema).optional(),
  })
  .superRefine((data, ctx) => {
    if (data.origin === "existente") {
      if (!data.existing_property_id) {
        ctx.addIssue({
          path: ["existing_property_id"],
          code: z.ZodIssueCode.custom,
          message: "Selecione um imóvel cadastrado",
        });
      }
      return;
    }
    if (!data.title || data.title.trim().length < 3) {
      ctx.addIssue({ path: ["title"], code: z.ZodIssueCode.custom, message: "Título deve ter ao menos 3 caracteres" });
    }
    if (!data.property_type) {
      ctx.addIssue({ path: ["property_type"], code: z.ZodIssueCode.custom, message: "Tipo é obrigatório" });
    }
    if (!data.listing_type) {
      ctx.addIssue({ path: ["listing_type"], code: z.ZodIssueCode.custom, message: "Operação é obrigatória" });
    }
    if (!data.condition) {
      ctx.addIssue({ path: ["condition"], code: z.ZodIssueCode.custom, message: "Condição é obrigatória" });
    }
    if (!data.value_cents || data.value_cents <= 0) {
      ctx.addIssue({ path: ["value_cents"], code: z.ZodIssueCode.custom, message: "Valor deve ser positivo" });
    }
    const postalDigits = onlyDigits(data.address_postal_code || "");
    if (!postalDigits) {
      ctx.addIssue({ path: ["address_postal_code"], code: z.ZodIssueCode.custom, message: "Informe o CEP do imóvel" });
    } else if (postalDigits.length !== 8) {
      ctx.addIssue({
        path: ["address_postal_code"],
        code: z.ZodIssueCode.custom,
        message: `CEP incompleto (${postalDigits.length}/8 dígitos) — ex.: 01310-100`,
      });
    }
    if (!data.address_street?.trim()) {
      ctx.addIssue({ path: ["address_street"], code: z.ZodIssueCode.custom, message: "Logradouro é obrigatório" });
    }
    if (!data.address_number?.trim()) {
      ctx.addIssue({ path: ["address_number"], code: z.ZodIssueCode.custom, message: "Número é obrigatório" });
    }
    if (!data.address_neighborhood?.trim()) {
      ctx.addIssue({ path: ["address_neighborhood"], code: z.ZodIssueCode.custom, message: "Bairro é obrigatório" });
    }
    if (!data.address_city?.trim()) {
      ctx.addIssue({ path: ["address_city"], code: z.ZodIssueCode.custom, message: "Cidade é obrigatória" });
    }
    if (!data.address_state_code || data.address_state_code.length !== 2) {
      ctx.addIssue({ path: ["address_state_code"], code: z.ZodIssueCode.custom, message: "UF deve ter 2 letras" });
    }
  });

type CaptacaoFormValues = z.infer<typeof captacaoSchema>;

type Step = 1 | 2 | 3 | 4 | 5 | 6;
const TOTAL_STEPS = 6;

interface CaptacaoWizardProps {
  open: boolean;
  onClose: () => void;
  initialOwnerLeadId?: string;
}

export function CaptacaoWizard({ open, onClose, initialOwnerLeadId }: CaptacaoWizardProps) {
  const navigate = useNavigate();
  const { leads, updateLeadStatus } = useLeadsContext();
  const { agents } = useAgentsContext();
  const { user: authUser } = useAuth();
  const { properties, fetchProperties } = usePropertiesContext();
    const handleCepEnrich = async (cep: string) => {
    try {
      const result = await lookupCep(cep);
      if (result) {
        let filled = false;
        if (result.street) { form.setValue("address_street", result.street, { shouldValidate: false }); filled = true; }
        if (result.neighborhood) { form.setValue("address_neighborhood", result.neighborhood, { shouldValidate: false }); filled = true; }
        if (result.city) { form.setValue("address_city", result.city, { shouldValidate: false }); filled = true; }
        if (result.stateCode) { form.setValue("address_state_code", result.stateCode, { shouldValidate: false }); filled = true; }
        // Só depois da consulta valida os campos preenchidos (evita erro durante o load).
        if (filled) {
          form.trigger(["address_street", "address_neighborhood", "address_city", "address_state_code"]).catch(() => {});
        }
      }
    } catch { /* silencioso — mantém preenchimento manual */ }
  };

const [step, setStep] = useState<Step>(1);
  const [submitting, setSubmitting] = useState(false);
  const [fetchedOwners, setFetchedOwners] = useState<Lead[]>([]);
  const [ownersLoading, setOwnersLoading] = useState(false);

  const form = useForm<CaptacaoFormValues>({
    resolver: zodResolver(captacaoSchema),
    defaultValues: {
      origin: "novo",
      owner_lead_id: initialOwnerLeadId ?? "",
      property_type: "apartment",
      listing_type: "sale",
      condition: "used",
      value_cents: 0,
      capturing_brokers: [{ broker_id: authUser?.id ?? "", brokerage_type: "captacao", commission_pct: 100, creci: "" }],
      address_state_code: "",
      images: [],
    },
  });

  const { fields: brokerFields, append: appendBroker, remove: removeBroker } =
    useFieldArray({ control: form.control, name: "capturing_brokers" });

  const origin = form.watch("origin");
  const existingPropertyId = form.watch("existing_property_id");
  const listingType = form.watch("listing_type");
  const valueCents = form.watch("value_cents") ?? 0;
  const capturingBrokers = form.watch("capturing_brokers");
  const ownerLeadId = form.watch("owner_lead_id");
  const totalCommission = (capturingBrokers ?? []).reduce(
    (acc, b) => acc + (Number.isFinite(b?.commission_pct) ? Number(b.commission_pct) : 0),
    0,
  );

  const agentOptions = useMemo(() => {
    const opts = agents.map((a) => ({ value: a.id, label: a.name }));
    // Garante opção no seletor para captadores referenciados por imóveis,
    // mesmo se o cache de agentes não os incluir (senão o campo fica em branco).
    const known = new Set(agents.map((a) => a.id));
    for (const p of properties) {
      for (const b of p.capturingBrokers ?? []) {
        if (!b.id || known.has(b.id)) continue;
        known.add(b.id);
        opts.push({ value: b.id, label: `Corretor (${b.id.slice(0, 8)})` });
      }
    }
    return opts;
  }, [agents, properties]);

  const ownerLeads = useMemo(
    () => leads.filter((l) => l.leadType === "captacao" || l.role === "owner"),
    [leads],
  );

  // Merge the (possibly stale) in-memory owner leads with a fresh server fetch so
  // the most recently created captação leads always appear (deduped, newest-first).
  const ownerOptions = useMemo(() => mergeOwnerOptions(ownerLeads, fetchedOwners), [ownerLeads, fetchedOwners]);

  // Guarantee the currently-selected owner stays visible even if it fell outside
  // the fetched page (e.g. a preset initialOwnerLeadId).
  const ownerPickerOptions = useMemo(
    () => ensureSelectedOwner(ownerOptions, ownerLeadId, leads),
    [ownerOptions, ownerLeadId, leads],
  );

  const propertyOptions = useMemo(
    () =>
      properties.map((p) => ({
        id: p.id,
        title: p.title,
        displayId: (p as any).display_id || (p as any).displayId,
        address: typeof p.address === "object" ? (p.address as any) : undefined,
      })),
    [properties],
  );

  // Auto-carrega o captador do imóvel selecionado (fluxo "imóvel existente")
  // na etapa de equipe/comissão: broka_id, tipo captação, % e CRECI.
  // Aplica uma única vez por seleção — refetch do contexto não sobrescreve
  // edições do operador.
  const prefilledPropertyRef = useRef<string | null>(null);
  useEffect(() => {
    if (origin !== "existente" || !existingPropertyId) {
      prefilledPropertyRef.current = null;
      return;
    }
    if (prefilledPropertyRef.current === existingPropertyId) return;
    prefilledPropertyRef.current = existingPropertyId;
    const prop = properties.find((p) => p.id === existingPropertyId);
    const refs = prop?.capturingBrokers ?? [];
    if (refs.length === 0) {
      // Imóvel sem captador registrado — mantém a linha padrão para escolha manual.
      return;
    }
    form.setValue(
      "capturing_brokers",
      refs.map((b) => ({
        broker_id: b.id,
        brokerage_type: "captacao",
        commission_pct: b.commissionPct ?? 100,
        creci: b.creci ?? "",
      })),
      { shouldValidate: true },
    );
  }, [origin, existingPropertyId, properties, form]);

  useEffect(() => {
    if (open) {
      form.reset({
        origin: "novo",
        owner_lead_id: initialOwnerLeadId ?? "",
        property_type: "apartment",
        listing_type: "sale",
        condition: "used",
        value_cents: 0,
        capturing_brokers: [{ broker_id: authUser?.id ?? "", brokerage_type: "captacao", commission_pct: 100, creci: "" }],
        address_state_code: "",
        images: [],
      });
      setStep(1);
    }
  }, [open, initialOwnerLeadId, form, authUser?.id]);

  // On open, fetch owner leads fresh (server-side role filter) so the latest
  // captação leads show even when the in-memory context list is stale or the
  // owner falls outside its unfiltered 1000-row window.
  useEffect(() => {
    if (!open) return;
    let ignore = false;
    setOwnersLoading(true);
    leadService
      .getAll({ role: "owner", limit: 500 })
      .then((res) => {
        if (ignore) return;
        const mapped = (Array.isArray(res.data) ? res.data : [])
          .map(mapApiLeadToLead)
          // Defensive: keep only owners in case the backend ignores the role filter.
          .filter((l) => l.role === "owner" || l.leadType === "captacao");
        setFetchedOwners(mapped);
      })
      .catch((e) => {
        if (ignore) return;
        notifyApiError(e, { context: "Carregar proprietários", silent: true });
      })
      .finally(() => {
        if (!ignore) setOwnersLoading(false);
      });
    return () => {
      ignore = true;
    };
  }, [open]);

  const handleClose = () => {
    setStep(1);
    setSubmitting(false);
    onClose();
  };

  // NOTA: não há mais a divisão automática 50/50 captador/atendente. Na
  // captação para o proprietário o atendente é o próprio captador — a
  // comissão padrão fica 100% no captador; qualquer rateio é feito
  // manualmente pelo operador (item a item, via "Adicionar participante").

  const goNext = async () => {
    const fieldsByStep: Record<Step, (keyof CaptacaoFormValues)[]> = {
      1: ["origin"],
      2: ["owner_lead_id"],
      3:
        origin === "existente"
          ? ["existing_property_id"]
          : ["title", "property_type", "listing_type", "condition", "value_cents"],
      4:
        origin === "existente"
          ? []
          : ["address_postal_code", "address_street", "address_number", "address_neighborhood", "address_city", "address_state_code"],
      5: [],
      6: ["capturing_brokers"],
    };
    const ok = await form.trigger(fieldsByStep[step]);
    if (!ok) return;
    if (step === 3 && origin === "existente") {
      setStep(6);
      return;
    }
    if (step < TOTAL_STEPS) setStep((step + 1) as Step);
  };

  const goPrev = () => {
    if (step === 6 && origin === "existente") {
      setStep(3);
      return;
    }
    if (step === 1) {
      handleClose();
      return;
    }
    setStep((step - 1) as Step);
  };

  const onSubmit: SubmitHandler<CaptacaoFormValues> = async (values) => {
    if (submitting) return;
    setSubmitting(true);
    try {
      let createdId: string | null = null;

      const owners: CaptacaoOwnerInput[] = [{ owner_id: values.owner_lead_id, owner_type: "person", percentage: 100 }];
      const capturing_brokers: CaptacaoCapturingBrokerInput[] = values.capturing_brokers.map((cb) => ({
        broker_id: cb.broker_id,
        role: brokerageTypeToLegacyRole(cb.brokerage_type),
        brokerage_type: cb.brokerage_type,
        commission_pct: cb.commission_pct,
        creci: cb.creci || undefined,
      }));

      if (values.origin === "existente" && values.existing_property_id) {
        const propId = values.existing_property_id;
        await propertyService.addOwner(propId, {
          owner_id: values.owner_lead_id,
          owner_type: "person",
          percentage: 100,
        });
        await Promise.all(
          capturing_brokers.map((cb) =>
            propertyService.addCapturingBroker(propId, {
              broker_id: cb.broker_id,
              role: cb.role,
              brokerage_type: cb.brokerage_type,
              commission_pct: cb.commission_pct,
              creci: cb.creci,
            }),
          ),
        );
        createdId = propId;
      } else {
        const valueAmount = (values.value_cents ?? 0) / 100;

        const images = values.images ?? [];
        const externalImages = images
          .filter((i) => !i.pending_file && i.url && !i.url.startsWith("blob:"))
          .map((img, idx) => ({
            url: img.url!,
            description: img.description ?? "",
            order: idx,
          }));
        const pendingFiles = images.filter((i) => i.pending_file).map((i) => i.pending_file as File);

        const property: any = {
          title: values.title!.trim(),
          status: "prospecting",
          publication_status: "draft",
          property_type: values.property_type,
          listing_type: values.listing_type,
          condition: values.condition,
          description: values.description?.trim() || undefined,
          address: {
            street: values.address_street!.trim(),
            number: values.address_number!.trim(),
            complement: values.address_complement?.trim() || "",
            neighborhood: values.address_neighborhood!.trim(),
            city: values.address_city!.trim(),
            state: values.address_state?.trim() || stateNameFromCode(values.address_state_code || ""),
            state_code: values.address_state_code!.toUpperCase(),
            postal_code: formatCep(values.address_postal_code!),
            country: "Brasil",
            country_code: "BR",
          },
          pricing:
            values.listing_type === "rent"
              ? { rent_price: { current_amount: valueAmount, currency: "BRL" } }
              : { sale_price: { current_amount: valueAmount, currency: "BRL" } },
          ...(externalImages.length > 0 ? { media: { images: externalImages } } : {}),
        };

        const idemKey = `captacao-${values.owner_lead_id}-${Date.now()}`;
        const res = await captacaoService.criar({ property, owners, capturing_brokers }, idemKey);
        const created = unwrapApiData<PropertyDTO>(res);
        createdId = created?.id ?? null;

        if (createdId && pendingFiles.length > 0) {
          const results = await Promise.allSettled(pendingFiles.map((f) => propertyService.uploadImage(createdId!, f)));
          const failed = results.filter((r) => r.status === "rejected").length;
          if (failed > 0) {
            toast.error(`${failed} imagem(ns) não foram enviadas. Tente novamente em PropertyDetails.`);
          }
        }
      }

      try {
        await leadService.convertToClient(values.owner_lead_id, { role: "owner" });
        await updateLeadStatus(values.owner_lead_id, "Convertido");
      } catch (e) {
        console.warn("convertToClient post-captação falhou (lead pode já estar convertido)", e);
      }

      await fetchProperties();

      toast.success("Captação registrada com sucesso");
      handleClose();
      if (createdId) {
        navigate(`/imoveis/${createdId}`);
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Erro ao registrar captação";
      toast.error(msg);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <ExpandableSidePanel
      open={open}
      onClose={handleClose}
      defaultSize="2xl"
      expandedSize="full"
      renderHeader={({ closeButton, expandButton }) => (
        <div className="flex-none px-6 py-4 border-b border-border bg-card flex items-center justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div className="w-9 h-9 rounded-lg bg-primary-50 flex items-center justify-center shrink-0">
              <Briefcase className="w-5 h-5 text-primary-600" />
            </div>
            <div className="min-w-0">
              <h2 className="text-base font-bold text-foreground">Iniciar Captação</h2>
              <p className="text-xs text-muted-foreground">
                Passo {step} de {TOTAL_STEPS}
                {" — "}
                {step === 1 && "Origem do imóvel"}
                {step === 2 && "Proprietário"}
                {step === 3 && (origin === "existente" ? "Selecionar imóvel" : "Imóvel: dados básicos")}
                {step === 4 && "Endereço"}
                {step === 5 && "Mídia (opcional)"}
                {step === 6 && "Equipe e comissões"}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            {expandButton}
            {closeButton}
          </div>
        </div>
      )}
    >
      <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="flex flex-col flex-1 min-h-0">
        <div className="flex-none px-6 pt-4 pb-2 border-b border-border bg-muted/40">
          <div className="flex items-center gap-1.5">
            {[1, 2, 3, 4, 5, 6].map((n) => {
              const skip = origin === "existente" && (n === 4 || n === 5);
              return (
                <div key={n} className={clsx("flex items-center gap-1.5 flex-1", skip && "opacity-40")}>
                  <div
                    className={clsx(
                      "w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold border-2 transition-colors shrink-0",
                      step >= n ? "bg-primary text-primary-foreground border-primary" : "bg-card text-muted-foreground border-border",
                    )}
                  >
                    {step > n ? <Check className="w-3.5 h-3.5" /> : n}
                  </div>
                  {n < TOTAL_STEPS && <div className={clsx("flex-1 h-0.5", step > n ? "bg-primary-600" : "bg-border")} />}
                </div>
              );
            })}
          </div>
        </div>

        <div className="flex-1 min-h-0 overflow-y-auto px-6 py-5 space-y-5">
          {step === 1 && (
            <section className="space-y-3">
              <SectionTitle
                icon={Briefcase}
                title="Origem do imóvel"
                subtitle="Você está captando um imóvel novo ou anexando proprietário a um já cadastrado?"
              />
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                {ORIGIN.map((o) => {
                  const checked = origin === o;
                  return (
                    <label
                      key={o}
                      className={clsx(
                        "rounded-xl border-2 p-4 cursor-pointer transition-all",
                        checked ? "border-primary-600 bg-primary-50" : "border-border hover:border-primary-200",
                      )}
                    >
                      <input type="radio" value={o} {...form.register("origin")} className="sr-only" />
                      <div className="flex items-start gap-2">
                        <div
                          className={clsx(
                            "w-5 h-5 rounded-full border-2 mt-0.5 flex items-center justify-center shrink-0",
                            checked ? "border-primary-600" : "border-border",
                          )}
                        >
                          {checked && <div className="w-2.5 h-2.5 rounded-full bg-primary-600" />}
                        </div>
                        <div>
                          <p className="text-sm font-bold text-foreground">{o === "novo" ? "Imóvel novo" : "Imóvel já cadastrado"}</p>
                          <p className="text-xs text-muted-foreground mt-0.5">
                            {o === "novo"
                              ? "Cadastrar imóvel + owner + captador num único fluxo."
                              : "Adicionar owner/captador a uma property existente."}
                          </p>
                        </div>
                      </div>
                    </label>
                  );
                })}
              </div>
            </section>
          )}

          {step === 2 && (
            <section className="space-y-3">
              <SectionTitle icon={User} title="Proprietário" subtitle="Lead com tipo Captação ou role=owner." />
              <LeadPicker
                leads={ownerPickerOptions}
                value={ownerLeadId}
                onChange={(id) => form.setValue("owner_lead_id", id, { shouldValidate: true })}
                placeholder={ownersLoading ? "Carregando proprietários…" : "Selecione um proprietário…"}
                inputPlaceholder="Buscar por nome, e-mail ou telefone…"
                emptyLabel={ownersLoading ? "Carregando…" : "Nenhum lead-captação encontrado."}
              />
              <FieldError error={form.formState.errors.owner_lead_id?.message} />
              <p className="text-xs text-muted-foreground">Não encontrou? Cadastre antes pelo botão "Novo Lead" (tipo Captação).</p>
            </section>
          )}

          {step === 3 && origin === "existente" && (
            <section className="space-y-3">
              <SectionTitle icon={Building2} title="Imóvel já cadastrado" subtitle="Selecione na lista de imóveis da empresa." />
              <PropertyPicker
                properties={propertyOptions}
                value={form.watch("existing_property_id")}
                onChange={(id) => form.setValue("existing_property_id", id, { shouldValidate: true })}
                placeholder="Selecione um imóvel…"
                includeNone={false}
              />
              <FieldError error={form.formState.errors.existing_property_id?.message} />
            </section>
          )}

          {step === 3 && origin === "novo" && (
            <section className="space-y-4">
              <SectionTitle
                icon={Building2}
                title="Imóvel"
                subtitle="Dados básicos. Detalhes (fotos, áreas, comodidades) ficam para depois em PropertyDetails."
              />
              <TextField
                control={form.control}
                name="title"
                label="Título"
                placeholder="Ex.: Apartamento Vila Olímpia 75m²"
              />
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <SelectField control={form.control} name="property_type" label="Tipo" options={PROPERTY_TYPE_OPTIONS} />
                <SelectField control={form.control} name="listing_type" label="Operação" options={LISTING_TYPE_OPTIONS} />
                <SelectField control={form.control} name="condition" label="Condição" options={CONDITION_OPTIONS} />
              </div>
              <Field
                label={listingType === "rent" ? "Valor do aluguel mensal (R$)" : "Valor do imóvel (R$)"}
                error={form.formState.errors.value_cents?.message}
                hint={valueCents > 0 ? formatCurrencyBRL0(valueCents / 100) : undefined}
              >
                <MoneyInput
                  fractionDigits={0}
                  value={valueCents > 0 ? valueCents / 100 : undefined}
                  onChange={(v) => {
                    form.setValue("value_cents", Math.round((v ?? 0) * 100), { shouldValidate: true });
                  }}
                  placeholder="850.000"
                  className="w-full px-3 py-2.5 rounded-lg border border-border bg-card text-sm focus:ring-2 focus:ring-primary/30 focus:outline-none"
                />
              </Field>
              <TextareaField
                control={form.control}
                name="description"
                label="Descrição (opcional)"
                rows={3}
                placeholder="Detalhes diferenciais do imóvel…"
              />
            </section>
          )}

          {step === 4 && origin === "novo" && (
            <section className="space-y-4">
              <SectionTitle
                icon={MapPin}
                title="Endereço completo"
                subtitle="Necessário para captação. Para autocomplete via CEP, edite depois em PropertyDetails."
              />
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <CepField control={form.control} name="address_postal_code" label="CEP" onCep={handleCepEnrich} />
                <TextField
                  control={form.control}
                  name="address_street"
                  label="Logradouro"
                  placeholder="Rua/Av."
                  className="sm:col-span-2"
                />
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <TextField control={form.control} name="address_number" label="Número" placeholder="123" />
                <TextField
                  control={form.control}
                  name="address_complement"
                  label="Complemento (opcional)"
                  placeholder="Ap. 42, bloco B"
                  className="sm:col-span-2"
                />
              </div>
              <TextField control={form.control} name="address_neighborhood" label="Bairro" />
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <TextField control={form.control} name="address_city" label="Cidade" className="sm:col-span-2" />
                <TextField
                  control={form.control}
                  name="address_state_code"
                  label="UF"
                  placeholder="SP"
                  inputClassName="uppercase"
                  inputProps={{ maxLength: 2 }}
                />
              </div>
            </section>
          )}

          {step === 5 && origin === "novo" && (
            <section className="space-y-3">
              <SectionTitle icon={ImageIcon} title="Mídia" subtitle="Fotos do imóvel. Pode pular e adicionar depois em PropertyDetails." />
              <DeferredMediaSection control={form.control} register={form.register} watch={form.watch} setValue={form.setValue} name="images" />
            </section>
          )}

          {step === 6 && (
            <section className="space-y-3">
              <SectionTitle
                icon={Users}
                title="Equipe e comissões"
                subtitle="A comissão padrão fica 100% no captador. Na captação para o proprietário, o atendente é o próprio captador — use “Adicionar participante” apenas se houver rateio com outras pessoas."
              />
              {brokerFields.map((field, idx) => {
                const tipoAtual = (capturingBrokers?.[idx]?.brokerage_type ?? "captacao") as BrokerageType;
                return (
                  <div key={field.id} className="rounded-xl border border-border p-4 space-y-3 bg-muted/30">
                    <div className="flex items-center justify-between">
                      <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Participante {idx + 1}</p>
                      {brokerFields.length > 1 && (
                        <button
                          type="button"
                          onClick={() => removeBroker(idx)}
                          aria-label={`Remover participante ${idx + 1}`}
                          className="p-1 text-muted-foreground hover:text-status-critical transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 rounded"
                        >
                          <Trash2 className="w-4 h-4" aria-hidden="true" />
                        </button>
                      )}
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                      <SelectField
                        control={form.control}
                        name={`capturing_brokers.${idx}.broker_id`}
                        label="Corretor"
                        placeholder="Selecione…"
                        options={agentOptions}
                      />
                      <SelectField
                        control={form.control}
                        name={`capturing_brokers.${idx}.brokerage_type`}
                        label="Tipo de corretagem"
                        description={BROKERAGE_TYPE_HINT[tipoAtual]}
                        options={BROKERAGE_TYPE_OPTIONS}
                      />
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                      <PercentField
                        control={form.control}
                        name={`capturing_brokers.${idx}.commission_pct`}
                        label="Comissão (%)"
                      />
                      <TextField
                        control={form.control}
                        name={`capturing_brokers.${idx}.creci`}
                        label="CRECI (opcional)"
                        placeholder="Ex.: CRECI-SP 12345"
                      />
                    </div>
                  </div>
                );
              })}
              <button
                type="button"
                onClick={() =>
                  appendBroker({ broker_id: "", brokerage_type: "atendimento", commission_pct: 0, creci: "" })
                }
                className="w-full flex items-center justify-center gap-2 py-2.5 rounded-lg border-2 border-dashed border-border text-sm font-medium text-muted-foreground hover:border-primary-300 hover:text-primary-700 transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500"
              >
                <Plus className="w-4 h-4" aria-hidden="true" /> Adicionar participante
              </button>
              <div
                role="status"
                aria-live="polite"
                className={clsx(
                  "rounded-lg border px-3 py-2 flex items-center justify-between text-sm",
                  Math.abs(totalCommission - 100) < 0.01
                    ? "border-status-ok/30 bg-status-ok-soft text-status-ok"
                    : "border-status-warning/30 bg-status-warning-soft text-status-warning",
                )}
              >
                <span className="font-medium">Total de comissão</span>
                <span className="font-bold">
                  {totalCommission.toLocaleString("pt-BR", { maximumFractionDigits: 2 })}%
                  {Math.abs(totalCommission - 100) >= 0.01 && <span className="ml-2 font-normal">(recomendado 100%)</span>}
                </span>
              </div>
            </section>
          )}
        </div>

        <div className="flex-none flex items-center justify-between px-6 py-4 border-t border-border bg-card">
          <button
            type="button"
            onClick={goPrev}
            disabled={submitting}
            className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-muted-foreground hover:text-foreground rounded-lg hover:bg-muted transition-colors disabled:opacity-50"
          >
            {step === 1 ? <X className="w-4 h-4" /> : <ArrowLeft className="w-4 h-4" />}
            {step === 1 ? "Cancelar" : "Voltar"}
          </button>
          {step < TOTAL_STEPS ? (
            <button
              type="button"
              onClick={goNext}
              disabled={submitting}
              className="flex items-center gap-2 px-5 py-2 text-sm font-medium rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors shadow-sm disabled:opacity-50"
            >
              Próximo <ArrowRight className="w-4 h-4" />
            </button>
          ) : (
            <button
              type="submit"
              disabled={submitting}
              className="flex items-center gap-2 px-5 py-2 text-sm font-medium rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors shadow-sm disabled:opacity-50"
            >
              {submitting && <Loader2 className="w-4 h-4 animate-spin" />}
              Registrar Captação
            </button>
          )}
        </div>
      </form>
      </Form>
    </ExpandableSidePanel>
  );
}

function SectionTitle({ icon: Icon, title, subtitle }: { icon: typeof Briefcase; title: string; subtitle?: string }) {
  return (
    <div>
      <h3 className="text-sm font-bold text-foreground flex items-center gap-2">
        <Icon className="w-4 h-4 text-primary-600" /> {title}
      </h3>
      {subtitle && <p className="text-xs text-muted-foreground mt-0.5">{subtitle}</p>}
    </div>
  );
}

function Field({ label, error, children, hint }: { label: string; error?: string; children: React.ReactNode; hint?: string }) {
  return (
    <div>
      <label className="block text-xs font-bold text-muted-foreground uppercase tracking-wide mb-1">{label}</label>
      {children}
      {hint && !error && <p className="text-xs text-muted-foreground mt-1">{hint}</p>}
      <FieldError error={error} />
    </div>
  );
}

function FieldError({ error }: { error?: string }) {
  if (!error) return null;
  return <p className="text-xs text-status-critical mt-1">{error}</p>;
}

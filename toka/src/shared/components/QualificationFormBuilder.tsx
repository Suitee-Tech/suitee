import { useState, useCallback, useEffect } from "react";
import { motion, AnimatePresence } from "motion/react";
import { leadService, QualField, type QualRoutingConfig, type RouletteConfig } from "../services/lead.service";
import { Button } from "../ui/button";
import {
  ListPlus,
  Plus,
  Trash2,
  GripVertical,
  Settings,
  Eye,
  EyeOff,
  Copy,
  ArrowUp,
  ArrowDown,
  Type,
  Hash,
  DollarSign,
  CalendarDays,
  Phone,
  Mail,
  List,
  ToggleLeft,
  AlignLeft,
  X,
  AlertCircle,
  Save,
  RotateCcw,
  FileText,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";

type FieldType =
  | "text"
  | "textarea"
  | "select"
  | "multiselect"
  | "currency"
  | "number"
  | "date"
  | "phone"
  | "email"
  | "boolean";

const FIELD_TYPE_META: Record<FieldType, { label: string; icon: React.ComponentType<any>; color: string }> = {
  text: { label: "Texto Curto", icon: Type, color: "text-primary-600 bg-primary-50" },
  textarea: { label: "Texto Longo", icon: AlignLeft, color: "text-violet-600 bg-violet-50" },
  select: { label: "Lista (Seleção Unica)", icon: List, color: "text-status-ok bg-status-ok-soft" },
  multiselect: { label: "Lista (Multipla Escolha)", icon: List, color: "text-status-info bg-teal-50" },
  currency: { label: "Moeda (R$)", icon: DollarSign, color: "text-status-warning bg-status-warning-soft" },
  number: { label: "Número", icon: Hash, color: "text-status-info bg-sky-50" },
  date: { label: "Data", icon: CalendarDays, color: "text-status-critical bg-status-critical-soft" },
  phone: { label: "Telefone", icon: Phone, color: "text-status-ok bg-status-ok-soft" },
  email: { label: "E-mail", icon: Mail, color: "text-status-info bg-status-info-soft" },
  boolean: { label: "Sim / Não", icon: ToggleLeft, color: "text-orange-600 bg-orange-50" },
};

export const DEFAULT_QUAL_FIELDS: QualField[] = [
  {
    id: "qf-1",
    label: "Tipo de Negócio",
    type: "select",
    required: true,
    active: true,
    options: ["Compra", "Locação", "Investimento"],
    placeholder: "Selecione...",
    helpText: "Define o roteamento por especialidade do corretor",
    conditionalOn: null,
    conditionalValue: null,
  },
  {
    id: "qf-2",
    label: "Categoria de Imóvel",
    type: "select",
    required: true,
    active: true,
    options: ["Residencial", "Comercial", "Lançamento", "Terreno", "Rural"],
    placeholder: "Selecione...",
    helpText: "Filtra corretores pela categoria de atuacao",
    conditionalOn: null,
    conditionalValue: null,
  },
  {
    id: "qf-3",
    label: "Regiao de Interesse",
    type: "text",
    required: true,
    active: true,
    options: [],
    placeholder: "Ex: Zona Sul, Alphaville...",
    helpText: "Bairros ou regioes de interesse do lead",
    conditionalOn: null,
    conditionalValue: null,
  },
  {
    id: "qf-4",
    label: "Orçamento Estimado",
    type: "currency",
    required: false,
    active: true,
    options: [],
    placeholder: "R$ 0,00",
    helpText: "Valor maximo que o lead pretende investir",
    conditionalOn: "qf-1",
    conditionalValue: "Compra",
  },
  {
    id: "qf-5",
    label: "Urgência na Mudança",
    type: "select",
    required: false,
    active: true,
    options: ["Imediato", "Ate 3 meses", "6 meses", "Sem pressa"],
    placeholder: "Selecione...",
    helpText: "",
    conditionalOn: null,
    conditionalValue: null,
  },
];

let fieldCounter = 6;
const genId = () => `qf-${fieldCounter++}`;

export function QualificationFormBuilder(props: { onSaved?: (cfg: RouletteConfig) => void } = {}) {
  const [fields, setFields] = useState<QualField[]>(DEFAULT_QUAL_FIELDS);
  const [qualRouting, setQualRouting] = useState<QualRoutingConfig>({
    specialtyFieldIds: [],
    regionFieldId: null,
  });
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [previewMode, setPreviewMode] = useState(false);
  const [addingField, setAddingField] = useState(false);
  const [newFieldType, setNewFieldType] = useState<FieldType>("text");
  const [newFieldLabel, setNewFieldLabel] = useState("");
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    loadConfig();
  }, []);

  const deriveRoutingDefaults = useCallback((nextFields: QualField[]): QualRoutingConfig => {
    const active = nextFields.filter((f) => f.active);
    const selectable = active.filter((f) => f.type === "select" || f.type === "multiselect");
    const specialtyFieldIds = selectable
      .filter((f) => {
        const label = String(f.label || "").toLowerCase();
        const help = String(f.helpText || "").toLowerCase();
        return help.includes("especialidade") || label.includes("tipo") || label.includes("categoria");
      })
      .map((f) => f.id);
    const chosenSpecialty = specialtyFieldIds.length > 0 ? specialtyFieldIds : selectable.slice(0, 1).map((f) => f.id);
    const regionField =
      active.find((f) => String(f.label || "").toLowerCase().includes("regiao")) ??
      active.find((f) => String(f.label || "").toLowerCase().includes("bairro")) ??
      active.find((f) => String(f.label || "").toLowerCase().includes("local"));
    return { specialtyFieldIds: chosenSpecialty, regionFieldId: regionField?.id ?? null };
  }, []);

  const loadConfig = async () => {
    try {
      const config = await leadService.getRouletteConfig();
      const nextFields =
        config && config.qualFields && config.qualFields.length > 0
          ? (config.qualFields as unknown as QualField[])
          : DEFAULT_QUAL_FIELDS;
      setFields(nextFields);
      const nextRouting = (config as RouletteConfig | undefined)?.qualRouting ?? deriveRoutingDefaults(nextFields);
      setQualRouting({
        specialtyFieldIds: Array.isArray(nextRouting?.specialtyFieldIds) ? nextRouting.specialtyFieldIds : [],
        regionFieldId: (nextRouting?.regionFieldId as any) ?? null,
      });
    } catch (error) {
      console.error("Error loading qual form config:", error);
      toast.error("Erro ao carregar formulário");
    }
  };

  const markDirty = () => setDirty(true);

  const addField = () => {
    if (!newFieldLabel.trim()) {
      toast.error("Informe o nome do campo");
      return;
    }
    const newField: QualField = {
      id: genId(),
      label: newFieldLabel.trim(),
      type: newFieldType,
      required: false,
      active: true,
      options: newFieldType === "select" || newFieldType === "multiselect" ? ["Opção 1"] : [],
      placeholder: "",
      helpText: "",
      conditionalOn: null,
      conditionalValue: null,
    };
    setFields((prev) => [...prev, newField]);
    setNewFieldLabel("");
    setNewFieldType("text");
    setAddingField(false);
    setExpandedId(newField.id);
    markDirty();
    toast.success(`Campo "${newField.label}" adicionado`);
  };

  const duplicateField = (id: string) => {
    const original = fields.find((f) => f.id === id);
    if (!original) return;
    const dup: QualField = { ...original, id: genId(), label: `${original.label} (Copia)` };
    const idx = fields.findIndex((f) => f.id === id);
    const next = [...fields];
    next.splice(idx + 1, 0, dup);
    setFields(next);
    markDirty();
    toast.success(`Campo duplicado`);
  };

  const removeField = (id: string) => {
    const removed = fields.find((f) => f.id === id);
    setFields((prev) => prev.filter((f) => f.id !== id));
    setFields((prev) =>
      prev.map((f) => (f.conditionalOn === id ? { ...f, conditionalOn: null, conditionalValue: null } : f)),
    );
    markDirty();
    if (removed) toast.success(`Campo "${removed.label}" removido`);
  };

  const updateField = useCallback((id: string, patch: Partial<QualField>) => {
    setFields((prev) => prev.map((f) => (f.id === id ? { ...f, ...patch } : f)));
    markDirty();
  }, []);

  const moveField = (id: string, dir: -1 | 1) => {
    setFields((prev) => {
      const idx = prev.findIndex((f) => f.id === id);
      if (idx < 0) return prev;
      const target = idx + dir;
      if (target < 0 || target >= prev.length) return prev;
      const next = [...prev];
      [next[idx], next[target]] = [next[target], next[idx]];
      return next;
    });
    markDirty();
  };

  const handleSave = async () => {
    const requiredEmpty = fields.filter((f) => f.active && f.required && !f.label.trim());
    if (requiredEmpty.length > 0) {
      toast.error("Campos obrigatorios precisam de um nome");
      return;
    }

    try {
      const saved = await leadService.updateRouletteConfig({
        qualFields: fields as any[],
        qualRouting: qualRouting as any,
      });
      setDirty(false);
      toast.success("Formulário de qualificação salvo com sucesso", {
        description: `${fields.filter((f) => f.active).length} campos ativos, ${fields.filter((f) => f.required).length} obrigatorios`,
      });
      props.onSaved?.(saved);
    } catch (error) {
      console.error("Error saving qual form config:", error);
      toast.error("Erro ao salvar formulário");
    }
  };

  const handleReset = () => {
    setFields(DEFAULT_QUAL_FIELDS);
    setDirty(true);
    setExpandedId(null);
    toast.info("Formulário restaurado para o padrão (não salvo)");
  };

  const addOption = (fieldId: string) => {
    setFields((prev) =>
      prev.map((f) => (f.id === fieldId ? { ...f, options: [...f.options, `Opção ${f.options.length + 1}`] } : f)),
    );
    markDirty();
  };

  const updateOption = (fieldId: string, idx: number, value: string) => {
    setFields((prev) =>
      prev.map((f) => {
        if (f.id !== fieldId) return f;
        const opts = [...f.options];
        opts[idx] = value;
        return { ...f, options: opts };
      }),
    );
    markDirty();
  };

  const removeOption = (fieldId: string, idx: number) => {
    setFields((prev) =>
      prev.map((f) => {
        if (f.id !== fieldId) return f;
        const opts = f.options.filter((_, i) => i !== idx);
        return { ...f, options: opts };
      }),
    );
    markDirty();
  };

  const selectFields = fields.filter((f) => (f.type === "select" || f.type === "multiselect") && f.active);
  const regionCandidates = fields.filter(
    (f) => f.active && (f.type === "text" || f.type === "select" || f.type === "multiselect"),
  );

  return (
    <div className="space-y-6">
      <div className="bg-card rounded-2xl border border-border shadow-sm p-6">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 mb-2">
          <div>
            <h3 className="text-lg font-bold text-foreground flex items-center gap-2">
              <ListPlus className="w-5 h-5 text-primary-600" /> Formulário de Qualificação
            </h3>
            <p className="text-sm text-muted-foreground mt-1">
              Defina campos obrigatorios que devem ser preenchidos antes do lead entrar na roleta. Arraste para reordenar,
              configure opções de lista e regras condicionais.
            </p>
          </div>
          <div className="flex items-center gap-2 flex-shrink-0">
            <Button
              variant="outline"
              onClick={() => setPreviewMode(!previewMode)}
              className={clsx(
                "font-bold",
                previewMode && "bg-primary/10 border-primary/30 text-primary hover:bg-primary/15 hover:text-primary",
              )}
            >
              {previewMode ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              {previewMode ? "Editar" : "Preview"}
            </Button>
          </div>
        </div>

        <div className="flex items-center gap-4 mt-4 pt-4 border-t border-border">
          <span className="text-xs text-muted-foreground">
            <span className="font-bold text-foreground">{fields.filter((f) => f.active).length}</span> campos ativos
          </span>
          <span className="text-foreground/90">|</span>
          <span className="text-xs text-muted-foreground">
            <span className="font-bold text-status-critical">{fields.filter((f) => f.required && f.active).length}</span>{" "}
            obrigatorios
          </span>
          <span className="text-foreground/90">|</span>
          <span className="text-xs text-muted-foreground">
            <span className="font-bold text-status-warning">{fields.filter((f) => f.conditionalOn && f.active).length}</span>{" "}
            condicionais
          </span>
          {dirty && (
            <span className="ml-auto flex items-center gap-1.5 text-xs font-bold text-status-warning bg-status-warning-soft px-2.5 py-1 rounded-full border border-status-warning/30">
              <AlertCircle className="w-3 h-3" /> Alterações não salvas
            </span>
          )}
        </div>

        <div className="mt-5 pt-5 border-t border-border grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="rounded-xl border border-border bg-muted/20 p-4">
            <div className="text-xs font-bold text-muted-foreground">Integração com capacidade do corretor</div>
            <div className="mt-3 space-y-2">
              {selectFields.length === 0 ? (
                <div className="text-xs text-muted-foreground">Ative um campo de lista para mapear especialidades.</div>
              ) : (
                selectFields.map((f) => {
                  const checked = qualRouting.specialtyFieldIds.includes(f.id);
                  return (
                    <label key={f.id} className="flex items-center gap-2 text-sm text-foreground cursor-pointer">
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() => {
                          setQualRouting((prev) => {
                            const has = prev.specialtyFieldIds.includes(f.id);
                            const next = has
                              ? prev.specialtyFieldIds.filter((x) => x !== f.id)
                              : [...prev.specialtyFieldIds, f.id];
                            return { ...prev, specialtyFieldIds: next };
                          });
                          markDirty();
                        }}
                      />
                      <span className="font-bold">{f.label}</span>
                    </label>
                  );
                })
              )}
            </div>
          </div>

          <div className="rounded-xl border border-border bg-muted/20 p-4">
            <div className="text-xs font-bold text-muted-foreground">Integração com regiões</div>
            <div className="mt-3">
              <select
                value={qualRouting.regionFieldId || ""}
                onChange={(e) => {
                  const v = e.target.value || null;
                  setQualRouting((prev) => ({ ...prev, regionFieldId: v }));
                  markDirty();
                }}
                className="w-full px-3 py-2 rounded-xl border border-border text-sm bg-card outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
              >
                <option value="">Nenhum</option>
                {regionCandidates.map((f) => (
                  <option key={f.id} value={f.id}>
                    {f.label}
                  </option>
                ))}
              </select>
              <div className="mt-2 text-xs text-muted-foreground">
                O valor deste campo é usado como localização do lead para o filtro por região na roleta.
              </div>
            </div>
          </div>
        </div>
      </div>

      <AnimatePresence mode="wait">
        {previewMode ? (
          <motion.div
            key="preview"
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -10 }}
            className="bg-card rounded-2xl border border-border shadow-sm p-6"
          >
            <div className="flex items-center gap-2 mb-6">
              <FileText className="w-5 h-5 text-primary-600" />
              <h3 className="text-base font-bold text-foreground">Preview do Formulário</h3>
              <span className="text-xs bg-primary-50 text-primary-600 font-bold px-2 py-0.5 rounded-full border border-primary-200">
                Como o recepcionista vera
              </span>
            </div>
            <div className="space-y-5 max-w-lg">
              {fields
                .filter((f) => f.active)
                .map((field) => {
                  const condLabel = field.conditionalOn ? fields.find((f) => f.id === field.conditionalOn)?.label : null;
                  return (
                    <div key={field.id}>
                      {condLabel && (
                        <p className="text-micro font-bold text-status-warning mb-1 flex items-center gap-1">
                          <AlertCircle className="w-3 h-3" /> Visivel apenas quando "{condLabel}" = "{field.conditionalValue}"
                        </p>
                      )}
                      <label className="block text-sm font-bold text-foreground mb-1.5">
                        {field.label}
                        {field.required && <span className="text-status-critical ml-1">*</span>}
                      </label>
                      {field.helpText && <p className="text-xs text-muted-foreground mb-2">{field.helpText}</p>}
                      {field.type === "select" || field.type === "multiselect" ? (
                        <select className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none">
                          <option value="">{field.placeholder || "Selecione..."}</option>
                          {field.options.map((opt, i) => (
                            <option key={i}>{opt}</option>
                          ))}
                        </select>
                      ) : field.type === "textarea" ? (
                        <textarea
                          rows={3}
                          placeholder={field.placeholder}
                          className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none resize-none"
                        />
                      ) : field.type === "boolean" ? (
                        <div className="flex items-center gap-4">
                          <label className="flex items-center gap-2 cursor-pointer">
                            <input type="radio" name={field.id} className="text-primary-600" />
                            <span className="text-sm text-foreground">Sim</span>
                          </label>
                          <label className="flex items-center gap-2 cursor-pointer">
                            <input type="radio" name={field.id} className="text-primary-600" />
                            <span className="text-sm text-foreground">Não</span>
                          </label>
                        </div>
                      ) : (
                        <input
                          type={
                            field.type === "currency" || field.type === "number"
                              ? "number"
                              : field.type === "date"
                                ? "date"
                                : field.type === "phone"
                                  ? "tel"
                                  : field.type === "email"
                                    ? "email"
                                    : "text"
                          }
                          placeholder={field.placeholder}
                          className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                        />
                      )}
                    </div>
                  );
                })}
            </div>
          </motion.div>
        ) : (
          <motion.div
            key="editor"
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -10 }}
            className="space-y-3"
          >
            {fields.map((field, index) => {
              const fieldType = (field.type in FIELD_TYPE_META ? field.type : "text") as FieldType;
              const meta = FIELD_TYPE_META[fieldType];
              const Icon = meta.icon;
              const isExpanded = expandedId === field.id;
              const condLabel = field.conditionalOn ? fields.find((f) => f.id === field.conditionalOn)?.label : null;

              return (
                <motion.div
                  key={field.id}
                  layout
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: field.active ? 1 : 0.55, y: 0 }}
                  className={clsx(
                    "bg-card rounded-2xl border shadow-sm overflow-hidden transition-colors",
                    isExpanded ? "border-primary-300 ring-2 ring-primary-600/10" : "border-border",
                  )}
                >
                  <div className="flex items-center gap-3 px-4 py-3.5">
                    <div className="flex flex-col items-center gap-0.5 flex-shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => moveField(field.id, -1)}
                        disabled={index === 0}
                        className="size-6 text-muted-foreground/60 hover:text-muted-foreground"
                      >
                        <ArrowUp className="w-3 h-3" />
                      </Button>
                      <GripVertical className="w-4 h-4 text-muted-foreground/60" />
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => moveField(field.id, 1)}
                        disabled={index === fields.length - 1}
                        className="size-6 text-muted-foreground/60 hover:text-muted-foreground"
                      >
                        <ArrowDown className="w-3 h-3" />
                      </Button>
                    </div>

                    <div className={clsx("w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0", meta.color)}>
                      <Icon className="w-4 h-4" />
                    </div>

                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-bold text-foreground truncate">{field.label}</span>
                        {field.required && (
                          <span className="px-1.5 py-0.5 text-caption font-bold text-status-critical bg-status-critical-soft rounded border border-status-critical/30 uppercase">
                            Obrigatório
                          </span>
                        )}
                        {condLabel && (
                          <span className="px-1.5 py-0.5 text-caption font-bold text-status-warning bg-status-warning-soft rounded border border-status-warning/30">
                            Condicional
                          </span>
                        )}
                      </div>
                      <span className="text-caption text-muted-foreground font-medium">{meta.label}</span>
                    </div>

                    <div className="flex items-center gap-1 flex-shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => updateField(field.id, { active: !field.active })}
                        className={clsx(
                          "size-8",
                          field.active
                            ? "text-status-ok hover:bg-status-ok-soft hover:text-status-ok"
                            : "text-muted-foreground",
                        )}
                        title={field.active ? "Desativar campo" : "Ativar campo"}
                      >
                        {field.active ? <Eye className="w-4 h-4" /> : <EyeOff className="w-4 h-4" />}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => duplicateField(field.id)}
                        className="size-8 text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
                        title="Duplicar campo"
                      >
                        <Copy className="w-4 h-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => removeField(field.id)}
                        className="size-8 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
                        title="Remover campo"
                      >
                        <Trash2 className="w-4 h-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => setExpandedId(isExpanded ? null : field.id)}
                        className={clsx("size-8", isExpanded ? "bg-primary/10 text-primary" : "text-muted-foreground")}
                      >
                        <Settings className="w-4 h-4" />
                      </Button>
                    </div>
                  </div>

                  <AnimatePresence>
                    {isExpanded && (
                      <motion.div
                        initial={{ height: 0, opacity: 0 }}
                        animate={{ height: "auto", opacity: 1 }}
                        exit={{ height: 0, opacity: 0 }}
                        transition={{ duration: 0.2 }}
                        className="overflow-hidden"
                      >
                        <div className="px-5 pb-5 pt-2 border-t border-border bg-muted/50">
                          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-5">
                            <div>
                              <label className="block text-xs font-bold text-foreground mb-1.5">Nome do Campo</label>
                              <input
                                type="text"
                                value={field.label}
                                onChange={(e) => updateField(field.id, { label: e.target.value })}
                                className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                              />
                            </div>

                            <div>
                              <label className="block text-xs font-bold text-foreground mb-1.5">Tipo do Campo</label>
                              <select
                                value={field.type}
                                onChange={(e) => {
                                  const newType = e.target.value as FieldType;
                                  const patch: Partial<QualField> = { type: newType };
                                  if ((newType === "select" || newType === "multiselect") && field.options.length === 0) {
                                    patch.options = ["Opção 1"];
                                  }
                                  updateField(field.id, patch);
                                }}
                                className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                              >
                                {Object.entries(FIELD_TYPE_META).map(([key, val]) => (
                                  <option key={key} value={key}>
                                    {val.label}
                                  </option>
                                ))}
                              </select>
                            </div>

                            <div>
                              <label className="block text-xs font-bold text-foreground mb-1.5">Placeholder</label>
                              <input
                                type="text"
                                value={field.placeholder}
                                onChange={(e) => updateField(field.id, { placeholder: e.target.value })}
                                placeholder="Texto de exemplo..."
                                className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                              />
                            </div>

                            <div>
                              <label className="block text-xs font-bold text-foreground mb-1.5">Texto de Ajuda</label>
                              <input
                                type="text"
                                value={field.helpText}
                                onChange={(e) => updateField(field.id, { helpText: e.target.value })}
                                placeholder="Dica para o preenchimento..."
                                className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                              />
                            </div>
                          </div>

                          <div className="flex items-center gap-6 mb-5">
                            <label className="flex items-center gap-2.5 cursor-pointer">
                              <input
                                type="checkbox"
                                checked={field.required}
                                onChange={(e) => updateField(field.id, { required: e.target.checked })}
                                className="rounded border-border text-primary-600 focus:ring-primary-600"
                              />
                              <span className="text-sm font-bold text-foreground">Campo obrigatório</span>
                            </label>
                            <label className="flex items-center gap-2.5 cursor-pointer">
                              <input
                                type="checkbox"
                                checked={field.active}
                                onChange={(e) => updateField(field.id, { active: e.target.checked })}
                                className="rounded border-border text-primary-600 focus:ring-primary-600"
                              />
                              <span className="text-sm font-bold text-foreground">Campo ativo</span>
                            </label>
                          </div>

                          {(field.type === "select" || field.type === "multiselect") && (
                            <div className="mb-5">
                              <label className="block text-xs font-bold text-foreground mb-2">
                                Opções da Lista ({field.options.length})
                              </label>
                              <div className="space-y-2">
                                {field.options.map((opt, oi) => (
                                  <div key={oi} className="flex items-center gap-2">
                                    <span className="text-xs font-bold text-muted-foreground w-5 text-right">{oi + 1}.</span>
                                    <input
                                      type="text"
                                      value={opt}
                                      onChange={(e) => updateOption(field.id, oi, e.target.value)}
                                      className="flex-1 px-3 py-1.5 bg-card border border-border rounded-lg text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                                    />
                                    <Button
                                      variant="ghost"
                                      size="icon"
                                      onClick={() => removeOption(field.id, oi)}
                                      disabled={field.options.length <= 1}
                                      className="size-7 text-muted-foreground hover:text-status-critical"
                                    >
                                      <X className="w-3.5 h-3.5" />
                                    </Button>
                                  </div>
                                ))}
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => addOption(field.id)}
                                  className="mt-1 text-xs font-bold text-primary hover:text-primary"
                                >
                                  <Plus className="w-3.5 h-3.5" /> Adicionar opção
                                </Button>
                              </div>
                            </div>
                          )}

                          <div className="p-4 bg-status-warning-soft/50 border border-status-warning/60 rounded-xl">
                            <label className="flex items-center gap-2.5 cursor-pointer mb-3">
                              <input
                                type="checkbox"
                                checked={!!field.conditionalOn}
                                onChange={(e) => {
                                  if (!e.target.checked) {
                                    updateField(field.id, { conditionalOn: null, conditionalValue: null });
                                  } else {
                                    const first = selectFields.find((sf) => sf.id !== field.id);
                                    updateField(field.id, {
                                      conditionalOn: first?.id || null,
                                      conditionalValue: first?.options[0] || null,
                                    });
                                  }
                                }}
                                className="rounded border-status-warning/40 text-status-warning focus:ring-amber-600"
                              />
                              <span className="text-sm font-bold text-status-warning">Exibicao condicional</span>
                              <span className="text-micro text-status-warning bg-status-warning-soft px-1.5 py-0.5 rounded">
                                Mostrar campo apenas quando...
                              </span>
                            </label>

                            {field.conditionalOn && (
                              <div className="grid grid-cols-2 gap-3 mt-2">
                                <div>
                                  <label className="block text-micro font-bold text-status-warning mb-1">Campo Pai</label>
                                  <select
                                    value={field.conditionalOn}
                                    onChange={(e) => {
                                      const parent = selectFields.find((sf) => sf.id === e.target.value);
                                      updateField(field.id, {
                                        conditionalOn: e.target.value,
                                        conditionalValue: parent?.options[0] || null,
                                      });
                                    }}
                                    className="w-full px-3 py-1.5 bg-card border border-status-warning/30 rounded-lg text-xs focus:ring-2 focus:ring-amber-600/20 focus:border-status-warning outline-none"
                                  >
                                    {selectFields
                                      .filter((sf) => sf.id !== field.id)
                                      .map((sf) => (
                                        <option key={sf.id} value={sf.id}>
                                          {sf.label}
                                        </option>
                                      ))}
                                  </select>
                                </div>
                                <div>
                                  <label className="block text-micro font-bold text-status-warning mb-1">Valor Igual a</label>
                                  <select
                                    value={field.conditionalValue || ""}
                                    onChange={(e) => updateField(field.id, { conditionalValue: e.target.value })}
                                    className="w-full px-3 py-1.5 bg-card border border-status-warning/30 rounded-lg text-xs focus:ring-2 focus:ring-amber-600/20 focus:border-status-warning outline-none"
                                  >
                                    {(selectFields.find((sf) => sf.id === field.conditionalOn)?.options || []).map((opt, i) => (
                                      <option key={i} value={opt}>
                                        {opt}
                                      </option>
                                    ))}
                                  </select>
                                </div>
                              </div>
                            )}
                          </div>
                        </div>
                      </motion.div>
                    )}
                  </AnimatePresence>
                </motion.div>
              );
            })}

            <AnimatePresence>
              {addingField ? (
                <motion.div
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -8 }}
                  className="bg-card rounded-2xl border-2 border-primary-200 shadow-sm p-5"
                >
                  <h4 className="text-sm font-bold text-foreground mb-4 flex items-center gap-2">
                    <Plus className="w-4 h-4 text-primary-600" /> Novo Campo
                  </h4>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
                    <div>
                      <label className="block text-xs font-bold text-foreground mb-1.5">Nome do Campo</label>
                      <input
                        type="text"
                        value={newFieldLabel}
                        onChange={(e) => setNewFieldLabel(e.target.value)}
                        placeholder="Ex: Tem animal de estimacao?"
                        className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                        autoFocus
                        onKeyDown={(e) => e.key === "Enter" && addField()}
                      />
                    </div>
                    <div>
                      <label className="block text-xs font-bold text-foreground mb-1.5">Tipo</label>
                      <select
                        value={newFieldType}
                        onChange={(e) => setNewFieldType(e.target.value as FieldType)}
                        className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none"
                      >
                        {Object.entries(FIELD_TYPE_META).map(([key, val]) => (
                          <option key={key} value={key}>
                            {val.label}
                          </option>
                        ))}
                      </select>
                    </div>
                  </div>

                  <div className="flex flex-wrap gap-2 mb-4">
                    {(Object.entries(FIELD_TYPE_META) as [FieldType, (typeof FIELD_TYPE_META)[FieldType]][]).map(([key, val]) => {
                      const TypeIcon = val.icon;
                      return (
                        <button
                          key={key}
                          onClick={() => setNewFieldType(key)}
                          className={clsx(
                            "flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-caption font-bold border transition-colors",
                            newFieldType === key
                              ? "border-primary-300 bg-primary-50 text-primary-700"
                              : "border-border bg-card text-muted-foreground hover:border-border",
                          )}
                        >
                          <TypeIcon className="w-3 h-3" /> {val.label}
                        </button>
                      );
                    })}
                  </div>

                  <div className="flex items-center justify-end gap-2">
                    <Button
                      variant="outline"
                      onClick={() => {
                        setAddingField(false);
                        setNewFieldLabel("");
                      }}
                    >
                      Cancelar
                    </Button>
                    <Button onClick={addField}>
                      <Plus className="w-4 h-4" /> Adicionar
                    </Button>
                  </div>
                </motion.div>
              ) : (
                <motion.button
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  onClick={() => setAddingField(true)}
                  className="w-full py-4 border-2 border-dashed border-border rounded-2xl text-sm font-bold text-muted-foreground hover:text-primary-600 hover:border-primary-300 hover:bg-primary-50/50 transition-all flex items-center justify-center gap-2"
                >
                  <Plus className="w-4 h-4" /> Adicionar Campo Personalizado
                </motion.button>
              )}
            </AnimatePresence>

            <div className="flex items-center justify-between">
              <Button variant="outline" onClick={handleReset} className="font-bold text-muted-foreground">
                <RotateCcw className="w-4 h-4" /> Restaurar Padrão
              </Button>
              <Button onClick={handleSave} disabled={!dirty}>
                <Save className="w-4 h-4" /> Salvar Formulário
              </Button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

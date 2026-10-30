import React, { useId, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import {
 X,
 Plus,
 DollarSign,
 Receipt,
 FileText,
 AlertCircle,
 CheckCircle2,
 Banknote,
 ChevronDown,
 Building2,
 Droplet,
 Zap,
 Flame,
 ShieldCheck,
 ArrowUpRight,
 Percent,
} from "lucide-react";
import { clsx } from "clsx";
import { motion, AnimatePresence } from "motion/react";
import { usePropertiesContext } from "../context/PropertiesContext";
import { Contract } from "../contracts/types";
import { ExpandableSidePanel } from "./ExpandableSidePanel";
import { formatMoneyBR, parseMoneyBR } from "../utils/format";
import { MoneyInput } from "../ui/money-input";
import { buildContractIssues, showContractIssueToasts, type ContractIssue } from "../utils/contractIssues";
import { useCreateChargeMutation } from "../hooks/useContracts";

// ContractIssuesBanner renders the pre-condition gaps for a contract with
// per-issue navigation CTAs. Shared by both modals in this file so the
// Nova Cobrança and Novo Repasse flows speak with one voice when refs
// are missing.
function ContractIssuesBanner({ issues }: { issues: ContractIssue[] }) {
 if (issues.length === 0) return null;
 return (
 <div className="p-4 rounded-xl border border-status-critical/30 bg-status-critical-soft text-sm text-status-critical">
 <div className="flex items-start gap-2 mb-2">
 <AlertCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />
 <p className="font-bold">Contrato incompleto — resolva antes de emitir:</p>
 </div>
 <ul className="space-y-2 ml-6">
 {issues.map((issue) => (
 <li key={issue.ref} className="flex items-center justify-between gap-3">
 <span>{issue.message}</span>
 <button
 type="button"
 onClick={issue.onCtaClick}
 className="inline-flex items-center gap-1 font-bold underline hover:no-underline whitespace-nowrap"
 >
 {issue.ctaLabel}
 <ArrowUpRight className="w-3 h-3" />
 </button>
 </li>
 ))}
 </ul>
 </div>
 );
}

// --- Types ---
interface ChargeFormData {
 description: string;
 category: string;
 value: string;
 dueDay: string;
 dueMonth: string;
 dueYear: string;
 recurrence: string;
 installments: string;
 notes: string;
}

const CHARGE_CATEGORIES = [
 { value: "Aluguel", label: "Aluguel", icon: <Building2 className="w-4 h-4" /> },
 { value: "Condominio", label: "Condominio", icon: <Building2 className="w-4 h-4" /> },
 { value: "IPTU", label: "IPTU", icon: <Receipt className="w-4 h-4" /> },
 { value: "Agua", label: "Agua", icon: <Droplet className="w-4 h-4" /> },
 { value: "Energia", label: "Energia", icon: <Zap className="w-4 h-4" /> },
 { value: "Gas", label: "Gas", icon: <Flame className="w-4 h-4" /> },
 { value: "Seguro", label: "Seguro Incendio", icon: <ShieldCheck className="w-4 h-4" /> },
 { value: "Multa", label: "Multa / Encargo", icon: <AlertCircle className="w-4 h-4" /> },
 { value: "Outros", label: "Outros", icon: <FileText className="w-4 h-4" /> },
];

const MONTHS = [
 "Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho",
 "Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro",
];

const RECURRENCE_OPTIONS = [
 { value: "unica", label: "Cobrança Única" },
 { value: "mensal", label: "Mensal (Recorrente)" },
 { value: "parcelada", label: "Parcelada" },
];

// ========================
// Nova Cobrança Modal
// ========================
interface NovaCobrancaModalProps {
 contract: Contract;
 onClose: () => void;
}

export function NovaCobrancaModal({ contract, onClose }: NovaCobrancaModalProps) {
 const formId = useId();
 const { properties } = usePropertiesContext();
 const navigate = useNavigate();
 const createChargeMutation = useCreateChargeMutation();
 const [step, setStep] = useState<1 | 2>(1);
 const [submitted, setSubmitted] = useState(false);

 // Resolve the property's owner so the charge has a valid owner_id; we
 // intentionally do not fall back to bogus "owner_123" strings — if we can't
 // resolve the owner the backend will reject the charge and surface a clear
 // error to the user.
 const resolvedProperty = contract.property_id
   ? properties.find((p) => p.id === contract.property_id)
   : undefined;
 const resolvedOwnerId = resolvedProperty?.ownerId
   ?? resolvedProperty?.owners?.[0]?.id
   ?? "";

 const issues = useMemo(
 () => buildContractIssues(contract, resolvedProperty, navigate),
 [contract, resolvedProperty, navigate],
 );
 const [form, setForm] = useState<ChargeFormData>({
 description: "",
 category: "Aluguel",
 value: String(contract.value),
 dueDay: "10",
 dueMonth: MONTHS[new Date().getMonth()],
 dueYear: "2026",
 recurrence: "unica",
 installments: "1",
 notes: "",
 });
 const [errors, setErrors] = useState<Partial<Record<keyof ChargeFormData, string>>>({});

 const formatCurrency = (val: number) => formatMoneyBR(val, { withSymbol: true, fractionDigits: 2 });
 const parseCurrencyValue = (val: string): number => parseMoneyBR(val) ?? 0;

 const updateField = (field: keyof ChargeFormData, value: string) => {
 setForm((prev) => ({ ...prev, [field]: value }));
 if (errors[field]) setErrors((prev) => ({ ...prev, [field]: undefined }));
 };

 const validateStep1 = (): boolean => {
 const newErrors: Partial<Record<keyof ChargeFormData, string>> = {};
 if (!form.description.trim()) newErrors.description = "Descrição obrigatória";
 if (!form.category) newErrors.category = "Selecione uma categoria";
 const val = parseCurrencyValue(form.value);
 if (val <= 0) newErrors.value = "Valor deve ser maior que zero";
 setErrors(newErrors);
 return Object.keys(newErrors).length === 0;
 };

 const validateStep2 = (): boolean => {
 const newErrors: Partial<Record<keyof ChargeFormData, string>> = {};
 const day = parseInt(form.dueDay);
 if (isNaN(day) || day < 1 || day > 31) newErrors.dueDay = "Dia inválido";
 if (form.recurrence === "parcelada") {
 const inst = parseInt(form.installments);
 if (isNaN(inst) || inst < 2 || inst > 60)
 newErrors.installments = "Parcelas entre 2 e 60";
 }
 setErrors(newErrors);
 return Object.keys(newErrors).length === 0;
 };

 const mapCategoryToType = (category: string) => {
 switch (category) {
 case "Aluguel": return "RENT";
 case "Condominio": return "CONDOMINIO";
 case "IPTU": return "IPTU";
 case "Agua":
 case "Energia":
 case "Gas": return "UTILITY";
 default: return "OTHER";
 }
 };

 const handleSubmit = async () => {
 if (!validateStep2()) return;

 if (issues.length > 0) {
 showContractIssueToasts(issues);
 return;
 }

 const value = parseCurrencyValue(form.value);
 const monthIndex = MONTHS.indexOf(form.dueMonth);
 const dueDay = form.dueDay.padStart(2, "0");
 const count = form.recurrence === "parcelada" ? parseInt(form.installments) : 1;

 for (let i = 0; i < count; i++) {
 const currentMonthIndex = (monthIndex + i) % 12;
 const yearOffset = Math.floor((monthIndex + i) / 12);
 const currentYear = parseInt(form.dueYear) + yearOffset;
 const currentMonthNum = String(currentMonthIndex + 1).padStart(2, "0");
 const currentMonthLabel = MONTHS[currentMonthIndex];
 const installmentSuffix = count > 1 ? ` (${i + 1}/${count})` : "";

 const isoDate = `${currentYear}-${currentMonthNum}-${dueDay}T00:00:00Z`;

 await createChargeMutation.mutateAsync({
 contract_id: contract.id,
 // buildContractIssues above guarantees customer_id, property_id, and the
 // resolved owner are present — the ! satisfies TS without runtime cost.
 tenant_party_id: contract.customer_id!,
 owner_id: resolvedOwnerId,
 property_id: contract.property_id!,
 type: mapCategoryToType(form.category),
 description: `${form.description || 'Cobrança'} - ${currentMonthLabel}/${currentYear}${installmentSuffix}`,
 reference_month: `${currentYear}-${currentMonthNum}`,
 amount: form.recurrence === "parcelada" ? Math.round((value / count) * 100) / 100 : value,
 due_date: isoDate,
 discount: 0,
 });
 }

 setSubmitted(true);
 setTimeout(() => onClose(), 1800);
 };

 const previewValue = parseCurrencyValue(form.value);
 const installmentCount =
 form.recurrence === "parcelada" ? parseInt(form.installments) || 1 : 1;
 const perInstallment =
 installmentCount > 1 ? Math.round((previewValue / installmentCount) * 100) / 100 : previewValue;

 // --- Success Screen ---
 if (submitted) {
 return (
 <AnimatePresence>
 <div className="fixed inset-0 z-50 flex justify-end">
 <motion.div
 initial={{ opacity: 0 }}
 animate={{ opacity: 1 }}
 exit={{ opacity: 0 }}
 className="absolute inset-0 bg-foreground/20 backdrop-blur-sm"
 />
 <motion.div
 initial={{ x: "100%", opacity: 0.5 }}
 animate={{ x: 0, opacity: 1 }}
 exit={{ x: "100%", opacity: 0.5 }}
 transition={{ type: "spring", bounce: 0, duration: 0.4 }}
 className="relative w-full max-w-xl bg-card h-full shadow-2xl flex flex-col items-center justify-center"
 >
 <motion.div
 initial={{ scale: 0 }}
 animate={{ scale: 1 }}
 transition={{ type: "spring", bounce: 0.4, delay: 0.2 }}
 className="w-20 h-20 rounded-full bg-status-ok-soft flex items-center justify-center mb-6"
 >
 <CheckCircle2 className="w-10 h-10 text-status-ok" />
 </motion.div>
 <motion.h2
 initial={{ opacity: 0, y: 10 }}
 animate={{ opacity: 1, y: 0 }}
 transition={{ delay: 0.35 }}
 className="text-xl font-bold text-foreground mb-2"
 >
 {installmentCount > 1
 ? `${installmentCount} cobranças criadas`
 : "Cobrança criada"}
 </motion.h2>
 <motion.p
 initial={{ opacity: 0, y: 10 }}
 animate={{ opacity: 1, y: 0 }}
 transition={{ delay: 0.45 }}
 className="text-sm text-muted-foreground text-center max-w-xs"
 >
 {form.description} - {formatCurrency(previewValue)}
 {installmentCount > 1
 ? ` em ${installmentCount}x de ${formatCurrency(perInstallment)}`
 : ""}
 </motion.p>
 </motion.div>
 </div>
 </AnimatePresence>
 );
 }

 return (
 <ExpandableSidePanel
 open={true}
 onClose={onClose}
 defaultSize="xl"
 expandedSize="full"
 bgClass="bg-muted"
 renderHeader={({ expandButton, closeButton }) => (
 <div className="bg-card px-6 py-4 border-b border-border flex items-center justify-between z-10 flex-shrink-0">
 <div className="flex items-center gap-3">
 <div className="p-2 bg-primary-50 text-primary-700 rounded-lg">
 <Plus className="w-5 h-5" />
 </div>
 <div>
 <h2 className="text-lg font-bold text-foreground">Nova Cobrança</h2>
 <p className="text-sm text-muted-foreground">
 {contract.id} - {contract.client}
 </p>
 </div>
 </div>
 <div className="flex items-center gap-2">
 {expandButton}
 {closeButton}
 </div>
 </div>
 )}
 >
 {/* Steps Indicator */}
 <div className="px-6 py-3 bg-card border-b border-border">
 <div className="flex items-center gap-2">
 <div className="flex items-center gap-2">
 <div
 className={clsx(
 "w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold transition-colors",
 step >= 1
 ? "bg-primary text-primary-foreground"
 : "bg-muted text-muted-foreground"
 )}
 >
 1
 </div>
 <span
 className={clsx(
 "text-xs font-medium",
 step >= 1 ? "text-foreground" : "text-muted-foreground"
 )}
 >
 Dados da Cobrança
 </span>
 </div>
 <div className="flex-1 h-px bg-muted mx-2" />
 <div className="flex items-center gap-2">
 <div
 className={clsx(
 "w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold transition-colors",
 step >= 2
 ? "bg-primary text-primary-foreground"
 : "bg-muted text-muted-foreground"
 )}
 >
 2
 </div>
 <span
 className={clsx(
 "text-xs font-medium",
 step >= 2 ? "text-foreground" : "text-muted-foreground"
 )}
 >
 Vencimento e Recorrencia
 </span>
 </div>
 </div>
 </div>

 {/* Form Content */}
 <div className="flex-1 overflow-y-auto p-6">
 {step === 1 && (
 <motion.div
 initial={{ opacity: 0, x: 20 }}
 animate={{ opacity: 1, x: 0 }}
 transition={{ duration: 0.2 }}
 className="space-y-6"
 >
 <ContractIssuesBanner issues={issues} />
 {/* Contract Info Card */}
 <div className="bg-card p-4 rounded-xl border border-border shadow-sm">
 <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
 <div>
 <p className="text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">
 Contrato
 </p>
 <p className="text-sm font-bold text-foreground">{contract.id}</p>
 </div>
 <div>
 <p className="text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">
 Inquilino
 </p>
 <p className="text-sm font-bold text-foreground">{contract.client}</p>
 </div>
 <div>
 <p className="text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">
 Imóvel
 </p>
 <p className="text-sm font-bold text-foreground truncate">
 {contract.property_title}
 </p>
 </div>
 <div>
 <p className="text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">
 Aluguel Vigente
 </p>
 <p className="text-sm font-bold text-primary-600">
 {formatCurrency(contract.value || 0)}
 </p>
 </div>
 </div>
 </div>

 {/* Category Selection */}
 <div>
 <label id={`${formId}-category-label`} className="block text-sm font-bold text-foreground mb-3">
 Categoria da Cobrança <span aria-hidden="true" className="text-status-critical">*</span>
 </label>
 <div role="group" aria-labelledby={`${formId}-category-label`} aria-describedby={errors.category ? `${formId}-category-error` : undefined} className="grid grid-cols-1 sm:grid-cols-3 gap-2">
 {CHARGE_CATEGORIES.map((cat) => (
 <button
 key={cat.value}
 type="button"
 aria-pressed={form.category === cat.value}
 onClick={() => {
 updateField("category", cat.value);
 if (!form.description && cat.value !== "Outros") {
 updateField("description", cat.label);
 }
 if (cat.value === "Aluguel") {
 updateField("value", String(contract.value));
 }
 }}
 className={clsx(
 "flex items-center gap-2 px-3 py-2.5 rounded-xl border text-sm font-medium transition-all",
 form.category === cat.value
 ? "bg-primary-50 border-primary-300 text-primary-700 ring-2 ring-primary-600/20"
 : "bg-card border-border text-muted-foreground hover:border-border hover:bg-muted"
 )}
 >
 <span
 className={clsx(
 form.category === cat.value
 ? "text-primary-600"
 : "text-muted-foreground"
 )}
 >
 {cat.icon}
 </span>
 {cat.label}
 </button>
 ))}
 </div>
 {errors.category && (
 <p id={`${formId}-category-error`} role="alert" className="text-xs text-status-critical mt-1 flex items-center gap-1">
 <AlertCircle className="w-3 h-3" /> {errors.category}
 </p>
 )}
 </div>

 {/* Description */}
 <div>
 <label htmlFor={`${formId}-description`} className="block text-sm font-bold text-foreground mb-1.5">
 Descrição <span aria-hidden="true" className="text-status-critical">*</span>
 </label>
 <input
 id={`${formId}-description`}
 type="text"
 aria-required="true"
 aria-invalid={errors.description ? true : undefined}
 aria-describedby={errors.description ? `${formId}-description-error` : undefined}
 value={form.description}
 onChange={(e) => updateField("description", e.target.value)}
 placeholder="Ex: Aluguel, Condominio, IPTU..."
 className={clsx(
 "w-full px-4 py-2.5 bg-card border rounded-xl text-sm outline-none transition-all",
 errors.description
 ? "border-status-critical/40 focus:ring-2 focus:ring-red-600/20 focus:border-status-critical"
 : "border-border focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
 )}
 />
 {errors.description && (
 <p id={`${formId}-description-error`} role="alert" className="text-xs text-status-critical mt-1 flex items-center gap-1">
 <AlertCircle className="w-3 h-3" /> {errors.description}
 </p>
 )}
 </div>

 {/* Value */}
 <div>
 <label htmlFor={`${formId}-value`} className="block text-sm font-bold text-foreground mb-1.5">
 Valor (R$) <span aria-hidden="true" className="text-status-critical">*</span>
 </label>
 <div className="relative">
 <DollarSign className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
 <MoneyInput
 id={`${formId}-value`}
 aria-required="true"
 aria-invalid={errors.value ? true : undefined}
 aria-describedby={errors.value ? `${formId}-value-error` : undefined}
 value={form.value === "" ? undefined : parseCurrencyValue(form.value)}
 onChange={(v) => updateField("value", v === undefined ? "" : String(v))}
 placeholder="0,00"
 className={clsx(
 "w-full pl-10 pr-4 py-2.5 bg-card border rounded-xl text-sm outline-none transition-all",
 errors.value
 ? "border-status-critical/40 focus:ring-2 focus:ring-red-600/20 focus:border-status-critical"
 : "border-border focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
 )}
 />
 </div>
 {errors.value && (
 <p id={`${formId}-value-error`} role="alert" className="text-xs text-status-critical mt-1 flex items-center gap-1">
 <AlertCircle className="w-3 h-3" /> {errors.value}
 </p>
 )}
 {form.category === "Aluguel" && (
 <p className="text-micro text-muted-foreground mt-1">
 Valor do aluguel vigente: {formatCurrency(contract.value || 0)}
 </p>
 )}
 </div>

 {/* Notes */}
 <div>
 <label htmlFor={`${formId}-notes`} className="block text-sm font-bold text-foreground mb-1.5">
 Observações{" "}
 <span className="text-muted-foreground font-normal">(opcional)</span>
 </label>
 <textarea
 id={`${formId}-notes`}
 value={form.notes}
 onChange={(e) => updateField("notes", e.target.value)}
 rows={3}
 placeholder="Informações adicionais sobre esta cobrança..."
 className="w-full px-4 py-3 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none resize-none transition-all"
 />
 </div>
 </motion.div>
 )}

 {step === 2 && (
 <motion.div
 initial={{ opacity: 0, x: 20 }}
 animate={{ opacity: 1, x: 0 }}
 transition={{ duration: 0.2 }}
 className="space-y-6"
 >
 {/* Due Date */}
 <div>
 <p className="block text-sm font-bold text-foreground mb-3">
 Data de Vencimento <span aria-hidden="true" className="text-status-critical">*</span>
 </p>
 <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
 <div>
 <label htmlFor={`${formId}-due-day`} className="block text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">
 Dia
 </label>
 <input
 id={`${formId}-due-day`}
 type="number"
 min="1"
 max="31"
 aria-required="true"
 aria-invalid={errors.dueDay ? true : undefined}
 aria-describedby={errors.dueDay ? `${formId}-due-day-error` : undefined}
 value={form.dueDay}
 onChange={(e) => updateField("dueDay", e.target.value)}
 className={clsx(
 "w-full px-3 py-2.5 bg-card border rounded-xl text-sm text-center outline-none transition-all",
 errors.dueDay
 ? "border-status-critical/40 focus:ring-2 focus:ring-red-600/20"
 : "border-border focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
 )}
 />
 {errors.dueDay && (
 <p id={`${formId}-due-day-error`} role="alert" className="text-micro text-status-critical mt-0.5">{errors.dueDay}</p>
 )}
 </div>
 <div>
 <label htmlFor={`${formId}-due-month`} className="block text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">
 Mês
 </label>
 <div className="relative">
 <select
 id={`${formId}-due-month`}
 aria-required="true"
 value={form.dueMonth}
 onChange={(e) => updateField("dueMonth", e.target.value)}
 className="w-full px-3 py-2.5 bg-card border border-border rounded-xl text-sm outline-none appearance-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all"
 >
 {MONTHS.map((m) => (
 <option key={m} value={m}>
 {m}
 </option>
 ))}
 </select>
 <ChevronDown className="w-4 h-4 text-muted-foreground absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
 </div>
 </div>
 <div>
 <label htmlFor={`${formId}-due-year`} className="block text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">
 Ano
 </label>
 <div className="relative">
 <select
 id={`${formId}-due-year`}
 aria-required="true"
 value={form.dueYear}
 onChange={(e) => updateField("dueYear", e.target.value)}
 className="w-full px-3 py-2.5 bg-card border border-border rounded-xl text-sm outline-none appearance-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all"
 >
 <option value="2025">2025</option>
 <option value="2026">2026</option>
 <option value="2027">2027</option>
 <option value="2028">2028</option>
 </select>
 <ChevronDown className="w-4 h-4 text-muted-foreground absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
 </div>
 </div>
 </div>
 </div>

 {/* Recurrence */}
 <div>
 <p id={`${formId}-recurrence-label`} className="block text-sm font-bold text-foreground mb-3">
 Tipo de Recorrência
 </p>
 <div role="group" aria-labelledby={`${formId}-recurrence-label`} className="space-y-2">
 {RECURRENCE_OPTIONS.map((opt) => (
 <button
 key={opt.value}
 type="button"
 aria-pressed={form.recurrence === opt.value}
 onClick={() => updateField("recurrence", opt.value)}
 className={clsx(
 "w-full flex items-center gap-3 px-4 py-3 rounded-xl border text-sm font-medium transition-all text-left",
 form.recurrence === opt.value
 ? "bg-primary-50 border-primary-300 text-primary-700 ring-2 ring-primary-600/20"
 : "bg-card border-border text-muted-foreground hover:border-border"
 )}
 >
 <div
 className={clsx(
 "w-4 h-4 rounded-full border-2 flex items-center justify-center transition-all",
 form.recurrence === opt.value
 ? "border-primary-600"
 : "border-border"
 )}
 >
 {form.recurrence === opt.value && (
 <div className="w-2 h-2 rounded-full bg-primary-600" />
 )}
 </div>
 {opt.label}
 </button>
 ))}
 </div>
 </div>

 {/* Installments (only for parcelada) */}
 {form.recurrence === "parcelada" && (
 <motion.div
 initial={{ opacity: 0, height: 0 }}
 animate={{ opacity: 1, height: "auto" }}
 exit={{ opacity: 0, height: 0 }}
 >
 <label htmlFor={`${formId}-installments`} className="block text-sm font-bold text-foreground mb-1.5">
 Número de Parcelas <span aria-hidden="true" className="text-status-critical">*</span>
 </label>
 <input
 id={`${formId}-installments`}
 type="number"
 min="2"
 max="60"
 aria-required="true"
 aria-invalid={errors.installments ? true : undefined}
 aria-describedby={errors.installments ? `${formId}-installments-error` : undefined}
 value={form.installments}
 onChange={(e) => updateField("installments", e.target.value)}
 className={clsx(
 "w-full px-4 py-2.5 bg-card border rounded-xl text-sm outline-none transition-all",
 errors.installments
 ? "border-status-critical/40 focus:ring-2 focus:ring-red-600/20"
 : "border-border focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600"
 )}
 />
 {errors.installments && (
 <p id={`${formId}-installments-error`} role="alert" className="text-xs text-status-critical mt-1 flex items-center gap-1">
 <AlertCircle className="w-3 h-3" /> {errors.installments}
 </p>
 )}
 </motion.div>
 )}

 {/* Preview Card */}
 <div className="bg-card p-5 rounded-xl border border-border shadow-sm">
 <h4 className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-3">
 Resumo da Cobrança
 </h4>
 <div className="space-y-2.5">
 <div className="flex justify-between items-center">
 <span className="text-sm text-muted-foreground">Descrição</span>
 <span className="text-sm font-bold text-foreground">
 {form.description || "-"}
 </span>
 </div>
 <div className="flex justify-between items-center">
 <span className="text-sm text-muted-foreground">Categoria</span>
 <span className="text-xs font-medium text-foreground bg-muted px-2 py-0.5 rounded">
 {form.category}
 </span>
 </div>
 <div className="flex justify-between items-center">
 <span className="text-sm text-muted-foreground">Valor Total</span>
 <span className="text-sm font-bold text-foreground">
 {formatCurrency(previewValue)}
 </span>
 </div>
 {form.recurrence === "parcelada" && installmentCount > 1 && (
 <div className="flex justify-between items-center">
 <span className="text-sm text-muted-foreground">Valor por Parcela</span>
 <span className="text-sm font-bold text-primary-600">
 {installmentCount}x de {formatCurrency(perInstallment)}
 </span>
 </div>
 )}
 <div className="flex justify-between items-center">
 <span className="text-sm text-muted-foreground">1º Vencimento</span>
 <span className="text-sm font-bold text-foreground">
 {form.dueDay.padStart(2, "0")}/{form.dueMonth}/{form.dueYear}
 </span>
 </div>
 <div className="flex justify-between items-center">
 <span className="text-sm text-muted-foreground">
 {form.recurrence === "parcelada"
 ? "Cobranças a criar"
 : "Tipo"}
 </span>
 <span className="text-sm font-bold text-foreground">
 {form.recurrence === "parcelada"
 ? `${installmentCount} parcelas`
 : form.recurrence === "mensal"
 ? "Recorrente"
 : "Unica"}
 </span>
 </div>
 <div className="border-t border-border pt-2 flex justify-between items-center">
 <span className="text-sm font-bold text-foreground">Contrato</span>
 <span className="text-sm font-bold text-foreground">{contract.id}</span>
 </div>
 </div>
 </div>
 </motion.div>
 )}
 </div>

 {/* Footer */}
 <div className="bg-card px-6 py-4 border-t border-border flex items-center gap-3">
 {step === 2 && (
 <button
 onClick={() => setStep(1)}
 className="px-5 py-2.5 bg-card border border-border text-foreground text-sm font-bold rounded-xl hover:bg-muted transition-colors"
 >
 Voltar
 </button>
 )}
 <button
 onClick={onClose}
 className="px-5 py-2.5 bg-card border border-border text-foreground text-sm font-bold rounded-xl hover:bg-muted transition-colors ml-auto"
 >
 Cancelar
 </button>
 {step === 1 ? (
 <button
 onClick={() => {
 if (validateStep1()) setStep(2);
 }}
 disabled={issues.length > 0}
 className="px-6 py-2.5 bg-primary text-primary-foreground text-sm font-bold rounded-xl hover:bg-primary/90 transition-colors shadow-sm flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
 >
 Próximo
 <ArrowUpRight className="w-4 h-4" />
 </button>
 ) : (
 <button
 onClick={handleSubmit}
 disabled={issues.length > 0}
 className="px-6 py-2.5 bg-primary text-primary-foreground text-sm font-bold rounded-xl hover:bg-primary/90 transition-colors shadow-sm flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
 >
 <Plus className="w-4 h-4" />
 Criar Cobrança
 </button>
 )}
 </div>
 </ExpandableSidePanel>
 );
}

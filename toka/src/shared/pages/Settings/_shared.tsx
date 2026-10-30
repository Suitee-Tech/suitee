import React, { createContext, useContext, useId } from "react";

type FieldContextValue = { label?: string; descriptionId?: string; required?: boolean };
const FieldLabelContext = createContext<FieldContextValue>({});
import { motion } from "motion/react";
import { AlertTriangle } from "lucide-react";
import { clsx } from "clsx";

import { DocumentInput, type DocumentMode } from "../../ui/document-input";
import { PhoneInput } from "../../ui/phone-input";
import { CepInput } from "../../ui/cep-input";

export interface TabRef {
  save: () => Promise<void>;
}

export function SectionCard({
  title,
  subtitle,
  children,
  delay = 0,
  actions,
}: {
  title: string;
  subtitle?: string;
  children: React.ReactNode;
  delay?: number;
  actions?: React.ReactNode;
}) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3, delay }}
      className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden"
    >
      <div className="px-6 py-5 border-b border-border flex items-center justify-between gap-3">
        <div>
          <h3 className="text-base font-bold text-foreground">{title}</h3>
          {subtitle && <p className="text-xs text-muted-foreground mt-0.5">{subtitle}</p>}
        </div>
        {actions}
      </div>
      <div className="p-6">{children}</div>
    </motion.div>
  );
}

export function FieldRow({
  label,
  description,
  required,
  children,
}: {
  label: string;
  description?: string;
  required?: boolean;
  children: React.ReactNode;
}) {
  const descriptionId = useId();
  return (
    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 py-3 first:pt-0 last:pb-0">
      <div className="min-w-0 sm:max-w-[60%]">
        <p className="text-sm font-bold text-foreground">
          {label}
          {required ? <span aria-hidden="true" className="text-status-critical"> *</span> : null}
        </p>
        {description && <p id={descriptionId} className="text-xs text-muted-foreground mt-0.5">{description}</p>}
      </div>
      <FieldLabelContext.Provider value={{ label, descriptionId: description ? descriptionId : undefined, required }}>
        <div className="shrink-0 w-full sm:w-auto">{children}</div>
      </FieldLabelContext.Provider>
    </div>
  );
}

export function SettingsInput({
  value,
  onChange,
  placeholder,
  type = "text",
  className = "",
  mono = false,
  disabled = false,
  icon,
  error,
  ariaLabel,
  id,
  required,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  type?: string;
  className?: string;
  mono?: boolean;
  disabled?: boolean;
  icon?: React.ReactNode;
  error?: string;
  ariaLabel?: string;
  id?: string;
  required?: boolean;
}) {
  const field = useContext(FieldLabelContext);
  const errorId = useId();
  const describedBy = [field.descriptionId, error ? errorId : undefined].filter(Boolean).join(" ") || undefined;
  const requiredAttr = required ?? field.required;
  const a11y = {
    "aria-label": ariaLabel ?? field.label,
    "aria-invalid": error ? true : undefined,
    "aria-describedby": describedBy,
    "aria-required": requiredAttr || undefined,
  };
  const inputClassName = clsx(
    "w-full px-3 py-2.5 bg-card border rounded-xl text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 transition-colors disabled:bg-muted disabled:text-muted-foreground",
    error
      ? "border-status-critical/60 focus:ring-status-critical/20 focus:border-status-critical bg-status-critical-soft/30"
      : "border-border focus:ring-primary-500/20 focus:border-primary-400",
    icon && "pl-9",
    mono && "font-mono text-xs",
    className,
  );
  // Variantes mascaradas (type cnpj/cpf/document/phone/cep): value e onChange
  // trafegam em dígitos crus; o componente cuida da máscara de exibição.
  const docMode: DocumentMode | null =
    type === "cnpj" ? "cnpj" : type === "cpf" ? "cpf" : type === "document" ? "auto" : null;
  return (
    <div>
      <div className="relative">
        {icon && (
          <div
            className={clsx(
              "absolute left-3 top-1/2 -translate-y-1/2",
              error ? "text-status-critical" : "text-muted-foreground",
            )}
          >
            {icon}
          </div>
        )}
        {type === "phone" ? (
          <PhoneInput id={id} {...a11y} value={value} onChange={onChange} placeholder={placeholder} disabled={disabled} className={inputClassName} />
        ) : type === "cep" ? (
          <CepInput id={id} {...a11y} value={value} onChange={onChange} placeholder={placeholder} disabled={disabled} className={inputClassName} />
        ) : docMode ? (
          <DocumentInput id={id} {...a11y} mode={docMode} value={value} onChange={onChange} placeholder={placeholder} disabled={disabled} className={inputClassName} />
        ) : (
          <input
            id={id}
            type={type}
            value={value}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
            placeholder={placeholder}
            disabled={disabled}
            {...a11y}
            className={inputClassName}
          />
        )}
      </div>
      {error && (
        <p id={errorId} role="alert" className="text-micro text-status-critical font-medium mt-1 flex items-center gap-1">
          <AlertTriangle className="w-3 h-3" />
          {error}
        </p>
      )}
    </div>
  );
}

export function Toggle({
  checked,
  onChange,
  size = "md",
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  size?: "sm" | "md";
}) {
  const field = useContext(FieldLabelContext);
  const w = size === "sm" ? "w-9" : "w-11";
  const h = size === "sm" ? "h-5" : "h-6";
  const dot = size === "sm" ? "w-3.5 h-3.5" : "w-4.5 h-4.5";
  const translate = size === "sm" ? "translate-x-4" : "translate-x-5";

  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={field.label}
      aria-describedby={field.descriptionId}
      onClick={() => onChange(!checked)}
      className={clsx(
        "relative inline-flex shrink-0 rounded-full transition-colors duration-200 ease-in-out cursor-pointer",
        w,
        h,
        checked ? "bg-primary-600" : "bg-muted",
      )}
    >
      <span
        className={clsx(
          "inline-block rounded-full bg-card shadow-sm transform transition-transform duration-200 ease-in-out mt-[3px] ml-[3px]",
          dot,
          checked ? translate : "translate-x-0",
        )}
      />
    </button>
  );
}

export function SettingsSelect({
  value,
  onChange,
  options,
  id,
  required,
  error,
  ariaLabel,
}: {
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
  id?: string;
  required?: boolean;
  error?: string;
  ariaLabel?: string;
}) {
  const field = useContext(FieldLabelContext);
  const errorId = useId();
  const describedBy = [field.descriptionId, error ? errorId : undefined].filter(Boolean).join(" ") || undefined;
  return (
    <>
    <select
      id={id}
      aria-label={ariaLabel ?? field.label}
      aria-describedby={describedBy}
      aria-invalid={error ? true : undefined}
      aria-required={(required ?? field.required) || undefined}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="w-full sm:w-auto px-3 py-2.5 bg-card border border-border rounded-xl text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-400 transition-colors cursor-pointer appearance-none pr-8 bg-[url('data:image/svg+xml;charset=UTF-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20width%3D%2212%22%20height%3D%2212%22%20viewBox%3D%220%200%2024%2024%22%20fill%3D%22none%22%20stroke%3D%22%2394a3b8%22%20stroke-width%3D%222.5%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%3E%3Cpolyline%20points%3D%226%209%2012%2015%2018%209%22%3E%3C%2Fpolyline%3E%3C%2Fsvg%3E')] bg-no-repeat bg-[right_0.75rem_center]"
    >
      {options.map((opt) => (
        <option key={opt.value} value={opt.value}>
          {opt.label}
        </option>
      ))}
    </select>
    {error && (
      <p id={errorId} role="alert" className="text-micro text-status-critical font-medium mt-1">{error}</p>
    )}
    </>
  );
}

export function Badge({
  text,
  variant = "default",
}: {
  text: string;
  variant?: "default" | "success" | "warning" | "danger" | "info";
}) {
  const colors = {
    default: "bg-muted text-muted-foreground border-border",
    success: "bg-status-ok-soft text-status-ok border-status-ok/30",
    warning: "bg-status-warning-soft text-status-warning border-status-warning/30",
    danger: "bg-status-critical-soft text-status-critical border-status-critical/30",
    info: "bg-primary-50 text-primary-700 border-primary-200",
  };
  return (
    <span className={clsx("inline-flex items-center px-2.5 py-1 text-[11px] font-bold rounded-lg border", colors[variant])}>
      {text}
    </span>
  );
}

import React, { useEffect, useId, useRef, useState } from "react";
import { AlertTriangle, ShieldAlert } from "lucide-react";
import { clsx } from "clsx";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogTitle,
} from "../ui/alert-dialog";
import { Button } from "../ui/button";

export interface ConfirmDialogInput {
  label: string;
  placeholder?: string;
  required?: boolean;
  minLength?: number;
}

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  description: React.ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: "danger" | "warning" | "default";
  doubleConfirm?: boolean;
  doubleConfirmLabel?: string;
  onConfirm: (value?: string) => void | Promise<void>;
  onCancel: () => void;
  /** Campo opcional de entrada (ex.: referência da conciliação), validado inline. */
  input?: ConfirmDialogInput;
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = "Confirmar",
  cancelLabel = "Cancelar",
  variant = "default",
  doubleConfirm = false,
  doubleConfirmLabel,
  onConfirm,
  onCancel,
  input,
}: ConfirmDialogProps) {
  const inputId = useId();
  const pendingRef = useRef(false);
  const opener = useRef<HTMLElement | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [step, setStep] = useState(0);
  const [isPending, setIsPending] = useState(false);
  const [inputValue, setInputValue] = useState("");
  const [inputTouched, setInputTouched] = useState(false);

  const inputTrimmed = inputValue.trim();
  const inputMin = input?.minLength ?? 3;
  const inputInvalid = input
    ? (input.required !== false && !inputTrimmed) || (inputTrimmed.length > 0 && inputTrimmed.length < inputMin)
    : false;

  const handleConfirm = async () => {
    if (pendingRef.current) return;
    if (input && inputInvalid) {
      setInputTouched(true);
      return;
    }
    if (doubleConfirm && step === 0) {
      setStep(1);
      return;
    }
    setActionError(null);
    pendingRef.current = true;
    setIsPending(true);
    try {
      await onConfirm(input ? inputTrimmed : undefined);
      setStep(0);
    } catch {
      setActionError("Não foi possível concluir a ação. Verifique os dados e tente novamente.");
    } finally {
      pendingRef.current = false;
      setIsPending(false);
    }
  };

  const handleCancel = () => {
    if (pendingRef.current) return;
    setStep(0);
    onCancel();
  };

  useEffect(() => {
    if (!open) {
      setStep(0);
      setIsPending(false);
      setInputValue("");
      setInputTouched(false);
      setActionError(null);
    }
  }, [open]);

  const iconBg =
    variant === "danger"
      ? "bg-status-critical-soft"
      : variant === "warning"
        ? "bg-status-warning-soft"
        : "bg-primary/10";

  const iconColor =
    variant === "danger"
      ? "text-status-critical"
      : variant === "warning"
        ? "text-status-warning"
        : "text-primary";

  const confirmVariant =
    variant === "danger" ? "destructive" : variant === "warning" ? "warning" : "default";

  const Icon = variant === "danger" ? ShieldAlert : AlertTriangle;

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !isPending) handleCancel();
      }}
    >
      <AlertDialogContent
        className="gap-4"
        onOpenAutoFocus={() => { opener.current = document.activeElement as HTMLElement | null; }}
        onCloseAutoFocus={(event) => {
          if (opener.current?.isConnected) {
            event.preventDefault();
            opener.current.focus();
          }
        }}
      >
        <div className="flex items-start gap-4">
          <div className={clsx("p-3 rounded-xl shrink-0", iconBg)}>
            <Icon className={clsx("w-6 h-6", iconColor)} />
          </div>
          <div className="flex-1 min-w-0">
            <AlertDialogTitle className="text-lg font-bold text-foreground">{title}</AlertDialogTitle>
            <AlertDialogDescription className="mt-1 text-foreground/80">{description}</AlertDialogDescription>
          </div>
        </div>

        {input && (
          <div className="space-y-1.5">
            <label htmlFor={inputId} className="block text-sm font-bold text-foreground">{input.label}</label>
            <input
              id={inputId}
              aria-invalid={inputInvalid && inputTouched}
              aria-describedby={inputInvalid && inputTouched ? `${inputId}-error` : undefined}
              type="text"
              value={inputValue}
              onChange={(e) => { setInputValue(e.target.value); setInputTouched(false); }}
              onBlur={() => setInputTouched(true)}
              placeholder={input.placeholder}
              disabled={isPending}
              className={`w-full rounded-xl border bg-muted px-3 py-2 text-sm outline-none transition-all focus:bg-card ${
                inputInvalid && inputTouched
                  ? "border-status-critical focus:ring-2 focus:ring-status-critical/20"
                  : "border-border focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20"
              }`}
            />
            {inputInvalid && inputTouched && (
              <p id={`${inputId}-error`} role="alert" className="text-xs text-status-critical">
                {input.required !== false && !inputTrimmed ? "Campo obrigatório." : `Informe no mínimo ${inputMin} caracteres.`}
              </p>
            )}
          </div>
        )}

        {doubleConfirm && step === 1 && (
          <div className="p-4 bg-status-critical-soft border border-status-critical/30 rounded-xl">
            <div className="flex items-start gap-3">
              <ShieldAlert className="w-5 h-5 text-status-critical shrink-0 mt-0.5" />
              <div>
                <p className="text-sm font-bold text-status-critical">
                  {doubleConfirmLabel || "Tem certeza absoluta?"}
                </p>
                <p className="text-xs text-status-critical mt-1">
                  Esta ação pode ter impacto significativo nos registros financeiros. Clique novamente para confirmar.
                </p>
              </div>
            </div>
          </div>
        )}

        {actionError && <p role="alert" className="text-sm text-status-critical">{actionError}</p>}

        <AlertDialogFooter>
          <Button variant="outline" onClick={handleCancel} disabled={isPending}>
            {cancelLabel}
          </Button>
          <Button
            variant={step === 1 ? "destructive" : confirmVariant}
            onClick={handleConfirm}
            loading={isPending}
            loadingText="Processando..."
            disabled={isPending || (!!input && inputInvalid)}
          >
            {step === 1 ? (
              <>
                <ShieldAlert className="w-4 h-4" /> Confirmar Definitivamente
              </>
            ) : (
              confirmLabel
            )}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

import * as React from "react";

import {
  formatCnpj,
  formatCpf,
  formatCpfCnpj,
  parseCnpj,
  parseCpf,
  parseCpfCnpj,
} from "../utils/masks";

export type DocumentMode = "cpf" | "cnpj" | "auto";

export interface DocumentInputProps
  extends Omit<React.ComponentProps<"input">, "value" | "onChange" | "type" | "inputMode"> {
  /** Dígitos crus do documento (sem máscara). `""`/`undefined` = vazio. */
  value: string | undefined;
  /** Recebe os dígitos crus (sem pontuação). A validação (estrita) fica no zod. */
  onChange: (value: string) => void;
  /**
   * `cpf` força máscara/limite de CPF (11 díg.); `cnpj` força CNPJ (14 díg.);
   * `auto` (default) usa CPF até 11 dígitos e CNPJ a partir daí.
   */
  mode?: DocumentMode;
}

const formatters: Record<DocumentMode, (v: string) => string> = {
  cpf: formatCpf,
  cnpj: formatCnpj,
  auto: formatCpfCnpj,
};

const parsers: Record<DocumentMode, (v: string) => string> = {
  cpf: parseCpf,
  cnpj: parseCnpj,
  auto: parseCpfCnpj,
};

/**
 * Input de documento (CPF/CNPJ) com máscara. Controlado/stateless no padrão
 * do MoneyInput: display de `format*(value)`, dígitos canônicos via `parse*`.
 * Não valida — quem valida (estritamente) é o schema zod via `isValidCpfCnpj`.
 */
export const DocumentInput = React.forwardRef<HTMLInputElement, DocumentInputProps>(
  ({ value, onChange, mode = "auto", ...props }, ref) => {
    const display = formatters[mode](value ?? "");

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange(parsers[mode](e.target.value));
    };

    return (
      <input
        ref={ref}
        type="text"
        inputMode="numeric"
        value={display}
        onChange={handleChange}
        {...props}
      />
    );
  },
);

DocumentInput.displayName = "DocumentInput";

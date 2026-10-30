import * as React from "react";

import { formatCep, parseCep } from "../utils/masks";

export interface CepInputProps
  extends Omit<React.ComponentProps<"input">, "value" | "onChange" | "type" | "inputMode"> {
  /** Dígitos crus do CEP (até 8), ex.: "01310100". `""`/`undefined` = vazio. */
  value: string | undefined;
  /** Recebe os dígitos crus (sem máscara). Formate com `formatCep` no submit se a API espera "00000-000". */
  onChange: (value: string) => void;
}

/**
 * Input de CEP com máscara 00000-000. Controlado/stateless no padrão do
 * MoneyInput: display de `formatCep(value)`, dígitos canônicos via `parseCep`.
 */
export const CepInput = React.forwardRef<HTMLInputElement, CepInputProps>(
  ({ value, onChange, ...props }, ref) => {
    const display = formatCep(value ?? "");

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange(parseCep(e.target.value));
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

CepInput.displayName = "CepInput";

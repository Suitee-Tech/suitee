import * as React from "react";

import { formatPhoneBR, parsePhoneBR } from "../utils/masks";

export interface PhoneInputProps
  extends Omit<React.ComponentProps<"input">, "value" | "onChange" | "type" | "inputMode"> {
  /** Dígitos canônicos do telefone (DDD + número), ex.: "11999998888". `""`/`undefined` = vazio. */
  value: string | undefined;
  /** Recebe os dígitos crus (sem máscara, sem "+55"). O "+55" é responsabilidade da camada de serviço. */
  onChange: (value: string) => void;
}

/**
 * Input de telefone BR com máscara (DD) NNNNN-NNNN / (DD) NNNN-NNNN.
 *
 * Controlado e stateless, no mesmo padrão do MoneyInput: renderiza a partir
 * de `value` via `formatPhoneBR`, e cada tecla é reduzida aos dígitos
 * canônicos via `parsePhoneBR` (que também tolera "+55"). Sem estado interno
 * nem `useEffect` — o cursor cai no fim e não "briga" com o usuário.
 */
export const PhoneInput = React.forwardRef<HTMLInputElement, PhoneInputProps>(
  ({ value, onChange, ...props }, ref) => {
    const display = formatPhoneBR(value ?? "");

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange(parsePhoneBR(e.target.value));
    };

    return (
      <input
        ref={ref}
        type="tel"
        inputMode="tel"
        value={display}
        onChange={handleChange}
        {...props}
      />
    );
  },
);

PhoneInput.displayName = "PhoneInput";

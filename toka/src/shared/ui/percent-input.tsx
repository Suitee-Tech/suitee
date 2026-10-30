import * as React from "react";

import { formatPercentBR, parsePercentBR } from "../utils/masks";

export interface PercentInputProps
  extends Omit<React.ComponentProps<"input">, "value" | "onChange" | "type" | "inputMode"> {
  /** Percentual como número humano (50 = 50%). `undefined` = vazio (mostra placeholder). */
  value: number | undefined;
  /** Recebe o número, ou `undefined` quando o campo é esvaziado. */
  onChange: (value: number | undefined) => void;
  /** Máximo de casas decimais ao reformatar (default 2). */
  fractionDigits?: number;
}

// Mantém só dígitos e UM separador decimal (vírgula), sem inserir agrupador
// de milhar — assim o cursor nunca "briga" durante a digitação livre da casa
// decimal. Diferente do MoneyInput, o percentual permite decimal arbitrária,
// então guardamos o texto digitado num buffer e só reformatamos no blur/sync.
const sanitize = (raw: string): string => {
  let s = raw.replace(/[^\d.,]/g, "").replace(/\./g, ",");
  const first = s.indexOf(",");
  if (first !== -1) {
    s = s.slice(0, first + 1) + s.slice(first + 1).replace(/,/g, "");
  }
  return s;
};

export const PercentInput = React.forwardRef<HTMLInputElement, PercentInputProps>(
  ({ value, onChange, fractionDigits = 2, onBlur, ...props }, ref) => {
    const [text, setText] = React.useState(() => formatPercentBR(value, { fractionDigits }));

    // Sincroniza com mudanças EXTERNAS de `value` (reset/defaultValues). Não
    // reformata enquanto o usuário digita: se o texto atual já representa o
    // mesmo número, não tocamos no buffer (preserva "12," e zeros à direita).
    React.useEffect(() => {
      if (parsePercentBR(text) !== value) {
        setText(formatPercentBR(value, { fractionDigits }));
      }
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [value]);

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      const next = sanitize(e.target.value);
      setText(next);
      onChange(parsePercentBR(next));
    };

    const handleBlur = (e: React.FocusEvent<HTMLInputElement>) => {
      setText(formatPercentBR(value, { fractionDigits }));
      onBlur?.(e);
    };

    return (
      <input
        ref={ref}
        type="text"
        inputMode="decimal"
        value={text}
        onChange={handleChange}
        onBlur={handleBlur}
        {...props}
      />
    );
  },
);

PercentInput.displayName = "PercentInput";

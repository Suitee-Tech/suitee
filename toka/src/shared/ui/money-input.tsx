import * as React from "react";

import { formatMoneyBR } from "../utils/format";

export interface MoneyInputProps
  extends Omit<React.ComponentProps<"input">, "value" | "onChange" | "type" | "inputMode"> {
  /** Valor em reais (ex.: 1234.56). `undefined` = campo vazio (mostra o placeholder). */
  value: number | undefined;
  /** Recebe o valor em reais, ou `undefined` quando o campo é esvaziado. */
  onChange: (value: number | undefined) => void;
  /** 2 = centavos (default); 0 = reais inteiros (preço/valor de negócio). */
  fractionDigits?: 0 | 2;
  /** Prefixa "R$" dentro do campo. Default `false` (labels já costumam dizer "(R$)"). */
  withSymbol?: boolean;
}

const fmtValue = (value: number | undefined, withSymbol: boolean, fractionDigits: 0 | 2): string => {
  return value == null || !Number.isFinite(value) ? "" : formatMoneyBR(value, { withSymbol, fractionDigits });
};

/**
 * Input de valor monetário com máscara de centavos (preenchimento da direita p/ esquerda).
 *
 * Quase-stateless: enquanto o usuário digita, o display usa um rascunho local
 * (mesmo conteúdo do prop, sem distorção de cursor); o valor é devolvido ao form
 * a cada tecla. No blur, se o form "perdeu" o valor por algum reset/glitch de
 * interação (ex.: mudança de tipo no formulário), o valor digitado é
 * **devolvido automaticamente** — o campo nunca fica limpo sem intenção.
 *
 * Digitar 1·5·0·0·0·0 → "1.500,00". Backspace remove o último dígito. Vazio → `undefined`.
 */
export const MoneyInput = React.forwardRef<HTMLInputElement, MoneyInputProps>(
  ({ value, onChange, fractionDigits = 2, withSymbol = false, onBlur, ...props }, ref) => {
    const [draft, setDraft] = React.useState<string | null>(null);
    const lastTypedRef = React.useRef<number | undefined>(undefined);

    const display = draft ?? fmtValue(value, withSymbol, fractionDigits);

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      // Extrai só os dígitos: descarta "R$", pontos de milhar, vírgula e o NBSP do Intl.
      const digits = e.target.value.replace(/\D/g, "");
      if (digits === "") {
        lastTypedRef.current = undefined;
        setDraft("");
        onChange(undefined);
        return;
      }
      // Acumulador inteiro (cap defensivo p/ não estourar Number.MAX_SAFE_INTEGER).
      const intVal = Number.parseInt(digits.slice(0, 15), 10);
      const val = fractionDigits === 2 ? intVal / 100 : intVal;
      lastTypedRef.current = val;
      setDraft(fmtValue(val, withSymbol, fractionDigits));
      onChange(val);
    };

    const handleBlur = (e: React.FocusEvent<HTMLInputElement>) => {
      const typed = lastTypedRef.current;
      // O form divergiu do que está visível (reset/glitch) → devolve o valor
      // digitado; depois volta a exibir o prop.
      if (typed != null && (value == null || !Number.isFinite(value) || value !== typed)) {
        lastTypedRef.current = undefined;
        onChange(typed);
      } else {
        lastTypedRef.current = undefined;
      }
      setDraft(null);
      onBlur?.(e);
    };

    return (
      <input
        ref={ref}
        type="text"
        inputMode="numeric"
        value={display}
        onChange={handleChange}
        onBlur={handleBlur}
        {...props}
      />
    );
  },
);

MoneyInput.displayName = "MoneyInput";

import { useEffect, useState } from "react";
import { Pipette } from "lucide-react";
import { clsx } from "clsx";

import { normalizeHexColor, readableTextColor } from "../utils/hexColor";

/**
 * Quadrado de cor que É o seletor: por baixo do quadrado (que mantém a
 * aparência já usada no white-label) vive um `<input type="color">` esticado e
 * transparente, então clicar na cor abre o seletor nativo do navegador.
 *
 * Por que o input nativo, e não um seletor próprio: é o único que já traz roda
 * de cores, conta-gotas em alguns sistemas, entrada por código e leitor de
 * tela, sem somar dependência nem superfície nova ao design system.
 *
 * `value` pode estar em qualquer formato (o campo de texto ao lado aceita
 * digitação livre): o input recebe a última forma VÁLIDA vista, para não piscar
 * preto enquanto o operador digita "#3" ou cola um valor com espaços.
 */
export function ColorInput({
  value,
  onChange,
  label,
  className,
}: {
  value: string;
  onChange: (hex: string) => void;
  /** Nome acessível do controle — o quadrado não tem texto. */
  label: string;
  className?: string;
}) {
  const normalized = normalizeHexColor(value);
  const [lastValid, setLastValid] = useState<string | null>(normalized);

  useEffect(() => {
    if (normalized) setLastValid(normalized);
  }, [normalized]);

  const pickerValue = normalized ?? lastValid;

  const shell = clsx(
    "relative block h-14 w-14 shrink-0 overflow-hidden rounded-2xl border border-border shadow-inner transition-transform",
    className,
  );

  // Valor atual não é hexadecimal (texto no meio da digitação, dado legado):
  // o quadrado continua mostrando o que está escrito, mas sem seletor.
  if (!pickerValue) {
    return <div className={shell} style={{ backgroundColor: value }} aria-hidden="true" />;
  }

  return (
    <label
      className={clsx(
        shell,
        "group cursor-pointer hover:scale-105 focus-within:ring-2 focus-within:ring-primary-500/40",
      )}
      title={`${label} — abrir o seletor de cores`}
    >
      <span className="absolute inset-0" style={{ backgroundColor: value }} aria-hidden="true" />
      {/* Ícone indicativo: o quadrado sozinho não diz que é clicável. Fica no
          centro da área clicável, com um chip do tom oposto ao da cor para
          continuar legível em qualquer fundo (verde-claro ou azul-escuro). */}
      <span
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 grid place-items-center"
      >
        <span
          className="grid h-6 w-6 place-items-center rounded-full transition-transform group-hover:scale-110 group-focus-within:scale-110"
          style={{
            backgroundColor: readableTextColor(value) === "#FFFFFF" ? "rgba(0,0,0,0.35)" : "rgba(255,255,255,0.55)",
            color: readableTextColor(value),
          }}
        >
          <Pipette data-testid="color-input-icon" className="h-3.5 w-3.5" />
        </span>
      </span>
      <input
        type="color"
        aria-label={label}
        value={pickerValue}
        onChange={(event) => onChange(event.target.value)}
        className="absolute inset-0 h-full w-full cursor-pointer border-0 bg-transparent p-0 opacity-0"
      />
    </label>
  );
}

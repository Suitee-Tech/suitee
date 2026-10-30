import { useId, useState } from "react";
import {
  CONTRACT_DATE_BASE_KEYS,
  CONTRACT_DATE_OFFSET_LIMITS,
  parseContractDateVariable,
  type ContractDateBaseKey,
  type ContractDateFormat,
  type ContractDateUnit,
} from "../../contracts/contractDateVariables";
import { getVariableByKey } from "../../contracts/contractVariables";
import { Button } from "../../ui/button";

interface AdvancedDateVariableBuilderProps {
  onSelect: (key: string) => void | Promise<unknown>;
  actionLabel?: string;
}

const UNIT_LABELS: Record<ContractDateUnit, string> = {
  days: "Dias corridos",
  business_days: "Dias úteis (segunda a sexta)",
  months: "Meses",
  years: "Anos",
};

const fieldClass = "w-full rounded-md border border-border bg-card px-2.5 py-2 text-xs text-foreground focus:outline-none focus:ring-2 focus:ring-primary-100";

/** Creates a token; the same date grammar is resolved in preview and generation. */
export function AdvancedDateVariableBuilder({
  onSelect,
  actionLabel = "Copiar variável de data",
}: AdvancedDateVariableBuilderProps) {
  const id = useId();
  const [baseKey, setBaseKey] = useState<ContractDateBaseKey>("current_date");
  const [direction, setDirection] = useState<"plus" | "minus">("plus");
  const [amount, setAmount] = useState("30");
  const [unit, setUnit] = useState<ContractDateUnit>("days");
  const [format, setFormat] = useState<ContractDateFormat>("br");
  const maxAmount = CONTRACT_DATE_OFFSET_LIMITS[unit];
  const numericAmount = Number(amount);
  const validAmount = /^\d+$/.test(amount) && Number.isInteger(numericAmount) && numericAmount <= maxAmount;
  const key = `${baseKey}_${direction}_${numericAmount}_${unit}${format === "br" ? "" : `_${format}`}`;
  const valid = validAmount && Boolean(parseContractDateVariable(key));

  return (
    <section aria-label="Construtor de datas" className="space-y-3 rounded-xl border border-primary-200 bg-primary-50/30 p-3">
      <div>
        <h5 className="text-sm font-semibold text-foreground">Datas e vencimentos</h5>
        <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
          Calcule uma data somando ou subtraindo um prazo. A data atual é calculada ao gerar o documento.
        </p>
      </div>
      <div className="space-y-1">
        <label htmlFor={`${id}-base`} className="text-xs font-medium">Data de referência</label>
        <select id={`${id}-base`} value={baseKey} onChange={(event) => setBaseKey(event.target.value as ContractDateBaseKey)} className={fieldClass}>
          {CONTRACT_DATE_BASE_KEYS.map((key) => (
            <option key={key} value={key}>{key} — {getVariableByKey(key)?.label || key}</option>
          ))}
        </select>
        <code className="block break-all font-mono text-[11px] text-primary-700">{`{{${baseKey}}}`}</code>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <div className="space-y-1">
          <label htmlFor={`${id}-direction`} className="text-xs font-medium">Operação</label>
          <select id={`${id}-direction`} value={direction} onChange={(event) => setDirection(event.target.value as "plus" | "minus")} className={fieldClass}>
            <option value="plus">Somar (plus)</option>
            <option value="minus">Subtrair (minus)</option>
          </select>
        </div>
        <div className="space-y-1">
          <label htmlFor={`${id}-amount`} className="text-xs font-medium">Quantidade</label>
          <input id={`${id}-amount`} type="number" min="0" max={maxAmount} step="1" value={amount} onChange={(event) => setAmount(event.target.value)} className={fieldClass} aria-invalid={!validAmount} aria-describedby={!validAmount ? `${id}-error` : undefined} />
        </div>
      </div>
      <div className="space-y-1">
        <label htmlFor={`${id}-unit`} className="text-xs font-medium">Unidade do prazo</label>
        <select id={`${id}-unit`} value={unit} onChange={(event) => setUnit(event.target.value as ContractDateUnit)} className={fieldClass}>
          {Object.entries(UNIT_LABELS).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
        </select>
      </div>
      <div className="space-y-1">
        <label htmlFor={`${id}-format`} className="text-xs font-medium">Formato da data</label>
        <select id={`${id}-format`} value={format} onChange={(event) => setFormat(event.target.value as ContractDateFormat)} className={fieldClass}>
          <option value="br">DD/MM/AAAA</option>
          <option value="iso">AAAA-MM-DD (ISO)</option>
          <option value="written">Por extenso (português)</option>
        </select>
      </div>
      {unit === "business_days" && <p className="text-xs leading-relaxed text-muted-foreground">Dias úteis excluem apenas sábados e domingos; feriados são contados.</p>}
      {(unit === "months" || unit === "years") && <p className="text-xs leading-relaxed text-muted-foreground">Quando o dia não existir no mês de destino, será usado o último dia desse mês.</p>}
      {!validAmount && <p id={`${id}-error`} role="alert" className="text-xs text-status-warning">Informe um número inteiro de 0 a {maxAmount}.</p>}
      {valid && <code className="block select-text break-all rounded-md border border-border bg-card p-2 font-mono text-xs text-primary-700">{`{{${key}}}`}</code>}
      <Button size="sm" className="w-full" disabled={!valid} onClick={() => onSelect(key)}>{actionLabel}</Button>
    </section>
  );
}

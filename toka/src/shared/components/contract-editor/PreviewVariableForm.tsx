import { useMemo, useState } from "react";
import { AlertTriangle, CheckCircle, FileText, Users, Hash, Type, List } from "lucide-react";
import { Button } from "../../ui/button";
import { getVariableByKey } from "../../contracts/contractVariables";
import {
  formatContractDateValue,
  parseContractDateVariable,
  resolveContractDateVariable,
  type ContractDateFormat,
} from "../../contracts/contractDateVariables";
import {
  classifyVariableValue,
  DOCX_SCALAR_OVERRIDES_KEY,
  formatTemplateScalarValue,
  resolveTemplateValue,
  type VariableMeta,
} from "../../utils/docx";

const PLACEHOLDER = "______________________";

interface PreviewVariableFormProps {
  templateVariables: string[];
  autoFilledData: Record<string, any>;
  onConfirm: (finalData: Record<string, any>) => void;
  onCancel: () => void;
  /**
   * Valor gravado nas variáveis que ficaram em branco.
   *
   * O padrão é a linha de preenchimento (`______`), que faz sentido quando o
   * documento é uma minuta para imprimir e assinar à mão. Quando o documento
   * vai para assinatura eletrônica, escrever essa linha no lugar do dado
   * coloca um texto sem sentido no contrato — nesse caso o chamador passa `""`.
   */
  emptyValue?: string;
}

function TypeIcon({ type }: { type: VariableMeta['type'] }) {
  const cls = "w-3 h-3 text-muted-foreground shrink-0";
  switch (type) {
    case 'object':
      return <Users className={cls} />;
    case 'array':
      return <List className={cls} />;
    case 'number':
      return <Hash className={cls} />;
    default:
      return <Type className={cls} />;
  }
}

function TypeBadge({ type, isDate = false }: { type: VariableMeta['type']; isDate?: boolean }) {
  const label: Record<string, string> = {
    string: 'Texto',
    number: 'Número',
    boolean: 'Sim/Não',
    object: 'Objeto',
    array: 'Lista',
    null: 'Vazio',
  };
  return (
    <span className="text-caption font-semibold uppercase tracking-wider text-muted-foreground bg-muted px-1.5 py-0.5 rounded border border-border">
      {isDate ? 'Data' : label[type] || type}
    </span>
  );
}

export function PreviewVariableForm({
  templateVariables,
  autoFilledData,
  onConfirm,
  onCancel,
  emptyValue = PLACEHOLDER,
}: PreviewVariableFormProps) {
  const variableKeys = useMemo(
    () => {
      const keys = templateVariables.map((key) => key.trim()).filter(Boolean);
      // Expose the source dates even when only a calculated deadline is in
      // the document, so missing bases can be filled during review.
      const bases = keys.flatMap((key) => {
        const date = parseContractDateVariable(key);
        return date && key !== date.baseKey ? [date.baseKey] : [];
      });
      return Array.from(new Set([...bases, ...keys]));
    },
    [templateVariables],
  );

  const [formData, setFormData] = useState<Record<string, string>>(() => {
    const initial: Record<string, string> = {};
    for (const key of variableKeys) {
      const value = resolveTemplateValue(autoFilledData, key);
      const meta = classifyVariableValue(value);
      if (meta?.type === 'string' || meta?.type === 'number' || meta?.type === 'boolean') {
        initial[key] = value === PLACEHOLDER || value === null || value === undefined
          ? ''
          : meta.type === 'boolean'
            ? (value ? 'Sim' : 'Não')
            : String(value);
      } else {
        initial[key] = '';
      }
    }
    return initial;
  });
  const [editedKeys, setEditedKeys] = useState<Set<string>>(() => new Set());
  const [dateFormats, setDateFormats] = useState<Record<string, ContractDateFormat>>({});

  const reviewedData = useMemo(() => {
    const context = { ...autoFilledData };
    for (const key of variableKeys) {
      const date = parseContractDateVariable(key);
      if (date && !Object.prototype.hasOwnProperty.call(context, date.baseKey)) {
        const base = resolveTemplateValue(autoFilledData, date.baseKey);
        if (base !== undefined) context[date.baseKey] = base;
      }
    }
    for (const key of editedKeys) {
      const original = resolveTemplateValue(autoFilledData, key);
      if (Array.isArray(original) || (original && typeof original === 'object')) continue;
      const value = formData[key]?.trim();
      if (value && typeof original === 'number' && !Number.isNaN(Number(value))) {
        context[key] = Number(value);
      } else if (value && typeof original === 'boolean') {
        const normalized = value.toLocaleLowerCase('pt-BR');
        context[key] = ['true', 'sim', '1'].includes(normalized) ? true
          : ['false', 'não', 'nao', '0'].includes(normalized) ? false : value;
      } else {
        context[key] = value || emptyValue;
      }
    }
    // Automatic date aliases and offsets must follow the reviewed source,
    // including aliases already present in the automatic context.
    for (const key of variableKeys) {
      const date = parseContractDateVariable(key);
      if (date && key !== date.baseKey && !editedKeys.has(key)) delete context[key];
    }
    return context;
  }, [autoFilledData, formData, editedKeys, emptyValue, variableKeys]);

  const varMeta = useMemo(() => {
    const result: Record<string, VariableMeta> = {};
    for (const key of variableKeys) {
      const { type, preview } = classifyVariableValue(resolveTemplateValue(reviewedData, key));
      result[key] = { key, type, preview };
    }
    return result;
  }, [reviewedData, variableKeys]);

  const fieldValue = (key: string): string => {
    if (!editedKeys.has(key)) {
      const value = resolveTemplateValue(reviewedData, key);
      if (value === null || value === undefined || value === PLACEHOLDER || typeof value === 'object') return '';
      return typeof value === 'boolean' ? (value ? 'Sim' : 'Não') : String(value);
    }
    return formData[key] || '';
  };

  const dateOutputValue = (key: string, format: ContractDateFormat): string => {
    const value = fieldValue(key);
    const formatted = formatContractDateValue(value, format);
    if (formatted !== undefined) return formatted;
    // Automatic written dates can be formatted again from their canonical
    // source. Explicit text overrides remain exactly as the user entered them.
    if (!editedKeys.has(key)) {
      const isoKey = `${key.replace(/_(iso|written)$/, '')}_iso`;
      const isoValue = resolveContractDateVariable(reviewedData, isoKey);
      const automaticDate = formatContractDateValue(isoValue, format);
      if (automaticDate !== undefined) return automaticDate;
    }
    return value;
  };

  const { filledCount, missingCount, groupedVars } = useMemo(() => {
    let filled = 0;
    let missing = 0;
    const grouped: Record<string, { key: string; label: string; isFilled: boolean; meta: VariableMeta | undefined }[]> = {};

    for (const key of variableKeys) {
      const systemVar = getVariableByKey(key);
      const category = systemVar?.category || "Outros";
      const meta = varMeta[key];
      const value = resolveTemplateValue(reviewedData, key);
      const manualValue = editedKeys.has(key) ? formData[key]?.trim() : undefined;

      // Considera preenchido se: array tem itens, objeto tem name, string não é placeholder
      let isFilled: boolean;
      if (meta?.type === 'array') {
        isFilled = Array.isArray(value) && value.length > 0;
      } else if (meta?.type === 'object') {
        const objectValue = value as Record<string, unknown> | null;
        isFilled = !!(objectValue?.name || objectValue?.label || objectValue?.title);
      } else if (meta?.type === 'null') {
        isFilled = !!manualValue;
      } else {
        isFilled = !!manualValue || (
          value !== null && value !== undefined && value !== '' && value !== PLACEHOLDER
        );
      }

      if (isFilled) filled++;
      else missing++;

      if (!grouped[category]) grouped[category] = [];
      grouped[category].push({
        key,
        label: systemVar?.label || key,
        isFilled,
        meta,
      });
    }

    return { filledCount: filled, missingCount: missing, groupedVars: grouped };
  }, [variableKeys, reviewedData, formData, editedKeys, varMeta]);

  const handleChange = (key: string, value: string) => {
    setFormData((prev) => ({ ...prev, [key]: value }));
    setEditedKeys((prev) => new Set([...prev, key]));
  };

  const handleSubmit = () => {
    // Mantém todo o contexto. A lista de metadados pode estar incompleta em
    // templates legados, e loops/caminhos pontuados precisam dos objetos raiz.
    const finalData: Record<string, any> = { ...reviewedData };
    const calculatedKeys = variableKeys.filter((key) => {
      const date = parseContractDateVariable(key);
      return date && date.baseKey !== key;
    });
    // Apply source dates before their calculated deadlines, independently of
    // the order of tags in the original document.
    for (const key of [...variableKeys.filter((key) => !calculatedKeys.includes(key)), ...calculatedKeys]) {
      const meta = varMeta[key];
      const autoValue = resolveTemplateValue(finalData, key);
      const val = editedKeys.has(key) ? formData[key]?.trim() : undefined;

      // Objetos e arrays permanecem intactos para loops; um override separado
      // pode corrigir apenas a representação escalar de {{variavel}}.
      if (meta?.type === 'object' || meta?.type === 'array') {
        if (val) {
          const existingOverrides = finalData[DOCX_SCALAR_OVERRIDES_KEY];
          finalData[DOCX_SCALAR_OVERRIDES_KEY] = {
            ...(existingOverrides && typeof existingOverrides === 'object'
              ? existingOverrides
              : {}),
            [key]: val,
          };
        }
        continue;
      }

      // Escalares: permite override e preserva false/0 (sem coerção por truthiness).
      if (val) {
        if (meta?.type === 'number' && !Number.isNaN(Number(val))) {
          finalData[key] = Number(val);
        } else if (meta?.type === 'boolean') {
          const normalized = val.toLocaleLowerCase('pt-BR');
          finalData[key] = ['true', 'sim', '1'].includes(normalized)
            ? true
            : ['false', 'não', 'nao', '0'].includes(normalized)
              ? false
              : val;
        } else {
          finalData[key] = val;
        }
      } else if (editedKeys.has(key)) {
        finalData[key] = emptyValue;
      } else {
        finalData[key] = autoValue !== null && autoValue !== undefined && autoValue !== ''
          ? autoValue
          : emptyValue;
      }
    }
    // Format output only after all date calculations are materialized, keeping
    // the canonical source intact while the user reviews dependent deadlines.
    for (const key of variableKeys) {
      const date = parseContractDateVariable(key);
      if (date) finalData[key] = dateOutputValue(key, dateFormats[key] || date.format) || emptyValue;
    }
    onConfirm(finalData);
  };

  return (
    <div className="flex flex-col bg-card rounded-xl overflow-hidden max-h-[70vh]">
      {/* Cabeçalho */}
      <div className="px-5 py-4 border-b border-border bg-card">
        <div className="flex items-center gap-2.5 mb-1.5">
          <div className="w-9 h-9 rounded-xl bg-primary-50 flex items-center justify-center shrink-0">
            <FileText className="w-5 h-5 text-primary-600" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-foreground">Preencher Variáveis do Contrato</h3>
            <p className="text-xs text-muted-foreground mt-0.5">
              Revise e preencha os campos antes de gerar o preview.
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2 mt-3">
          <span className="inline-flex items-center gap-1 text-[11px] font-medium text-status-ok bg-status-ok-soft border border-status-ok/30 px-2 py-0.5 rounded-full">
            <CheckCircle className="w-3 h-3" />
            {filledCount} preenchidas
          </span>
          {missingCount > 0 && (
            <span className="inline-flex items-center gap-1 text-[11px] font-medium text-status-warning bg-status-warning-soft border border-status-warning/30 px-2 py-0.5 rounded-full">
              <AlertTriangle className="w-3 h-3" />
              {missingCount} pendentes
            </span>
          )}
        </div>
      </div>

      {/* Lista de variáveis */}
      <div className="flex-1 overflow-y-auto min-h-0 custom-scrollbar">
        <div className="px-5 py-4 space-y-5">
          {Object.entries(groupedVars).map(([category, vars]) => (
            <div key={category}>
              <h4 className="text-caption font-bold text-muted-foreground uppercase tracking-wider mb-3">
                {category}
              </h4>
              <div className="space-y-3">
                {vars.map(({ key, label, isFilled, meta }) => {
                  const currentValue = fieldValue(key);
                  const isNonScalar = meta?.type === 'object' || meta?.type === 'array';
                  const autoValue = resolveTemplateValue(reviewedData, key);
                  const date = parseContractDateVariable(key);
                  const isCalculated = !!date && key !== date.baseKey;
                  const automaticScalarValue = formatTemplateScalarValue(autoValue);
                  const hasAutomaticScalarValue = automaticScalarValue !== null &&
                    automaticScalarValue !== undefined &&
                    String(automaticScalarValue) !== '';
                  const isPlaceholder = !isNonScalar && (
                    autoValue === PLACEHOLDER || autoValue === null || autoValue === undefined || autoValue === ''
                  );

                  return (
                    <div key={key} className="space-y-1">
                      <div className="flex flex-wrap items-center justify-between gap-1">
                        <label htmlFor={`contract-variable-${key}`} className="text-xs font-medium text-foreground flex items-center gap-1.5">
                          {label}
                          {meta && <TypeBadge type={meta.type} isDate={!!date} />}
                        </label>
                        <code className="text-xs font-mono text-foreground break-all">{`{{${key}}}`}</code>
                      </div>
                      {isCalculated && (
                        <p className="text-[11px] text-muted-foreground">
                          Calculada a partir de <code>{`{{${date.baseKey}}}`}</code>. Você pode editar o resultado.
                        </p>
                      )}

                      {isNonScalar ? (
                        <div className="space-y-2">
                          <div className="w-full px-3 py-2 text-sm border rounded-lg border-border bg-muted/50 flex items-center gap-2">
                            <TypeIcon type={meta!.type} />
                            <span className="text-xs text-foreground flex-1 truncate">
                              {meta!.preview || (meta!.type === 'array' ? 'Vazio' : 'Objeto')}
                            </span>
                            <span className="text-caption font-semibold text-muted-foreground bg-card px-1.5 py-0.5 rounded border border-border shrink-0">
                              Auto
                            </span>
                          </div>
                          {!hasAutomaticScalarValue && (
                            <input
                              id={`contract-variable-${key}`}
                              type="text"
                              value={currentValue}
                              onChange={(e) => handleChange(key, e.target.value)}
                              placeholder={meta!.type === 'array'
                                ? "Resumo textual opcional (não adiciona itens)..."
                                : "Preencher valor textual..."}
                              className="w-full px-3 py-2 text-sm border rounded-lg border-status-warning/30 bg-status-warning-soft placeholder:text-status-warning text-status-warning focus:outline-none focus:ring-2 focus:ring-primary-100 focus:border-primary-300"
                            />
                          )}
                        </div>
                      ) : (
                        <div className="relative">
                          <input
                            id={`contract-variable-${key}`}
                            type="text"
                            value={currentValue}
                            onChange={(e) => handleChange(key, e.target.value)}
                            placeholder={isPlaceholder ? (date ? "DD/MM/AAAA ou AAAA-MM-DD" : "Preencher valor...") : undefined}
                            className={`w-full px-3 py-2 text-sm border rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-primary-100 focus:border-primary-300 ${
                              isPlaceholder
                                ? "border-status-warning/30 bg-status-warning-soft placeholder:text-status-warning text-status-warning"
                                : "border-border bg-card text-foreground"
                            }`}
                          />
                          {!isPlaceholder && !currentValue && (
                            <span className="absolute right-2.5 top-1/2 -translate-y-1/2 text-caption font-semibold text-muted-foreground bg-muted px-1.5 py-0.5 rounded">
                              Auto
                            </span>
                          )}
                        </div>
                      )}
                      {date && !isNonScalar && (
                        <div className="space-y-1.5 pt-1">
                          <label htmlFor={`contract-variable-format-${key}`} className="block text-[11px] font-medium text-muted-foreground">
                            Formato da data no contrato
                          </label>
                          <select
                            id={`contract-variable-format-${key}`}
                            aria-label={`Formato da data no contrato: ${key}`}
                            value={dateFormats[key] || date.format}
                            onChange={(event) => setDateFormats((previous) => ({ ...previous, [key]: event.target.value as ContractDateFormat }))}
                            className="w-full rounded-lg border border-border bg-card px-3 py-2 text-xs text-foreground focus:outline-none focus:ring-2 focus:ring-primary-100 focus:border-primary-300"
                          >
                            <option value="br">Português brasileiro — DD/MM/AAAA</option>
                            <option value="written">Português brasileiro — por extenso</option>
                            <option value="iso">ISO — AAAA-MM-DD</option>
                          </select>
                          <p className="text-[11px] text-muted-foreground">
                            No contrato: <output aria-label={`Data no contrato: ${key}`} className="font-medium text-foreground">{dateOutputValue(key, dateFormats[key] || date.format) || 'A preencher'}</output>
                          </p>
                          <p className="text-[11px] text-muted-foreground">Para editar a data, use DD/MM/AAAA ou AAAA-MM-DD.</p>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Rodapé */}
      <div className="px-5 py-3 border-t border-border bg-card flex items-center justify-between">
        <Button variant="outline" onClick={onCancel}>
          Cancelar
        </Button>
        <Button onClick={handleSubmit}>Gerar Preview</Button>
      </div>
    </div>
  );
}

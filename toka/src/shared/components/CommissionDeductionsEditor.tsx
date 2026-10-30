import { useEffect, useRef, useState } from "react";
import { ArrowRight, Loader2, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "../ui/button";
import { Card } from "../ui/card";
import { MoneyInput } from "../ui/money-input";
import { PercentInput } from "../ui/percent-input";
import { formatMoneyBR, parseMoneyBR } from "../utils/format";
import { formatPercentBR } from "../utils/masks";
import { commissionService, type Commission, type DeductionKind } from "../services/commission.service";
import { brokerAdvanceService } from "../services/brokerAdvance.service";
import { tenantFinancialSettingsService } from "../services/tenantFinancialSettings.service";

interface CommissionDeductionsEditorProps {
  commission: Commission;
  onChange: (updated: Commission) => void;
  readOnly?: boolean;
  /**
   * Por que o lançamento está bloqueado. Sem isto o editor fica inerte e o
   * operador não descobre o que fazer — o backend só aceita desconto enquanto a
   * comissão está `PENDING`.
   */
  readOnlyReason?: string;
}

const KIND_OPTIONS: { value: DeductionKind; label: string }[] = [
  { value: "IRRF", label: "IRRF (Imposto de Renda)" },
  { value: "INSS", label: "INSS" },
  { value: "ISS", label: "ISS" },
  { value: "OUTROS", label: "Outros" },
  // Não é tributo: abate o adiantamento que a imobiliária fez ao corretor.
  { value: "ADVANCE", label: "Adiantamento (abate o saldo do corretor)" },
];

/** Rótulo de leitura dos descontos já lançados — `ADVANCE` cru não diz nada ao operador. */
const KIND_LABEL: Record<DeductionKind, string> = {
  IRRF: "IRRF",
  INSS: "INSS",
  ISS: "ISS",
  OUTROS: "OUTROS",
  ADVANCE: "Adiantamento",
};

/**
 * Como o desconto é informado: valor fixo em reais ou alíquota sobre o bruto.
 * A distinção importa porque o backend guarda `rate_pct`/`base_cents` só no
 * segundo caso — e é isso que permite auditar um valor calculado.
 */
type DeductionMode = "amount" | "rate";

/** Limite aceito pelo backend (validateDeductionRate). */
const MAX_RATE_PCT = 100;

/** Alíquotas padrão do tenant por tipo de retenção. */
type DefaultRates = { INSS: number; ISS: number };

const NO_DEFAULT_RATES: DefaultRates = { INSS: 0, ISS: 0 };

/**
 * Só INSS e ISS têm alíquota padrão configurável: IRRF depende da faixa da
 * tabela progressiva e "Outros" é livre — nos dois o campo começa vazio.
 */
const defaultRateFor = (kind: DeductionKind, rates: DefaultRates): number | undefined => {
  const pct = kind === "INSS" ? rates.INSS : kind === "ISS" ? rates.ISS : 0;
  return Number.isFinite(pct) && pct > 0 ? pct : undefined;
};

export function CommissionDeductionsEditor({ commission, onChange, readOnly, readOnlyReason }: CommissionDeductionsEditorProps) {
  const [kind, setKind] = useState<DeductionKind>("IRRF");
  const [description, setDescription] = useState("");
  const [amount, setAmount] = useState("");
  const [mode, setMode] = useState<DeductionMode>("amount");
  const [rate, setRate] = useState<number | undefined>(undefined);
  /** Verdadeiro depois que o operador digita a alíquota — o padrão do tenant não sobrescreve. */
  const [rateTouched, setRateTouched] = useState(false);
  /** Só mostra erro depois da primeira tentativa (digitar, sair do campo ou enviar). */
  const [touched, setTouched] = useState(false);
  const [defaultRates, setDefaultRates] = useState<DefaultRates>(NO_DEFAULT_RATES);
  const [ratesLoaded, setRatesLoaded] = useState(false);
  /**
   * Saldo de adiantamentos do corretor da comissão, em centavos. `null` =
   * desconhecido (ainda carregando ou a consulta falhou): a tela só bloqueia o
   * envio quando CONHECE o saldo — quem recusa o lançamento sem saldo é o
   * backend, e um aviso errado impediria um desconto legítimo.
   */
  const [advanceBalanceCents, setAdvanceBalanceCents] = useState<number | null>(null);
  const [advanceBalanceLoaded, setAdvanceBalanceLoaded] = useState(false);
  const [loading, setLoading] = useState(false);
  const pendingDeductionRef = useRef<{ payloadKey: string; requestID: string } | null>(null);

  // Os padrões do tenant só são usados no modo percentual: buscar antes disso
  // seria uma requisição por abertura de drawer sem uso nenhum.
  useEffect(() => {
    if (mode !== "rate" || ratesLoaded) return;
    let active = true;
    (async () => {
      try {
        const settings = await tenantFinancialSettingsService.get();
        if (!active) return;
        setDefaultRates({
          INSS: settings?.default_inss_pct ?? 0,
          ISS: settings?.default_iss_pct ?? 0,
        });
      } catch (err) {
        // Sem os padrões o lançamento por alíquota continua funcionando: o
        // operador digita a alíquota e o valor calculado aparece na hora.
        console.warn("CommissionDeductionsEditor: falha ao carregar alíquotas padrão", err);
      } finally {
        if (active) setRatesLoaded(true);
      }
    })();
    return () => {
      active = false;
    };
  }, [mode, ratesLoaded]);

  // Pré-preenche a alíquota do tipo escolhido (INSS/ISS) — tanto ao entrar no
  // modo percentual quanto ao trocar o tipo, e também quando os padrões do
  // tenant chegam depois. Um valor digitado pelo operador nunca é sobrescrito.
  useEffect(() => {
    if (mode !== "rate" || rateTouched) return;
    setRate(defaultRateFor(kind, defaultRates));
  }, [mode, kind, rateTouched, defaultRates]);

  // O saldo de adiantamentos só interessa ao tipo ADVANCE — buscar antes disso
  // seria uma requisição por abertura de drawer sem uso nenhum. O saldo é o do
  // corretor DA comissão: é o desconto dele que o backend consome.
  useEffect(() => {
    if (kind !== "ADVANCE" || advanceBalanceLoaded) return;
    const brokerId = commission.broker_id;
    if (!brokerId) {
      setAdvanceBalanceCents(0);
      setAdvanceBalanceLoaded(true);
      return;
    }
    let active = true;
    (async () => {
      try {
        const balance = await brokerAdvanceService.outstandingForBroker(brokerId);
        if (active) setAdvanceBalanceCents(balance.outstanding_cents);
      } catch (err) {
        // Sem o saldo o lançamento continua possível: o backend é quem valida.
        console.warn("CommissionDeductionsEditor: falha ao carregar o saldo de adiantamentos", err);
        if (active) setAdvanceBalanceCents(null);
      } finally {
        if (active) setAdvanceBalanceLoaded(true);
      }
    })();
    return () => {
      active = false;
    };
  }, [kind, advanceBalanceLoaded, commission.broker_id]);

  const gross = commission.gross_amount ?? commission.amount;
  /** Base de cálculo em centavos — é o bruto da comissão que sofre a retenção. */
  const baseCents = Math.round((Number.isFinite(gross) ? gross : 0) * 100);
  const calculatedAmount =
    rate !== undefined && rate > 0 && baseCents > 0 ? Math.round((baseCents * rate) / 100) / 100 : 0;
  const parsedAmount = parseMoneyBR(amount);

  /** Saldo a recuperar do corretor, em reais (para exibição). */
  const advanceBalance =
    advanceBalanceCents === null ? undefined : (advanceBalanceCents ?? 0) / 100;

  /**
   * O valor digitado passa do saldo do corretor? O backend recusa (400) o
   * lançamento nesse caso, então a tela avisa ANTES do envio e bloqueia o submit.
   */
  const exceedsAdvanceBalance =
    kind === "ADVANCE" &&
    mode === "amount" &&
    advanceBalanceCents !== null &&
    parsedAmount !== null &&
    parsedAmount > 0 &&
    Math.round(parsedAmount * 100) > advanceBalanceCents;

  const advanceExcessMessage = `O valor excede o saldo de adiantamentos do corretor (${formatMoneyBR((advanceBalanceCents ?? 0) / 100)}). O servidor recusaria este lançamento — ajuste o valor ou registre um novo adiantamento.`;

  const currentError = (() => {
    if (mode === "amount") {
      if (parsedAmount === null || parsedAmount <= 0) return "Informe um valor maior que zero.";
      // Antecipa a recusa do backend: um desconto de adiantamento sem saldo
      // correspondente reduziria o líquido do corretor abaixo do que ele já
      // recebeu de adiantamento. Bloquear aqui evita o 400 no envio.
      if (exceedsAdvanceBalance) return advanceExcessMessage;
      return undefined;
    }
    if (rate === undefined || !Number.isFinite(rate) || rate <= 0) return "Informe uma alíquota maior que zero.";
    if (rate > MAX_RATE_PCT) return "A alíquota não pode passar de 100%.";
    if (baseCents <= 0) return "Esta comissão não tem valor bruto para servir de base de cálculo.";
    if (calculatedAmount <= 0) return "O valor calculado ficou zerado. Revise a alíquota.";
    return undefined;
  })();

  const handleModeChange = (nextMode: DeductionMode) => {
    setMode(nextMode);
    setTouched(false);
  };

  const handleKindChange = (nextKind: DeductionKind) => {
    setKind(nextKind);
    setTouched(false);
    // ADVANCE é sempre valor fixo: o abatimento consome saldo em reais e não
    // tem alíquota nem base de cálculo para registrar.
    if (nextKind === "ADVANCE") setMode("amount");
  };

  const handleAdd = async () => {
    setTouched(true);
    if (currentError) return;
    const value = mode === "rate" ? calculatedAmount : (parsedAmount as number);
    const ratePct = mode === "rate" ? rate : undefined;
    const payloadKey = JSON.stringify([
      kind,
      description.trim(),
      value,
      ratePct ?? null,
      mode === "rate" ? baseCents : null,
    ]);
    let pendingDeduction = pendingDeductionRef.current;
    if (pendingDeduction?.payloadKey !== payloadKey) {
      pendingDeduction = { payloadKey, requestID: crypto.randomUUID() };
      pendingDeductionRef.current = pendingDeduction;
    }
    setLoading(true);
    try {
      const updated = await commissionService.addDeduction(commission.id, {
        deduction_id: pendingDeduction.requestID,
        kind,
        description: description.trim(),
        amount: value,
        // rate_pct/base_cents só vão no lançamento por alíquota: no valor fixo
        // eles mentiriam sobre a origem do número.
        ...(mode === "rate" ? { rate_pct: ratePct, base_cents: baseCents } : {}),
      });
      onChange(updated);
      setKind("IRRF");
      setDescription("");
      setAmount("");
      setRateTouched(false);
      setTouched(false);
      pendingDeductionRef.current = null;
      toast.success("Dedução adicionada");
    } catch (err) {
      console.error(err);
      toast.error("Não foi possível adicionar a dedução.");
    } finally {
      setLoading(false);
    }
  };

  const handleRemove = async (deductionId?: string) => {
    if (!deductionId) {
      toast.error("Esta dedução antiga ainda não possui identificador. Recarregue a página após a migração.");
      return;
    }
    setLoading(true);
    try {
      const updated = await commissionService.removeDeduction(commission.id, deductionId);
      onChange(updated);
      toast.success("Dedução removida");
    } catch (err) {
      console.error(err);
      toast.error("Não foi possível remover a dedução.");
    } finally {
      setLoading(false);
    }
  };

  const net = commission.net_amount ?? gross;
  const deductionsTotal = gross - net;
  const deductionCount = commission.deductions?.length ?? 0;
  const displayedError = touched && !exceedsAdvanceBalance ? currentError : undefined;

  return (
    <Card className="p-4">
      <h4 className="text-sm font-bold mb-3">Descontos (IRRF, INSS, ISS, Adiantamento)</h4>

      {/* Bruto → Descontos → Líquido: a composição que explica o valor que o
          corretor recebe. Sem ela o operador só via o líquido e não tinha como
          conferir a retenção que o gerou. */}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm">
        <div>
          <p className="text-xs text-muted-foreground">Bruto</p>
          <p className="font-bold">{formatMoneyBR(gross)}</p>
        </div>
        <ArrowRight className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <div>
          <p className="text-xs text-muted-foreground">
            Descontos{deductionCount > 0 ? ` (${deductionCount})` : ""}
          </p>
          <p className={deductionsTotal > 0 ? "font-bold text-status-critical" : "font-bold text-muted-foreground"}>
            {deductionsTotal > 0 ? `−${formatMoneyBR(deductionsTotal)}` : formatMoneyBR(0)}
          </p>
        </div>
        <ArrowRight className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <div>
          <p className="text-xs text-muted-foreground">Líquido</p>
          <p className="font-bold text-status-ok">{formatMoneyBR(net)}</p>
        </div>
      </div>

      <div className="border-t border-border mt-3 pt-3 space-y-2">
        {(commission.deductions || []).map((d, idx) => {
          // Lançamento por alíquota: mostrar COMO o valor saiu. O valor continua
          // sendo o dado principal (é ele que abate o líquido), e a alíquota com
          // a base explica de onde ele veio.
          const hasRate = typeof d.rate_pct === "number" && d.rate_pct > 0;
          return (
            <div key={d.id || idx} className="flex items-center justify-between gap-3 text-sm">
              <div className="min-w-0">
                <div>
                  <span className="font-medium">{KIND_LABEL[d.kind] ?? d.kind}</span>
                  {d.description && <span className="text-muted-foreground ml-2">— {d.description}</span>}
                </div>
                {hasRate && (
                  <p className="text-xs text-muted-foreground">
                    {formatPercentBR(d.rate_pct)}% de {formatMoneyBR((d.base_cents ?? 0) / 100)} ={" "}
                    {formatMoneyBR(d.amount)}
                  </p>
                )}
              </div>
              <div className="flex items-center gap-2">
                <span className="font-bold">-{formatMoneyBR(d.amount)}</span>
                {!readOnly && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 text-status-critical"
                    onClick={() => handleRemove(d.id)}
                    disabled={loading || !d.id}
                    title="Remover dedução"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </Button>
                )}
              </div>
            </div>
          );
        })}
        {(commission.deductions?.length ?? 0) === 0 && <p className="text-xs text-muted-foreground">Nenhuma dedução registrada.</p>}
      </div>

      {!readOnly && (
        <div className="border-t border-border mt-3 pt-3">
          <p className="text-xs font-semibold mb-2">Adicionar desconto</p>
          <div className="flex flex-wrap items-center gap-2 mb-2">
            <span className="text-xs text-muted-foreground">Lançar por</span>
            <div
              role="group"
              aria-label="Forma de lançamento do desconto"
              className="inline-flex overflow-hidden rounded-xl border border-border"
            >
              {([
                { value: "amount", label: "Valor (R$)" },
                { value: "rate", label: "Percentual (%)" },
              ] as { value: DeductionMode; label: string }[]).map((option) => {
                // O adiantamento é sempre valor fixo: não existe alíquota sobre
                // o bruto para um abatimento de saldo.
                const disabled = kind === "ADVANCE" && option.value === "rate";
                return (
                  <button
                    key={option.value}
                    type="button"
                    aria-pressed={mode === option.value}
                    disabled={disabled}
                    title={disabled ? "O adiantamento é sempre lançado por valor: ele abate o saldo do corretor." : undefined}
                    onClick={() => handleModeChange(option.value)}
                    className={
                      disabled
                        ? "px-3 py-1.5 text-xs font-medium bg-muted text-muted-foreground/60 cursor-not-allowed"
                        : mode === option.value
                          ? "px-3 py-1.5 text-xs font-bold bg-primary text-primary-foreground"
                          : "px-3 py-1.5 text-xs font-medium bg-card text-muted-foreground hover:text-foreground"
                    }
                  >
                    {option.label}
                  </button>
                );
              })}
            </div>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-4 gap-2">
            <select
              aria-label="Tipo de desconto"
              value={kind}
              onChange={(e) => handleKindChange(e.target.value as DeductionKind)}
              className="px-3 py-2 bg-card border border-border rounded-xl text-sm"
            >
              {KIND_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
            <input
              type="text"
              aria-label="Descrição do desconto"
              placeholder="Descrição (opcional)"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              className="px-3 py-2 bg-card border border-border rounded-xl text-sm"
            />
            {mode === "amount" ? (
              <MoneyInput
                aria-label="Valor do desconto"
                placeholder="0,00"
                value={amount === "" ? undefined : (parsedAmount ?? undefined)}
                onChange={(v) => setAmount(v === undefined ? "" : String(v))}
                className="px-3 py-2 bg-card border border-border rounded-xl text-sm"
              />
            ) : (
              <PercentInput
                aria-label="Alíquota do desconto (%)"
                placeholder="0,00"
                value={rate}
                onChange={(v) => {
                  setRate(v);
                  setRateTouched(true);
                }}
                onBlur={() => setTouched(true)}
                aria-invalid={displayedError ? true : undefined}
                className="px-3 py-2 bg-card border border-border rounded-xl text-sm"
              />
            )}
            <Button onClick={handleAdd} disabled={loading}>
              {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Plus className="w-4 h-4" />}
              <span className="ml-2">Adicionar</span>
            </Button>
          </div>

          {mode === "rate" && (
            <div className="mt-2 rounded-xl border border-border bg-muted/40 p-3 space-y-1 text-xs text-muted-foreground">
              <p>
                Base de cálculo (bruto da comissão):{" "}
                <span className="font-bold text-foreground">{formatMoneyBR(gross)}</span>
                {rate !== undefined && rate > 0 && <> × {formatPercentBR(rate)}%</>}
              </p>
              {/* O valor NÃO é digitado neste modo: dizer isso evita que o
                  operador procure um campo de valor que não existe mais. */}
              <p>
                Valor calculado:{" "}
                <span className="text-sm font-bold text-foreground">{formatMoneyBR(calculatedAmount)}</span> — calculado
                pelo sistema a partir da alíquota, não digitado.
              </p>
            </div>
          )}

          {kind === "ADVANCE" && (
            <div className="mt-2 rounded-xl border border-border bg-muted/40 p-3 space-y-1 text-xs text-muted-foreground">
              {!advanceBalanceLoaded ? (
                <p>Consultando o saldo de adiantamentos do corretor…</p>
              ) : advanceBalance === undefined ? (
                <p>
                  Não foi possível consultar o saldo de adiantamentos do corretor agora. O servidor valida o lançamento
                  no envio: sem saldo, o desconto é recusado.
                </p>
              ) : (
                <>
                  <p>
                    Saldo de adiantamentos do corretor:{" "}
                    <span className="font-bold text-foreground">{formatMoneyBR(advanceBalance)}</span>
                    {advanceBalance <= 0 && ' — não há adiantamento em aberto para abater.'}
                  </p>
                  <p>
                    O valor digitado abate esse saldo do adiantamento mais antigo para o mais novo, e não tem alíquota
                    nem base de cálculo: é sempre valor fixo.
                  </p>
                </>
              )}
              {/* Aviso ANTES do envio: o backend recusaria o desconto acima do
                  saldo, e o operador descobriria só depois de confirmar. */}
              {exceedsAdvanceBalance && (
                <p role="alert" className="font-medium text-status-critical">
                  {advanceExcessMessage}
                </p>
              )}
            </div>
          )}

          {displayedError && (
            <p role="alert" className="mt-2 text-xs font-medium text-status-critical">
              {displayedError}
            </p>
          )}

          <p className="text-xs text-muted-foreground mt-2">
            {kind === "ADVANCE"
              ? "O adiantamento é sempre valor fixo: o desconto consome o saldo que a imobiliária já entregou ao corretor."
              : mode === "rate"
                ? "A alíquota padrão da imobiliária vem de Configurações financeiras e pode ser ajustada neste lançamento."
                : "Sem alíquota, o valor é digitado e não fica registrado como cálculo sobre o bruto."}{" "}
            O lançamento fica liberado enquanto a comissão está <span className="font-bold">Pendente</span> — antes da
            aprovação da ordem de pagamento. Depois disso o líquido já está comprometido.
          </p>
        </div>
      )}

      {/* Bloqueio EXPLICADO: sem isto o formulário simplesmente não aparece e o
          operador não sabe se é permissão, status ou um erro da tela. */}
      {readOnly && readOnlyReason && (
        <div className="border-t border-border mt-3 pt-3">
          <div className="rounded-xl border border-status-warning/30 bg-status-warning-soft p-3">
            <p className="text-xs font-bold text-status-warning">Lançamento de desconto indisponível</p>
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{readOnlyReason}</p>
          </div>
        </div>
      )}
    </Card>
  );
}

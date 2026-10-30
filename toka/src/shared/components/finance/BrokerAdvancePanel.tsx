import { useState, type FormEvent } from 'react';
import { HandCoins } from 'lucide-react';
import { toast } from 'sonner';

import { formatMoneyBR } from '../../utils/format';
import { Input } from '../../ui/input';
import { Label } from '../../ui/label';
import { MoneyInput } from '../../ui/money-input';
import { Textarea } from '../../ui/textarea';
import { brokerAdvanceService, type BrokerAdvance } from '../../services/brokerAdvance.service';
import { FinancialActionNotice, FinancialActionPanel, PanelSection } from './FinancialActionPanel';

export interface BrokerAdvancePanelProps {
  /**
   * Corretores conhecidos pelo contexto compartilhado (AgentsContext). Vazio
   * quando quem opera não tem `brokers:read` — nesse caso o campo aceita o id
   * digitado, em vez de inventar uma lista.
   */
  brokers: { id: string; name?: string }[];
  onClose: () => void;
  /** Chamado com o adiantamento já registrado (a lista se atualiza por trás). */
  onCreated: (advance: BrokerAdvance) => void;
}

/** Data de hoje em AAAA-MM-DD (o input type="date" não trabalha com fuso). */
const today = (): string => {
  const now = new Date();
  const month = String(now.getMonth() + 1).padStart(2, '0');
  const day = String(now.getDate()).padStart(2, '0');
  return `${now.getFullYear()}-${month}-${day}`;
};

const errorMessage = (error: unknown): string => {
  const candidate = error as { response?: { data?: { message?: string } }; message?: string };
  return candidate?.response?.data?.message || candidate?.message || 'Não foi possível registrar o adiantamento.';
};

/**
 * Concessão de adiantamento ao corretor, no shell único das ações financeiras
 * (`FinancialActionPanel`): painel lateral, seções nomeadas, rodapé fixo e erro
 * da operação com `role="alert"`.
 *
 * O valor é digitado (é dinheiro saindo do caixa agora) e o saldo só passa a
 * existir depois — quem abate é o desconto do tipo "Adiantamento" lançado na
 * comissão, em Financeiro → Comissões.
 */
export function BrokerAdvancePanel({ brokers, onClose, onCreated }: BrokerAdvancePanelProps) {
  const [brokerId, setBrokerId] = useState('');
  const [amount, setAmount] = useState<number | undefined>(undefined);
  const [grantedAt, setGrantedAt] = useState(today);
  const [description, setDescription] = useState('');
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  /** Só mostra erro de campo depois da primeira tentativa de envio. */
  const [touched, setTouched] = useState(false);

  const hasBrokerCatalog = brokers.length > 0;
  const brokerError = brokerId.trim() === '' ? 'Selecione o corretor que recebeu o adiantamento.' : undefined;
  const amountError = amount === undefined || amount <= 0 ? 'Informe um valor maior que zero.' : undefined;
  const formError = brokerError ?? amountError;

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    setTouched(true);
    setSubmitError(null);
    if (formError) return;

    setSubmitting(true);
    try {
      const created = await brokerAdvanceService.create({
        broker_id: brokerId.trim(),
        amount: amount as number,
        description: description.trim() || undefined,
        granted_at: grantedAt || undefined,
      });
      toast.success('Adiantamento registrado.');
      onCreated(created);
      onClose();
    } catch (err) {
      const message = errorMessage(err);
      setSubmitError(message);
      toast.error(message);
      console.error(err);
    } finally {
      setSubmitting(false);
    }
  };

  const formId = 'new-broker-advance';

  return (
    <FinancialActionPanel
      onClose={onClose}
      title="Novo adiantamento"
      subtitle="Dinheiro adiantado ao corretor, a recuperar nas próximas comissões"
      icon={<HandCoins className="size-5" />}
      iconClassName="bg-status-info-soft text-status-info"
      formId={formId}
      submitLabel="Registrar adiantamento"
      submitIcon={<HandCoins className="size-4" />}
      submitting={submitting}
      submittingLabel="Registrando…"
      error={submitError}
      ariaLabel="Novo adiantamento"
    >
      <form id={formId} onSubmit={handleSubmit} className="space-y-5">
        <PanelSection title="Quem recebeu" description="O saldo fica no nome deste corretor e abate nas comissões dele.">
          {hasBrokerCatalog ? (
            <div className="space-y-1.5">
              <Label htmlFor="advance-broker">Corretor</Label>
              <select
                id="advance-broker"
                value={brokerId}
                onChange={(event) => setBrokerId(event.target.value)}
                className="w-full rounded-xl border border-border bg-card px-3 py-2 text-sm outline-none focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20"
              >
                <option value="">Selecione o corretor…</option>
                {brokers.map((broker) => (
                  <option key={broker.id} value={broker.id}>
                    {broker.name?.trim() || broker.id}
                  </option>
                ))}
              </select>
            </div>
          ) : (
            <div className="space-y-1.5">
              <Label htmlFor="advance-broker-id">ID do corretor</Label>
              <Input
                id="advance-broker-id"
                value={brokerId}
                onChange={(event) => setBrokerId(event.target.value)}
                placeholder="ex.: 8f1c…"
              />
              <p className="text-xs text-muted-foreground">
                A lista de corretores não está disponível nesta conta (depende de <span className="font-bold">brokers:read</span>).
                Informe o id do corretor.
              </p>
            </div>
          )}
          {touched && brokerError && (
            <p role="alert" className="text-xs font-medium text-status-critical">
              {brokerError}
            </p>
          )}
        </PanelSection>

        <PanelSection title="Valor e data" description="O valor sai do caixa na concessão.">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="advance-amount">Valor (R$)</Label>
              <MoneyInput
                id="advance-amount"
                aria-label="Valor do adiantamento"
                placeholder="0,00"
                value={amount}
                onChange={setAmount}
                aria-invalid={touched && amountError ? true : undefined}
                className="px-3 py-2 bg-card border border-border rounded-xl text-sm"
              />
              {touched && amountError && (
                <p role="alert" className="text-xs font-medium text-status-critical">
                  {amountError}
                </p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="advance-granted-at">Data</Label>
              <Input
                id="advance-granted-at"
                type="date"
                aria-label="Data do adiantamento"
                value={grantedAt}
                onChange={(event) => setGrantedAt(event.target.value)}
              />
            </div>
          </div>
        </PanelSection>

        <PanelSection title="Descrição" description="Opcional — aparece na lista e no histórico do adiantamento.">
          <Textarea
            aria-label="Descrição do adiantamento"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            placeholder="ex.: adiantamento de comissão pedido em 10/09"
            rows={3}
          />
        </PanelSection>

        <FinancialActionNotice>
          Registrar o adiantamento <span className="font-bold">não</span> desconta nada da comissão sozinho. A
          recuperação acontece ao lançar um desconto do tipo <span className="font-bold">Adiantamento</span> na comissão
          do corretor (Financeiro → Comissões → Descontos), que consome este saldo do mais antigo para o mais novo.
          {amount !== undefined && amount > 0 && (
            <>
              {' '}
              Neste lançamento: <span className="font-bold">{formatMoneyBR(amount)}</span> a recuperar.
            </>
          )}
        </FinancialActionNotice>
      </form>
    </FinancialActionPanel>
  );
}

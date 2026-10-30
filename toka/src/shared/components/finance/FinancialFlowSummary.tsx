import type { FinancialFlow } from '../../services/financialReport.service';

export function FinancialFlowSummary({
  flow = 'all',
  compact = false,
}: {
  flow?: FinancialFlow | 'all';
  compact?: boolean;
}) {
  if (compact) {
    return (
      <aside className="rounded-xl border border-border bg-muted/40 p-4 text-xs leading-relaxed text-muted-foreground">
        <p className="mb-1 font-semibold text-foreground">Comissão e repasse automáticos</p>
        <p>
          A assinatura provisiona a comissão conforme a divisão acordada. O recebimento conciliado
          alimenta sua cobertura e gera o repasse {flow === 'sale' ? 'ao vendedor' : 'ao proprietário'},
          descontadas as retenções previstas. A comissão é liberada quando os recebimentos cobrem a obrigação.
          O pagamento bancário exige aprovação e confirmação.
        </p>
        {flow === 'sale' && (
          <p className="mt-2">Dação e troca compõem a venda, mas não geram saldo em dinheiro para repasse.</p>
        )}
      </aside>
    );
  }

  const beneficiary = flow === 'sale' ? 'vendedor' : flow === 'rent' ? 'proprietário' : 'vendedor ou proprietário';
  const steps = [
    {
      title: 'Contrato assinado',
      description: 'A comissão é provisionada conforme o percentual e a divisão acordados. O plano define os recebíveis da venda ou as cobranças da locação.',
    },
    {
      title: 'Recebimento conciliado',
      description: 'O recebimento cobre as obrigações e os valores de terceiros. A intermediação é receita bruta; as comissões dos corretores são despesas. Na locação, a taxa de administração é uma receita separada.',
    },
    {
      title: 'Comissão e repasse',
      description: `A comissão é liberada quando os recebimentos cobrem a obrigação. O repasse ao ${beneficiary} é gerado automaticamente sobre o saldo conciliado após retenções e deduções.`,
    },
    {
      title: 'Pagamento confirmado',
      description: 'Aprovar e confirmar o pagamento continua sendo uma ação do responsável financeiro, com comprovante quando exigido. Gerar a obrigação não movimenta a conta bancária.',
    },
  ];

  return (
    <section aria-label="Do contrato ao repasse" className="rounded-xl border border-border bg-muted/30 p-5">
      <h2 className="text-sm font-semibold text-foreground">Do contrato ao repasse</h2>
      <ol className="mt-4 grid gap-4 sm:grid-cols-2">
        {steps.map((step, index) => (
          <li key={step.title} className="flex gap-3">
            <span aria-hidden="true" className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary-50 text-xs font-bold text-primary-700">{index + 1}</span>
            <div>
              <h3 className="text-xs font-semibold text-foreground">{step.title}</h3>
              <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{step.description}</p>
            </div>
          </li>
        ))}
      </ol>
    </section>
  );
}

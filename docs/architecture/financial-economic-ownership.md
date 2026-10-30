# Titularidade e provisionamento financeiro

Revisão de 08/09/2026, aplicada ao fluxo de vendas e locações.

| Parcela | Reconhecimento | Liquidação |
| --- | --- | --- |
| Intermediação e administração da imobiliária | Receita do serviço, separada do recebimento bruto | Entrada de caixa identificada no recebimento |
| Comissão suportada pela imobiliária | Despesa e comissão a pagar, provisionadas uma vez | Baixa da obrigação; retenções continuam a pagar |
| Valor do proprietário ou vendedor | Obrigação de terceiro | Repasse limitado ao saldo disponível |
| Participação de parceiro | Obrigação ao parceiro, separada da receita própria | Baixa da obrigação correspondente |
| Encargo de responsabilidade do proprietário | Reserva do saldo do proprietário | Pagamento por conta do proprietário, sem despesa própria |
| Despesa de responsabilidade da imobiliária | Despesa própria | Saída de caixa da imobiliária |

Os novos lançamentos de intermediação usam receita bruta e despesa de comissão separadas. O valor retido pela imobiliária após a comissão não é confundido com a receita bruta do serviço. Essa representação não reclassifica automaticamente contratos históricos que adotavam receita líquida.

O plano de comissão é congelado na contratação. Recebimentos financiam as obrigações existentes e geram os repasses correspondentes, sem provisionar a mesma comissão outra vez. Na locação manual, a composição registra separadamente a administração, a comissão coberta naquele recebimento, a parcela retida e o valor do proprietário. O saldo de comissão financiado por outros meses não reduz novamente a margem do mês exibido.

A distribuição da comissão **sempre fecha 100%**, dos dois lados (criação do negócio no painel e resolução do plano financeiro): cada papel com corretor recebe o percentual configurado em Configurações › Comissões e o restante é retido pela imobiliária como receita retida. Rateio informado incompleto é completado pela retenção — com registro em audit (`commission.split_filled_by_retention`) — e negócio sem nenhum participante retém 100% (`commission.no_participants` com o valor retido). Só uma soma **acima** de 100% falha a materialização: não existe retenção negativa e nenhum corretor recebe acima do configurado. Corrigir um rateio já congelado (venda assinada e recebida) é operação explícita: `cmd/repair-sale-commission` cancela a obrigação errada e materializa o rateio correto com o mesmo lastro de caixa.

A DRE operacional considera recebimentos e pagamentos do período. Para recebimentos manuais com razão vinculado, usa a receita efetivamente registrada nele; lançamentos antigos sem essa evidência preservam o tratamento líquido anterior. Comissões entram como despesa quando a receita é bruta. Repasses de terceiros não entram como despesa da imobiliária.

Cobranças recebidas sem alocação econômica permanecem no fluxo de caixa e são exibidas como **recebimentos a classificar**, fora da receita própria. Não há atribuição automática do aluguel inteiro ou do preço de venda à imobiliária.

Retenções de comissões não são saídas bancárias: a despesa pode ser maior que o pagamento líquido, e a diferença permanece nas obrigações a recolher. Aprovação e confirmação do pagamento são etapas separadas do provisionamento.

## Evidências automatizadas

- `manual_sale_financial_journey_postgres_integration_test.go`: provisão antes do recebimento, receita bruta, despesa, repasse, retenções e caixa líquido.
- `manual_rent_financial_journey_postgres_integration_test.go`: composição por recebimento, financiamento parcial, repetição sem duplicação e receita de administração.
- `financial_reporting_postgres_integration_test.go`: exclusão de repasses da DRE, preservação do legado e recebimentos sem classificação.
- `presentation.test.ts` e `EconomicResults.test.tsx`: apresentação da margem, despesas e valores a classificar.
- `zero_balance_test.go`, `transfer_zero_balance_test.go` e `managed_bill_zero_transfer_test.go`: repasse sem saldo a repassar encerra sem perna de caixa (inclusive ao consumir a última reserva de conta administrada), reabre quando o saldo volta e nunca inventa lançamento de R$ 0,00.
- `commission_resolver_test.go`: retenção residual fecha o plano em 100% (rateio parcial e negócio sem participantes).
- `dealBrokerParticipations.test.ts`: rateio inicial do negócio pela configuração de comissões, sempre fechando 100%.

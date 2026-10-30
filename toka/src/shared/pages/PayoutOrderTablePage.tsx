import { BadgeDollarSign } from "lucide-react";

import { PayoutOrderTable } from "../components/PayoutOrderTable";
import { FinanceAreaNav } from "../components/finance/FinanceAreaNav";
import { PayoutViewSwitcher } from "../components/payouts/PayoutViewSwitcher";

/**
 * Página de "Ordens de pagamento" — a ferramenta de consolidação do operador
 * (agrupamento em lote, aprovação em massa e controle de cobertura por aluguéis).
 * A "listagem simples" (linha a linha) fica como submodo acessível por este
 * botão, sem virar item de menu separado.
 */
export default function PayoutOrderTablePage() {
  return (
    <div className="p-6 max-w-7xl mx-auto space-y-6">
      <FinanceAreaNav />
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <BadgeDollarSign className="w-6 h-6 text-primary-600" />
            Ordens de pagamento
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            Consolide e aprove em lote as ordens de pagamento de comissões, repasses e royalties, acompanhando os recursos disponíveis para liquidação.
          </p>
        </div>
        <PayoutViewSwitcher />
      </div>

      <PayoutOrderTable />
    </div>
  );
}

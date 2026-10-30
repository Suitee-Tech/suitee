import { HandCoins } from "lucide-react";

import { ComissoesTab } from "./Finance/ComissoesTab";
import { FinanceAreaNav } from "../components/finance/FinanceAreaNav";

/**
 * Página de Comissões — componente autônomo (busca os próprios dados via
 * commissionService), agora com rota própria `/financeiro/comissoes`. Extraído
 * do mega-componente Finance para reduzir o estado centralizado.
 */
export default function ComissoesPage() {
  return (
    <div className="mx-auto max-w-7xl space-y-4 p-5 lg:p-6">
      <FinanceAreaNav />
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2 text-foreground">
          <HandCoins className="w-6 h-6 text-primary-600" />
          Comissões
        </h1>
        <p className="text-sm text-muted-foreground mt-1">
          Gestão de comissões dos corretores — aprovação, pagamento com comprovante, deduções e cancelamento.
        </p>
      </div>
      <ComissoesTab />
    </div>
  );
}

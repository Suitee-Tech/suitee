import { useState, type Dispatch } from "react";
import { Landmark } from "lucide-react";

import { ContasSection } from "./Finance/ContasSection";
import type { FilterAction } from "./Finance/state";
import { useContractsContext } from "../context/ContractsContext";
import { useFocusMode } from "../context/FocusModeContext";
import { ExportReportModal, type ExportConfig } from "../components/ExportReportModal";
import { FinanceAreaNav } from "../components/finance/FinanceAreaNav";

/**
 * Página "Contas & encargos" — envolve o ContasSection com estado próprio
 * (busca, contrato, export), extraído do mega-componente Finance.
 */
export default function ContasPage() {
  const { contracts } = useContractsContext();
  const { focusMode } = useFocusMode();
  const [selectedContractId] = useState<string | null>(null);
  const [contasSearch, setContasSearch] = useState("");
  const [exportConfig, setExportConfig] = useState<ExportConfig | null>(null);

  // O ContasSection usa filtersDispatch só para SET_TAB_SEARCH ('contas').
  const dispatchContas: Dispatch<FilterAction> = (action) => {
    if ("type" in action && action.type === "SET_TAB_SEARCH" && "tab" in action && action.tab === "contas") {
      setContasSearch("search" in action ? action.search ?? "" : "");
    }
  };

  return (
    <div className="mx-auto max-w-7xl space-y-6 p-5 lg:p-6">
      <FinanceAreaNav />
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2 text-foreground">
          <Landmark className="w-6 h-6 text-primary-600" /> Contas &amp; encargos
        </h1>
        <p className="text-sm text-muted-foreground mt-1">
          Contas e encargos por imóvel, com configuração de pagamento e acompanhamento das obrigações.
        </p>
      </div>

      <ContasSection
        contracts={contracts}
        selectedContractId={selectedContractId}
        focusMode={focusMode}
        contasSearch={contasSearch}
        filtersDispatch={dispatchContas}
        setExportConfig={setExportConfig}
      />

      {exportConfig && <ExportReportModal config={exportConfig} onClose={() => setExportConfig(null)} />}
    </div>
  );
}

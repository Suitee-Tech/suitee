import { useState } from "react";
import { Download, Loader2, Shield } from "lucide-react";
import { toast } from "sonner";
import { settingsService } from "../../../shared/services/settings.service";

export function LGPDDataExportCard() {
  const [loading, setLoading] = useState(false);

  const handleExport = async () => {
    if (loading) return;
    setLoading(true);
    try {
      const data = await settingsService.getMyLGPDData();
      const json = JSON.stringify(data, null, 2);
      const blob = new Blob([json], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `meus-dados-lgpd-${new Date().toISOString().split("T")[0]}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      toast.success("Seus dados foram baixados.");
    } catch (err) {
      const msg =
        (err as { response?: { data?: { message?: string } } })?.response?.data?.message ||
        "Não foi possível gerar o arquivo. Tente novamente em alguns minutos.";
      toast.error(msg);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="bg-card border border-border rounded-3xl p-6 shadow-sm mb-6">
      <div className="flex items-start gap-3 mb-3">
        <div className="w-10 h-10 rounded-xl bg-status-ok-soft flex items-center justify-center flex-shrink-0 text-status-ok">
          <Shield className="w-5 h-5" />
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-bold text-foreground">Privacidade — Lei Geral de Proteção de Dados</p>
          <p className="text-xs text-muted-foreground mt-0.5">
            Baixe seus dados de cadastro e preferências. O arquivo também indica como acessar informações mantidas por outros serviços.
          </p>
        </div>
      </div>

      <button
        type="button"
        onClick={handleExport}
        disabled={loading}
        className="w-full flex items-center justify-center gap-2 px-4 py-2.5 bg-muted text-foreground text-sm font-medium rounded-xl hover:bg-muted/80 transition-colors disabled:opacity-50"
      >
        {loading ? (
          <>
            <Loader2 className="w-4 h-4 animate-spin" />
            Gerando...
          </>
        ) : (
          <>
            <Download className="w-4 h-4" />
            Baixar meus dados (JSON)
          </>
        )}
      </button>

      {/* Os documentos legais também precisam ser alcançáveis de dentro do
          portal: quem já está logado não vê mais a tela de login, e o modal de
          aceite só aparece quando há versão nova para aceitar. */}
      <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
        <a href="/portal/privacy" className="text-primary-600 hover:text-primary-700 underline">
          Política de Privacidade
        </a>
        <span className="text-muted-foreground" aria-hidden="true">
          ·
        </span>
        <a href="/portal/terms" className="text-primary-600 hover:text-primary-700 underline">
          Termos de Uso
        </a>
      </div>
    </div>
  );
}

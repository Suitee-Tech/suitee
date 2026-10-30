import { useState, useEffect, useCallback, useMemo } from "react";
import {
  FileText,
  Receipt,
  DollarSign,
  AlertCircle,
  Calendar,
  History,
  Download,
  CheckCircle2,
  FileCode,
  Loader2,
  RefreshCw,
  Trash2,
  Users,
  ChevronDown,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";

import {
  fiscalService,
  type FiscalDeadlineDTO,
  type FiscalReportHistoryDTO,
  type DimobReportDTO,
  type InformeDTO,
} from "../services/fiscal.service";
import { Button } from "../ui/button";
import { ResponsiveTable } from "../ui/responsive-table";
import { getCompany } from "../services/auth.service";
import { Can } from "../../lib/permissions/Can";
import { ConfirmDialog } from "../components/ConfirmDialog";

const currentYear = new Date().getFullYear();
/** Anos oferecidos no seletor: o ano corrente e os três anteriores. */
const YEAR_OPTIONS = [currentYear, currentYear - 1, currentYear - 2, currentYear - 3];

const HISTORY_PAGE_SIZE = 10;

const DEADLINE_STYLE: Record<
  FiscalDeadlineDTO["status"],
  { bg: string; badge: string; badgeText: string; icon: string }
> = {
  UPCOMING: {
    bg: "bg-primary-50/50 border-primary-100",
    badge: "bg-primary-100 text-primary-700",
    badgeText: "Futuro",
    icon: "text-primary-500",
  },
  DUE_SOON: {
    bg: "bg-status-critical-soft border-status-critical/20",
    badge: "bg-status-critical/90 text-white",
    badgeText: "Próximo",
    icon: "text-status-critical",
  },
  OVERDUE: {
    bg: "bg-status-critical-soft border-status-critical/30",
    badge: "bg-status-critical/90 text-white",
    badgeText: "Atrasado",
    icon: "text-status-critical",
  },
  COMPLETED: {
    bg: "bg-status-ok-soft/50 border-status-ok/20",
    badge: "bg-status-ok-soft text-status-ok",
    badgeText: "Concluído",
    icon: "text-status-ok",
  },
};

const REPORT_TAG_COLORS: Record<string, string> = {
  DIMOB: "bg-primary-50 text-primary-700 border-primary-200",
  NFSE: "bg-status-ok-soft text-status-ok border-status-ok/30",
  INFORME: "bg-status-warning-soft text-status-warning border-status-warning/30",
};

const REPORT_TAG_LABELS: Record<string, string> = {
  DIMOB: "DIMOB",
  NFSE: "NFS-e",
  INFORME: "Informe",
};

const HISTORY_TYPE_FILTERS = [
  { value: "", label: "Todos os tipos" },
  { value: "DIMOB", label: "DIMOB" },
  { value: "INFORME", label: "Informes" },
  { value: "NFSE", label: "NFS-e" },
];

const DIMOB_STATUS_LABEL: Record<string, string> = {
  PENDING: "Não gerada",
  GENERATING: "Gerando...",
  READY: "Pronta para envio",
  SUBMITTED: "Transmitida",
  ERROR: "Erro na geração",
};

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("pt-BR", {
    day: "2-digit",
    month: "long",
    year: "numeric",
  });
}

function formatDateTime(iso?: string): string {
  if (!iso) return "";
  return new Date(iso).toLocaleString("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * Informes gerados antes da separação por fluxo (T-06) não têm os campos de
 * locação/venda — a tabela cai nos totais legados e avisa para regravar.
 */
function hasFlowBreakdown(informe: InformeDTO): boolean {
  return (informe.rental_count ?? 0) > 0 || (informe.sale_count ?? 0) > 0;
}

function formatCurrency(value: number): string {
  return value.toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
}

/** Extrai o nome do arquivo do Content-Disposition, com fallback. */
function filenameFromHeaders(headers: unknown, fallback: string): string {
  const disposition = (headers as Record<string, string> | undefined)?.["content-disposition"];
  const match = disposition?.match(/filename="?([^";]+)"?/i);
  return match?.[1] ?? fallback;
}

/** Salva um blob como download, sem depender de window.open (Bearer). */
function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export function FiscalReports() {
  const [year, setYear] = useState(currentYear);
  const [deadlines, setDeadlines] = useState<FiscalDeadlineDTO[]>([]);
  const [reportHistory, setReportHistory] = useState<FiscalReportHistoryDTO[]>([]);
  const [reportTotal, setReportTotal] = useState(0);
  const [dimobStatus, setDimobStatus] = useState<DimobReportDTO | null>(null);
  const [informes, setInformes] = useState<InformeDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMoreReports, setLoadingMoreReports] = useState(false);
  const [generatingDimob, setGeneratingDimob] = useState(false);
  const [generatingInformes, setGeneratingInformes] = useState(false);
  const [downloadingDimob, setDownloadingDimob] = useState(false);
  const [historyType, setHistoryType] = useState("");
  const [reportToArchive, setReportToArchive] = useState<FiscalReportHistoryDTO | null>(null);

  const fetchData = useCallback(async (targetYear: number, type: string, offset = 0, append = false) => {
    if (!append) setLoading(true);
    try {
      const [calendarRes, historyRes, dimobRes, informesRes] = await Promise.allSettled([
        fiscalService.getCalendar(targetYear),
        fiscalService.getReportHistory({
          limit: HISTORY_PAGE_SIZE,
          offset,
          type: type || undefined,
          year: targetYear,
        }),
        fiscalService.getDimobStatus(targetYear),
        fiscalService.listInformes(targetYear),
      ]);

      if (calendarRes.status === "fulfilled") setDeadlines(calendarRes.value.data);
      if (historyRes.status === "fulfilled") {
        const page = historyRes.value.data ?? [];
        setReportHistory((prev) => (append ? [...prev, ...page] : page));
        const total = Number((historyRes.value.meta as { total?: number } | undefined)?.total ?? page.length);
        setReportTotal(total);
      }
      if (dimobRes.status === "fulfilled") setDimobStatus(dimobRes.value.data);
      if (informesRes.status === "fulfilled") setInformes(informesRes.value.data ?? []);

      const failed = [calendarRes, historyRes, dimobRes, informesRes].filter((r) => r.status === "rejected").length;
      if (failed === 4) {
        toast.error("Não foi possível carregar os dados fiscais. Tente novamente.");
      } else if (failed > 0 && !append) {
        toast.warning("Alguns dados fiscais não puderam ser carregados.");
      }
    } catch {
      toast.error("Erro ao carregar dados fiscais.");
    } finally {
      setLoading(false);
      setLoadingMoreReports(false);
    }
  }, []);

  useEffect(() => {
    void fetchData(year, historyType);
  }, [fetchData, year, historyType]);

  const handleGenerateDimob = async () => {
    setGeneratingDimob(true);
    try {
      // A declarante (R01) vem da empresa atual: sem CNPJ o arquivo não pode
      // ser montado e o download responde 400. O fallback preserva o
      // comportamento antigo — o backend avisa com mensagem própria.
      let companyCnpj = "";
      let companyName = "";
      try {
        const companyId = localStorage.getItem("current_company_id");
        if (companyId) {
          const company = await getCompany(companyId);
          companyCnpj = company?.tax_id ?? "";
          companyName = company?.legal_name || company?.name || "";
        }
      } catch {
        // segue sem a declarante — o download exibirá a mensagem do backend
      }
      const res = await fiscalService.generateDimob(year, companyCnpj, companyName);
      setDimobStatus(res.data);
      toast.success(`DIMOB ${year} gerada. Baixe o arquivo e importe no programa da Receita Federal.`);
      void fetchData(year, historyType);
    } catch {
      toast.error("Erro ao gerar DIMOB.");
    } finally {
      setGeneratingDimob(false);
    }
  };

  const handleDownloadDimob = async () => {
    setDownloadingDimob(true);
    try {
      const response = await fiscalService.downloadFile(`/fiscal/dimob/${year}/file`);
      saveBlob(response.data, `dimob-${year}.txt`);
      // Advertências não bloqueantes (contrato sem número/data, CPF/CNPJ de
      // parte ausente) chegam no header — o operador precisa vê-las antes de
      // transmitir.
      const warnings = (response.headers as Record<string, string> | undefined)?.["x-dimob-warnings"];
      if (typeof warnings === "string" && warnings.trim() !== "") {
        toast.warning(`DIMOB gerada com advertências: ${warnings}`, { duration: 15000 });
      } else {
        toast.success(`Arquivo dimob-${year}.txt baixado.`);
      }
    } catch (error) {
      const message = (error as { response?: { data?: { message?: string } } })?.response?.data?.message;
      toast.error(message || "Erro ao baixar a DIMOB.");
    } finally {
      setDownloadingDimob(false);
    }
  };

  const handleGenerateInformes = async () => {
    setGeneratingInformes(true);
    try {
      await fiscalService.generateInformes(year);
      toast.success("Informes de rendimentos gerados!");
      void fetchData(year, historyType);
    } catch {
      toast.error("Erro ao gerar informes.");
    } finally {
      setGeneratingInformes(false);
    }
  };

  const handleDownloadInforme = async (informe: InformeDTO) => {
    try {
      const response = await fiscalService.downloadInformePDF(informe.id);
      const url = URL.createObjectURL(new Blob([response.data], { type: "application/pdf" }));
      window.open(url, "_blank", "noopener,noreferrer");
      window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
    } catch {
      toast.error("Erro ao abrir o informe. Tente novamente.");
    }
  };

  const handleDownloadReport = async (report: FiscalReportHistoryDTO) => {
    if (!report.file_url) {
      toast.error("O arquivo deste relatório ainda não foi gerado.");
      return;
    }
    try {
      const apiPath = report.file_url.startsWith("/api/v1")
        ? report.file_url.slice("/api/v1".length)
        : report.file_url;
      const response = await fiscalService.downloadFile(apiPath);
      const filename = filenameFromHeaders(response.headers, `${report.name}.txt`);
      saveBlob(response.data, filename);
      const warnings = (response.headers as Record<string, string> | undefined)?.["x-dimob-warnings"];
      if (typeof warnings === "string" && warnings.trim() !== "") {
        toast.warning(`Arquivo gerado com advertências: ${warnings}`, { duration: 15000 });
      }
    } catch (error) {
      const message = (error as { response?: { data?: { message?: string } } })?.response?.data?.message;
      toast.error(message || "Erro ao baixar relatório.");
    }
  };

  const handleArchiveReport = async () => {
    if (!reportToArchive) return;
    const target = reportToArchive;
    try {
      await fiscalService.archiveReport(target.id);
      setReportHistory((prev) => prev.filter((item) => item.id !== target.id));
      setReportTotal((prev) => Math.max(0, prev - 1));
      toast.success("Relatório arquivado: sai da listagem e o registro fica para auditoria.");
    } catch {
      toast.error("Erro ao arquivar o relatório.");
    }
  };

  const handleLoadMoreReports = () => {
    setLoadingMoreReports(true);
    void fetchData(year, historyType, reportHistory.length, true);
  };

  const hasDueSoonDeadline = deadlines.some((d) => d.status === "DUE_SOON" || d.status === "OVERDUE");
  const dimobReady = dimobStatus?.status === "READY" || dimobStatus?.status === "SUBMITTED";
  const dimobLabel = DIMOB_STATUS_LABEL[dimobStatus?.status ?? "PENDING"] ?? "Não gerada";
  const missingDeclarante = dimobReady && !dimobStatus?.declarante_cnpj;
  const informesTotalNet = useMemo(
    () => informes.reduce((sum, informe) => sum + (informe.total_net ?? 0), 0),
    [informes],
  );
  const hasMoreReports = reportHistory.length < reportTotal;

  return (
    <div className="flex flex-col h-full bg-muted/50 font-sans overflow-hidden">
      <div className="flex-none px-6 lg:px-8 py-6 border-b border-border bg-card sticky top-0 z-10">
        <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
          <div>
            <div className="flex items-center gap-3 mb-1">
              <h1 className="text-2xl sm:text-3xl font-bold text-foreground dark:text-white tracking-tight">
                Fiscal
              </h1>
              <span className="px-2.5 py-1 bg-primary-50 dark:bg-primary-900/30 text-primary-700 dark:text-primary-300 text-xs font-bold rounded-full border border-primary-100 dark:border-primary-800">
                Ano {year}
              </span>
            </div>
            <p className="text-sm text-muted-foreground">
              Declarações fiscais obrigatórias, notas fiscais de serviço e informes de rendimentos
            </p>
          </div>
          <div className="flex items-center gap-3">
            <label className="flex items-center gap-2 text-sm text-muted-foreground">
              <span className="hidden sm:inline">Ano-calendário</span>
              <div className="relative">
                <select
                  aria-label="Ano-calendário"
                  value={year}
                  onChange={(event) => setYear(Number(event.target.value))}
                  className="appearance-none rounded-lg border border-border bg-card py-2 pl-3 pr-8 text-sm font-medium text-foreground"
                >
                  {YEAR_OPTIONS.map((option) => (
                    <option key={option} value={option}>
                      {option}
                    </option>
                  ))}
                </select>
                <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              </div>
            </label>
            <Button variant="outline" onClick={() => void fetchData(year, historyType)} disabled={loading}>
              <RefreshCw className={clsx("w-4 h-4", loading && "animate-spin")} />
              Atualizar
            </Button>
          </div>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-6 lg:p-8 space-y-8">
        {loading && (
          <div className="flex items-center justify-center py-20">
            <Loader2 className="w-8 h-8 text-primary-500 animate-spin" />
          </div>
        )}

        {!loading && (
          <>
            {hasDueSoonDeadline && (
              <div className="bg-status-warning-soft border border-status-warning/30 rounded-xl p-4 flex items-start gap-3">
                <AlertCircle className="w-5 h-5 text-status-warning flex-shrink-0 mt-0.5" />
                <p className="text-sm text-status-warning font-medium">
                  <span className="font-bold">Atenção:</span> Existem obrigações fiscais com prazo próximo. Verifique o
                  calendário fiscal abaixo.
                </p>
              </div>
            )}

            <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
              {/* DIMOB */}
              <div className="bg-card rounded-2xl border border-border shadow-sm flex flex-col overflow-hidden hover:border-primary-300 dark:hover:border-primary-600 transition-colors">
                <div className="p-6 flex-1 flex flex-col">
                  <div className="flex justify-between items-start mb-4">
                    <div className="flex items-center gap-3">
                      <div className="w-12 h-12 rounded-xl bg-primary-50 dark:bg-primary-900/30 flex items-center justify-center text-primary-600 dark:text-primary-400">
                        <FileText className="w-6 h-6" />
                      </div>
                      <div>
                        <h2 className="text-lg font-bold text-foreground dark:text-white">DIMOB</h2>
                        <p className="text-xs text-muted-foreground font-medium">
                          Declaração de Atividades Imobiliárias
                        </p>
                      </div>
                    </div>
                    <span
                      className={clsx(
                        "px-2 py-1 text-micro font-bold rounded uppercase tracking-wider",
                        dimobStatus?.status === "ERROR"
                          ? "bg-status-critical-soft text-status-critical"
                          : dimobReady
                            ? "bg-status-ok-soft text-status-ok"
                            : "bg-muted text-muted-foreground",
                      )}
                    >
                      {dimobLabel}
                    </span>
                  </div>
                  <p className="text-sm text-muted-foreground flex-1">
                    Arquivo para envio à Receita Federal com os aluguéis recebidos no ano-calendário {year}, no leiaute
                    oficial do programa da DIMOB.
                  </p>

                  {dimobStatus && (dimobStatus.total_rentals > 0 || dimobStatus.total_sales > 0) && (
                    <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs font-medium text-muted-foreground">
                      <span>{dimobStatus.total_rentals} locações no arquivo</span>
                      {dimobStatus.total_sales > 0 && (
                        <span title="A intermediação de venda entra na ficha R04, ainda não emitida nesta versão">
                          {dimobStatus.total_sales} vendas (fora do arquivo nesta versão)
                        </span>
                      )}
                    </div>
                  )}

                  {dimobReady && (
                    <dl className="mt-4 space-y-1 text-xs">
                      <div className="flex gap-1.5">
                        <dt className="text-muted-foreground">Declarante:</dt>
                        <dd className="font-medium text-foreground truncate">
                          {dimobStatus?.declarante_cnpj
                            ? `${dimobStatus.declarante_nome || "—"} · ${dimobStatus.declarante_cnpj}`
                            : "não informada"}
                        </dd>
                      </div>
                      {dimobStatus?.generated_at && (
                        <div className="flex gap-1.5">
                          <dt className="text-muted-foreground">Gerada em:</dt>
                          <dd className="font-medium text-foreground">{formatDateTime(dimobStatus.generated_at)}</dd>
                        </div>
                      )}
                    </dl>
                  )}

                  {missingDeclarante && (
                    <p className="mt-3 flex items-start gap-2 rounded-lg border border-status-warning/30 bg-status-warning-soft px-3 py-2 text-xs text-status-warning">
                      <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                      <span>
                        Gerada sem o CNPJ da empresa. Preencha o CNPJ no cadastro e gere novamente — sem ele o download
                        é recusado.
                      </span>
                    </p>
                  )}

                  <div className="mt-4 flex items-center gap-4 text-xs font-medium text-muted-foreground">
                    <span className="flex items-center gap-1.5">
                      <Calendar className="w-3.5 h-3.5" /> Anual · entrega até o último dia útil de fevereiro
                    </span>
                  </div>
                </div>
                <div className="p-4 bg-muted/50 border-t border-border flex gap-2">
                  <Can permission="fiscal:generate">
                    <Button
                      onClick={handleGenerateDimob}
                      loading={generatingDimob}
                      loadingText="Gerando..."
                      className="flex-1"
                      variant={dimobReady ? "outline" : "default"}
                    >
                      <RefreshCw className="w-4 h-4" />
                      {dimobReady ? "Gerar novamente" : "Gerar DIMOB"}
                    </Button>
                  </Can>
                  <Button
                    onClick={handleDownloadDimob}
                    loading={downloadingDimob}
                    loadingText="Baixando..."
                    disabled={!dimobReady}
                    title={dimobReady ? "Baixar o arquivo de importação" : "Gere a DIMOB primeiro"}
                    className="flex-1"
                  >
                    <Download className="w-4 h-4" />
                    Baixar arquivo
                  </Button>
                </div>
              </div>

              {/* NFS-e */}
              <div className="bg-card rounded-2xl border border-border shadow-sm flex flex-col overflow-hidden hover:border-status-ok/40 dark:hover:border-status-ok transition-colors">
                <div className="p-6 flex-1 flex flex-col">
                  <div className="flex justify-between items-start mb-4">
                    <div className="flex items-center gap-3">
                      <div className="w-12 h-12 rounded-xl bg-status-ok-soft flex items-center justify-center text-status-ok">
                        <Receipt className="w-6 h-6" />
                      </div>
                      <div>
                        <h2 className="text-lg font-bold text-foreground dark:text-white">Nota Fiscal de Serviço</h2>
                        <p className="text-xs text-muted-foreground font-medium">NFS-e de administração</p>
                      </div>
                    </div>
                    <span className="px-2 py-1 bg-muted text-muted-foreground text-micro font-bold rounded uppercase tracking-wider">
                      Em breve
                    </span>
                  </div>
                  <p className="text-sm text-muted-foreground flex-1">
                    Emissão de nota fiscal dos serviços de administração de imóveis prestados aos proprietários,
                    conforme legislação municipal.
                  </p>
                  <div className="mt-6 flex items-center gap-4 text-xs font-medium text-muted-foreground">
                    <span className="flex items-center gap-1.5">
                      <Calendar className="w-3.5 h-3.5" /> Frequência: Mensal
                    </span>
                  </div>
                </div>
                <div className="p-4 bg-muted/50 border-t border-border">
                  {/* NFS-e não é emitida: não há integração com emissor municipal.
                      O registro interno não é nota autorizada, então a área não
                      oferece ação de emissão até a integração existir. */}
                  <div
                    role="note"
                    className="flex items-start gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2.5 text-xs text-muted-foreground"
                  >
                    <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" aria-hidden="true" />
                    <span>
                      Emissão indisponível nesta versão. Depende da integração com o emissor municipal de NFS-e.
                    </span>
                  </div>
                </div>
              </div>

              {/* Informes de rendimentos */}
              <div className="bg-card rounded-2xl border border-border shadow-sm flex flex-col overflow-hidden hover:border-status-warning/40 dark:hover:border-status-warning transition-colors">
                <div className="p-6 flex-1 flex flex-col">
                  <div className="flex justify-between items-start mb-4">
                    <div className="flex items-center gap-3">
                      <div className="w-12 h-12 rounded-xl bg-status-warning-soft flex items-center justify-center text-status-warning">
                        <DollarSign className="w-6 h-6" />
                      </div>
                      <div>
                        <h2 className="text-lg font-bold text-foreground dark:text-white">Informe de Rendimentos</h2>
                        <p className="text-xs text-muted-foreground font-medium">Para proprietários</p>
                      </div>
                    </div>
                    <span
                      className={clsx(
                        "px-2 py-1 text-micro font-bold rounded uppercase tracking-wider",
                        informes.length > 0 ? "bg-status-ok-soft text-status-ok" : "bg-muted text-muted-foreground",
                      )}
                    >
                      {informes.length > 0 ? `${informes.length} gerados` : "Não gerados"}
                    </span>
                  </div>
                  <p className="text-sm text-muted-foreground flex-1">
                    Comprovante anual de rendimentos recebidos para a Declaração de Imposto de Renda dos proprietários
                    locadores.
                  </p>
                  {informes.length > 0 && (
                    <div className="mt-4 text-xs font-medium text-muted-foreground">
                      Total líquido de {year}:{" "}
                      <span className="text-foreground">{formatCurrency(informesTotalNet)}</span>
                    </div>
                  )}
                  <div className="mt-4 flex items-center gap-4 text-xs font-medium text-muted-foreground">
                    <span className="flex items-center gap-1.5">
                      <Calendar className="w-3.5 h-3.5" /> Frequência: Anual
                    </span>
                  </div>
                </div>
                <div className="p-4 bg-muted/50 border-t border-border">
                  <Can permission="fiscal:generate">
                    <Button
                      variant="outline"
                      onClick={handleGenerateInformes}
                      loading={generatingInformes}
                      loadingText="Gerando..."
                      className="w-full border-status-warning/30 text-status-warning hover:bg-status-warning-soft hover:text-status-warning"
                    >
                      <Download className="w-4 h-4" />
                      {informes.length > 0 ? "Gerar novamente" : "Gerar Informes"}
                    </Button>
                  </Can>
                </div>
              </div>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
              <div className="bg-card rounded-2xl border border-border shadow-sm p-6">
                <div className="flex items-center gap-3 mb-6">
                  <Calendar className="w-5 h-5 text-primary-600 dark:text-primary-400" />
                  <div>
                    <h2 className="text-lg font-bold text-foreground dark:text-white">Calendário Fiscal</h2>
                    <p className="text-xs text-muted-foreground mt-0.5">Próximos prazos e vencimentos importantes</p>
                  </div>
                </div>

                {deadlines.length === 0 ? (
                  <p className="text-sm text-muted-foreground text-center py-8">
                    Nenhum prazo fiscal cadastrado para {year}.
                  </p>
                ) : (
                  <div className="space-y-4">
                    {deadlines.map((deadline) => {
                      const style = DEADLINE_STYLE[deadline.status];
                      return (
                        <div
                          key={deadline.id}
                          className={clsx("border rounded-xl p-4 flex items-center justify-between", style.bg)}
                        >
                          <div className="flex items-start gap-3">
                            {deadline.status === "DUE_SOON" || deadline.status === "OVERDUE" ? (
                              <AlertCircle className={clsx("w-5 h-5 mt-0.5", style.icon)} />
                            ) : deadline.type === "DIMOB" ? (
                              <FileCode className={clsx("w-5 h-5 mt-0.5", style.icon)} />
                            ) : (
                              <DollarSign className={clsx("w-5 h-5 mt-0.5", style.icon)} />
                            )}
                            <div>
                              <h3 className="text-sm font-bold text-foreground dark:text-white">{deadline.title}</h3>
                              <p className="text-xs text-muted-foreground mt-0.5">{deadline.description}</p>
                              <p className="text-xs font-medium text-foreground mt-1">
                                Prazo: {formatDate(deadline.due_date)}
                              </p>
                            </div>
                          </div>
                          <span
                            className={clsx(
                              "px-2.5 py-1 text-micro font-bold rounded uppercase tracking-wider",
                              style.badge,
                            )}
                          >
                            {style.badgeText}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>

              {/* Histórico de relatórios */}
              <div className="bg-card rounded-2xl border border-border shadow-sm p-6">
                <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-6">
                  <div className="flex items-center gap-3">
                    <History className="w-5 h-5 text-primary-600 dark:text-primary-400" />
                    <div>
                      <h2 className="text-lg font-bold text-foreground dark:text-white">Histórico de Relatórios</h2>
                      <p className="text-xs text-muted-foreground mt-0.5">
                        {reportTotal > 0 ? `${reportTotal} arquivo(s) em ${year}` : `Nenhum arquivo em ${year}`}
                      </p>
                    </div>
                  </div>
                  <label className="flex items-center gap-2 text-xs text-muted-foreground">
                    <span className="sr-only">Filtrar por tipo</span>
                    <div className="relative">
                      <select
                        aria-label="Filtrar por tipo"
                        value={historyType}
                        onChange={(event) => setHistoryType(event.target.value)}
                        className="appearance-none rounded-lg border border-border bg-card py-1.5 pl-2.5 pr-7 text-xs font-medium text-foreground"
                      >
                        {HISTORY_TYPE_FILTERS.map((option) => (
                          <option key={option.value} value={option.value}>
                            {option.label}
                          </option>
                        ))}
                      </select>
                      <ChevronDown className="pointer-events-none absolute right-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                    </div>
                  </label>
                </div>

                {reportHistory.length === 0 ? (
                  <div className="text-center py-8">
                    <p className="text-sm text-muted-foreground">
                      {historyType
                        ? `Nenhum relatório do tipo ${REPORT_TAG_LABELS[historyType] ?? historyType} em ${year}.`
                        : `Nenhum relatório gerado para ${year}.`}
                    </p>
                    {!historyType && (
                      <Can permission="fiscal:generate">
                        <Button variant="outline" size="sm" className="mt-4" onClick={handleGenerateDimob}>
                          <FileText className="w-4 h-4" />
                          Gerar DIMOB {year}
                        </Button>
                      </Can>
                    )}
                  </div>
                ) : (
                  <div className="space-y-3">
                    {reportHistory.map((item) => (
                      <div
                        key={item.id}
                        className="flex items-center justify-between gap-3 p-3 border border-border rounded-xl hover:bg-muted transition-colors group"
                      >
                        <div className="flex items-center gap-4 min-w-0">
                          <span
                            className={clsx(
                              "px-2 py-1 text-micro font-bold rounded-md border min-w-[56px] text-center",
                              REPORT_TAG_COLORS[item.type] || "bg-muted text-foreground border-border",
                            )}
                          >
                            {REPORT_TAG_LABELS[item.type] || item.type}
                          </span>
                          <div className="min-w-0">
                            <h4 className="text-sm font-bold text-foreground dark:text-white truncate">{item.name}</h4>
                            <p className="text-xs text-muted-foreground">{formatDateTime(item.generated_at)}</p>
                          </div>
                        </div>
                        <div className="flex items-center gap-1 text-muted-foreground shrink-0">
                          {item.status === "READY" ? (
                            <CheckCircle2 className="w-4 h-4 text-status-ok" aria-label="Pronto" />
                          ) : (
                            <AlertCircle className="w-4 h-4 text-status-critical" aria-label="Com erro" />
                          )}
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => void handleDownloadReport(item)}
                            disabled={!item.file_url}
                            className="size-9 hover:bg-primary-50 hover:text-primary-600 disabled:opacity-40"
                            title={item.file_url ? "Baixar arquivo" : "Arquivo ainda não gerado"}
                            aria-label={`Baixar ${item.name}`}
                          >
                            <Download className="w-4 h-4" />
                          </Button>
                          <Can permission="fiscal:generate">
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => setReportToArchive(item)}
                              className="size-9 hover:bg-status-critical-soft hover:text-status-critical"
                              title="Arquivar (sai da listagem; a linha fica para auditoria)"
                              aria-label={`Arquivar ${item.name}`}
                            >
                              <Trash2 className="w-4 h-4" />
                            </Button>
                          </Can>
                        </div>
                      </div>
                    ))}

                    {hasMoreReports && (
                      <Button
                        variant="outline"
                        className="w-full"
                        onClick={handleLoadMoreReports}
                        loading={loadingMoreReports}
                        loadingText="Carregando..."
                      >
                        Carregar mais ({reportTotal - reportHistory.length} restantes)
                      </Button>
                    )}
                  </div>
                )}
              </div>
            </div>

            {/* Informes de rendimentos do ano */}
            <div className="bg-card rounded-2xl border border-border shadow-sm p-6">
              <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-6">
                <div className="flex items-center gap-3">
                  <Users className="w-5 h-5 text-primary-600 dark:text-primary-400" />
                  <div>
                    <h2 className="text-lg font-bold text-foreground dark:text-white">
                      Informes de Rendimentos — {year}
                    </h2>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      Um informe por proprietário, com o total bruto, a taxa de administração e o valor líquido do ano
                    </p>
                  </div>
                </div>
                {informes.length > 0 && (
                  <Can permission="fiscal:generate">
                    <Button variant="outline" size="sm" onClick={handleGenerateInformes} loading={generatingInformes}>
                      <RefreshCw className="w-4 h-4" />
                      Regravar informes
                    </Button>
                  </Can>
                )}
              </div>

              {informes.some((informe) => !hasFlowBreakdown(informe)) && (
                <div className="mb-4 flex flex-col gap-2 rounded-xl border border-status-warning/30 bg-status-warning-soft px-4 py-3 text-xs text-status-warning sm:flex-row sm:items-center sm:justify-between">
                  <span className="flex items-start gap-2">
                    <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                    <span>
                      Há informes gerados antes da separação por locação e venda. Regrave para ver os valores de cada
                      fluxo e a comissão de intermediação de venda.
                    </span>
                  </span>
                  <Can permission="fiscal:generate">
                    <Button
                      variant="outline"
                      size="sm"
                      className="shrink-0"
                      onClick={handleGenerateInformes}
                      loading={generatingInformes}
                    >
                      Regravar informes de {year}
                    </Button>
                  </Can>
                </div>
              )}

              {informes.length === 0 ? (
                <div className="text-center py-8">
                  <p className="text-sm text-muted-foreground">Nenhum informe de rendimentos gerado para {year}.</p>
                  <Can permission="fiscal:generate">
                    <Button
                      variant="outline"
                      size="sm"
                      className="mt-4"
                      onClick={handleGenerateInformes}
                      loading={generatingInformes}
                    >
                      <DollarSign className="w-4 h-4" />
                      Gerar informes de {year}
                    </Button>
                  </Can>
                </div>
              ) : (
                <div className="overflow-x-auto">
                  <ResponsiveTable
                    data={informes}
                    caption="Informes de rendimentos por proprietário"
                    getRowKey={(informe) => informe.id}
                    columns={[
                      { key: "owner", header: "Proprietário", primary: true, cell: (informe) => informe.owner_name || "—" },
                      {
                        key: "rental",
                        header: "Locação",
                        className: "text-right",
                        cell: (informe) => hasFlowBreakdown(informe) ? formatCurrency(informe.rental_gross ?? 0) : <span className="text-muted-foreground">—</span>,
                      },
                      {
                        key: "admin",
                        header: "Taxa adm.",
                        className: "text-right",
                        cell: (informe) => hasFlowBreakdown(informe) ? <span className="text-muted-foreground">{formatCurrency(informe.rental_admin_fee ?? 0)}</span> : <span className="text-muted-foreground">—</span>,
                      },
                      {
                        key: "sale",
                        header: "Vendas",
                        className: "text-right",
                        hideOnMobile: true,
                        cell: (informe) => hasFlowBreakdown(informe) ? <span title={`${informe.sale_count ?? 0} venda(s) no ano`}>{formatCurrency(informe.sale_gross ?? 0)}</span> : <span className="text-muted-foreground">—</span>,
                      },
                      {
                        key: "intermediation",
                        header: "Intermediação",
                        className: "text-right",
                        cell: (informe) => hasFlowBreakdown(informe) ? (
                          <span title={`Locação: ${formatCurrency(informe.rental_intermediation ?? 0)} · Venda: ${formatCurrency(informe.sale_intermediation ?? 0)}`}>
                            {formatCurrency((informe.rental_intermediation ?? 0) + (informe.sale_intermediation ?? 0))}
                          </span>
                        ) : <span className="text-muted-foreground">{formatCurrency(informe.total_intermediation ?? 0)}</span>,
                      },
                      { key: "net", header: "Líquido", className: "text-right", cell: (informe) => <span className="font-bold">{formatCurrency(informe.total_net ?? 0)}</span> },
                      {
                        key: "status",
                        header: "Situação",
                        cell: (informe) => (
                          <span className={clsx(
                            "px-2 py-0.5 text-micro font-bold rounded uppercase tracking-wider",
                            informe.status === "READY" ? "bg-status-ok-soft text-status-ok" : informe.status === "ERROR" ? "bg-status-critical-soft text-status-critical" : "bg-muted text-muted-foreground",
                          )}>
                            {informe.status === "READY" ? "Pronto" : informe.status === "ERROR" ? "Erro" : "Processando"}
                          </span>
                        ),
                      },
                      {
                        key: "download",
                        header: "Informe",
                        className: "text-right",
                        cell: (informe) => (
                          <Button variant="ghost" size="sm" onClick={() => void handleDownloadInforme(informe)} aria-label={`Abrir informe de ${informe.owner_name || informe.owner_id}`}>
                            <Download className="w-4 h-4" /> PDF
                          </Button>
                        ),
                      },
                    ]}
                  />
                </div>
              )}
            </div>
          </>
        )}
      </div>

      <ConfirmDialog
        open={reportToArchive !== null}
        title="Arquivar relatório?"
        description={
          reportToArchive
            ? `"${reportToArchive.name}" sai da listagem do histórico. O arquivo deixa de aparecer, mas o registro permanece para auditoria.`
            : ""
        }
        confirmLabel="Arquivar"
        variant="warning"
        onConfirm={handleArchiveReport}
        onCancel={() => setReportToArchive(null)}
      />
    </div>
  );
}

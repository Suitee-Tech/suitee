import { useState, useMemo } from "react";
import {
  X,
  Download,
  FileSpreadsheet,
  FileText,
  Table2,
  CheckCircle2,
  Calendar,
  Building2,
  Eye,
  ChevronLeft,
  ChevronRight,
  CalendarRange,
  RotateCcw,
} from "lucide-react";
import { clsx } from "clsx";
import { downloadCsv } from "../utils/csvExport";
import { printHtml } from "../utils/printDocument";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "../ui/dialog";
import { Button } from "../ui/button";

export interface ExportColumn {
  key: string;
  label: string;
  align?: "left" | "center" | "right";
  format?: (value: any) => string;
}

export interface ExportConfig {
  title: string;
  subtitle?: string;
  columns: ExportColumn[];
  data: Record<string, any>[];
  summaryRows?: { label: string; value: string; color?: string }[];
  filterDescription?: string;
  dateKey?: string;
  recalcSummary?: (filteredData: Record<string, any>[]) => { label: string; value: string; color?: string }[];
}

interface ExportReportModalProps {
  config: ExportConfig;
  onClose: () => void;
}

type ExportFormat = "csv" | "pdf";

const PREVIEW_PAGE_SIZE = 8;

function formatCellValue(col: ExportColumn, row: Record<string, any>): string {
  const raw = row[col.key];
  if (col.format) return col.format(raw);
  if (raw == null) return "";
  return String(raw);
}

function generateCSV(config: ExportConfig): string {
  const BOM = "\uFEFF";
  const sep = ";";
  const lines: string[] = [];

  lines.push(config.columns.map((c) => `"${c.label}"`).join(sep));

  config.data.forEach((row) => {
    const cells = config.columns.map((col) => {
      const val = formatCellValue(col, row).replace(/"/g, '""');
      return `"${val}"`;
    });
    lines.push(cells.join(sep));
  });

  if (config.summaryRows && config.summaryRows.length > 0) {
    lines.push("");
    config.summaryRows.forEach((sr) => {
      lines.push(`"${sr.label}"${sep}"${sr.value}"`);
    });
  }

  return BOM + lines.join("\r\n");
}

function generatePDFHTML(config: ExportConfig): string {
  const now = new Date();
  const dateStr = now.toLocaleDateString("pt-BR", { day: "2-digit", month: "long", year: "numeric" });
  const timeStr = now.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" });

  const summaryHTML =
    config.summaryRows && config.summaryRows.length > 0
      ? `<div style="margin-top:24px;display:flex;gap:16px;flex-wrap:wrap;">
${config.summaryRows
  .map(
    (sr) => `
<div style="flex:1;min-width:140px;background:#f8fafc;border:1px solid #e2e8f0;border-radius:8px;padding:12px 16px;">
<div style="font-size:11px;color:#64748b;margin-bottom:4px;">${sr.label}</div>
<div style="font-size:18px;font-weight:700;color:${sr.color || "#0f172a"};">${sr.value}</div>
</div>
`,
  )
  .join("")}
</div>`
      : "";

  const tableRows = config.data
    .map(
      (row, i) => `
<tr style="background:${i % 2 === 0 ? "#fff" : "#f8fafc"};">
${config.columns
  .map(
    (col) => `
<td style="padding:8px 12px;font-size:12px;color:#334155;text-align:${col.align || "left"};border-bottom:1px solid #f1f5f9;">
${formatCellValue(col, row)}
</td>
`,
  )
  .join("")}
</tr>
`,
    )
    .join("");

  return `<!DOCTYPE html>
<html lang="pt-BR">
<head>
<meta charset="UTF-8">
<title>${config.title}</title>
<style>
@page { margin: 20mm 15mm; size: A4 landscape; }
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: 'Segoe UI', system-ui, -apple-system, sans-serif; color: #0f172a; padding: 0; }
@media print {
body { -webkit-print-color-adjust: exact; print-color-adjust: exact; }
.no-print { display: none !important; }
}
</style>
</head>
<body>
<div style="max-width:1100px;margin:0 auto;padding:32px 0;">
<div style="display:flex;justify-content:space-between;align-items:flex-start;margin-bottom:24px;padding-bottom:16px;border-bottom:2px solid #4f46e5;">
<div>
<div style="font-size:10px;font-weight:700;color:#4f46e5;letter-spacing:1.5px;text-transform:uppercase;margin-bottom:4px;">suitee Imobiliario</div>
<h1 style="font-size:22px;font-weight:800;color:#0f172a;margin-bottom:4px;">${config.title}</h1>
${config.subtitle ? `<p style="font-size:12px;color:#64748b;">${config.subtitle}</p>` : ""}
${config.filterDescription ? `<p style="font-size:11px;color:#94a3b8;margin-top:4px;">Filtros: ${config.filterDescription}</p>` : ""}
</div>
<div style="text-align:right;">
<div style="font-size:11px;color:#64748b;">Gerado em</div>
<div style="font-size:13px;font-weight:600;color:#334155;">${dateStr}</div>
<div style="font-size:11px;color:#94a3b8;">${timeStr}</div>
</div>
</div>
${summaryHTML}
<div style="margin-top:24px;border:1px solid #e2e8f0;border-radius:8px;overflow:hidden;">
<table style="width:100%;border-collapse:collapse;">
<thead>
<tr style="background:#f1f5f9;">
${config.columns
  .map(
    (col) => `
<th style="padding:10px 12px;font-size:10px;font-weight:700;color:#64748b;text-transform:uppercase;letter-spacing:0.5px;text-align:${col.align || "left"};border-bottom:2px solid #e2e8f0;">
${col.label}
</th>
`,
  )
  .join("")}
</tr>
</thead>
<tbody>
${tableRows}
</tbody>
</table>
</div>
<div style="margin-top:24px;padding-top:12px;border-top:1px solid #e2e8f0;display:flex;justify-content:space-between;align-items:center;">
<div style="font-size:10px;color:#94a3b8;">Total de registros: ${config.data.length}</div>
<div style="font-size:10px;color:#94a3b8;">Documento gerado automaticamente pelo suitee - ${dateStr}</div>
</div>
<div class="no-print" style="margin-top:32px;text-align:center;">
<button onclick="window.print()" style="padding:12px 32px;background:#4f46e5;color:white;border:none;border-radius:8px;font-size:14px;font-weight:700;cursor:pointer;">
Imprimir / Salvar como PDF
</button>
</div>
</div>
</body>
</html>`;
}

export function ExportReportModal({ config, onClose }: ExportReportModalProps) {
  const [format, setFormat] = useState<ExportFormat>("csv");
  const [exporting, setExporting] = useState(false);
  const [exported, setExported] = useState(false);
  const [previewPage, setPreviewPage] = useState(0);
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");

  const hasDateFilter = !!config.dateKey;
  const isDateRangeActive = hasDateFilter && (dateFrom || dateTo);

  const effectiveData = useMemo(() => {
    if (!config.dateKey || (!dateFrom && !dateTo)) return config.data;
    return config.data.filter((row) => {
      const dateVal = row[config.dateKey!];
      if (!dateVal) return true;
      const d = String(dateVal);
      if (dateFrom && d < dateFrom) return false;
      if (dateTo && d > dateTo) return false;
      return true;
    });
  }, [config.data, config.dateKey, dateFrom, dateTo]);

  const effectiveSummary = useMemo(() => {
    if (!isDateRangeActive || !config.recalcSummary) return config.summaryRows;
    return config.recalcSummary(effectiveData);
  }, [effectiveData, isDateRangeActive, config]);

  const effectiveConfig = useMemo((): ExportConfig => {
    const dateDesc = isDateRangeActive
      ? `Período: ${dateFrom ? new Date(dateFrom + "T12:00:00").toLocaleDateString("pt-BR") : "início"} a ${dateTo ? new Date(dateTo + "T12:00:00").toLocaleDateString("pt-BR") : "hoje"}`
      : null;
    const fullFilterDesc = [config.filterDescription, dateDesc].filter(Boolean).join(" | ") || undefined;

    return { ...config, data: effectiveData, summaryRows: effectiveSummary, filterDescription: fullFilterDesc };
  }, [config, effectiveData, effectiveSummary, isDateRangeActive, dateFrom, dateTo]);

  const totalPages = Math.ceil(effectiveData.length / PREVIEW_PAGE_SIZE);
  const previewData = effectiveData.slice(previewPage * PREVIEW_PAGE_SIZE, (previewPage + 1) * PREVIEW_PAGE_SIZE);

  const handleClearDateRange = () => {
    setDateFrom("");
    setDateTo("");
    setPreviewPage(0);
  };

  const handleExport = () => {
    setExporting(true);

    const timestamp = new Date().toISOString().split("T")[0];
    const slug = effectiveConfig.title.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/-+$/, "");
    const finish = () => {
      setExporting(false);
      setExported(true);
      setTimeout(() => setExported(false), 2500);
    };

    if (format === "csv") {
      // downloadCsv injeta o BOM UTF-8: sem ele o Excel abre os acentos como
      // ANSI e "Comissão" vira "ComissÃ£o" no arquivo exportado.
      downloadCsv(`${slug}-${timestamp}.csv`, generateCSV(effectiveConfig));
      finish();
      return;
    }

    // window.open precisa acontecer no MESMO gesto do clique: dentro do
    // setTimeout anterior o navegador bloqueava como popup e a exportação
    // "não fazia nada".
    const html = generatePDFHTML(effectiveConfig);
    const win = window.open("", "_blank");
    if (win) {
      win.document.write(html);
      win.document.close();
    } else {
      // Popup bloqueado: imprime o relatório num iframe oculto; o usuário
      // escolhe "Salvar como PDF" na caixa nativa.
      printHtml(html);
    }
    finish();
  };

  return (
    <Dialog open onOpenChange={(next) => { if (!next) onClose(); }}>
      <DialogContent className="flex max-h-[90vh] flex-col gap-0 overflow-hidden p-0 sm:max-w-3xl">
        <div className="px-6 py-4 border-b border-border flex items-center gap-3 flex-shrink-0 pr-12">
          <div className="p-2.5 bg-primary-50 rounded-xl">
            <Download className="w-5 h-5 text-primary-600" />
          </div>
          <div>
            <DialogTitle className="text-lg font-bold text-foreground">Exportar Relatório</DialogTitle>
            <DialogDescription className="text-xs">{config.data.length} registros disponíveis para exportação</DialogDescription>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto p-6 space-y-6">
          <div className="bg-muted rounded-xl p-4 border border-border flex flex-col sm:flex-row sm:items-center gap-4">
            <div className="flex items-center gap-3 flex-1">
              <Building2 className="w-5 h-5 text-muted-foreground" />
              <div>
                <p className="text-sm font-bold text-foreground">{config.title}</p>
                {config.subtitle && <p className="text-xs text-muted-foreground">{config.subtitle}</p>}
              </div>
            </div>
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Calendar className="w-3.5 h-3.5" />
              {new Date().toLocaleDateString("pt-BR", { day: "2-digit", month: "short", year: "numeric" })}
            </div>
            {(config.filterDescription || isDateRangeActive) && (
              <span className="inline-flex items-center px-2.5 py-1 text-caption font-bold bg-status-warning-soft text-status-warning border border-status-warning/30 rounded-lg">
                Filtros ativos
              </span>
            )}
          </div>

          {hasDateFilter && (
            <div>
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-2">
                  <CalendarRange className="w-4 h-4 text-muted-foreground" />
                  <label className="text-sm font-bold text-foreground">Período de exportação</label>
                </div>
                {isDateRangeActive && (
                  <button
                    onClick={handleClearDateRange}
                    className="flex items-center gap-1 text-xs font-medium text-muted-foreground hover:text-primary-600 transition-colors px-2 py-1 rounded-lg hover:bg-primary-50"
                  >
                    <RotateCcw className="w-3 h-3" /> Limpar periodo
                  </button>
                )}
              </div>
              <div className="flex flex-col sm:flex-row gap-3">
                <div className="flex-1">
                  <label className="block text-caption font-bold text-muted-foreground uppercase tracking-wider mb-1">Data inicial</label>
                  <input
                    type="date"
                    value={dateFrom}
                    onChange={(e) => { setDateFrom(e.target.value); setPreviewPage(0); }}
                    max={dateTo || undefined}
                    className="w-full px-3 py-2 border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 bg-card"
                  />
                </div>
                <div className="flex items-end pb-2 text-muted-foreground text-xs font-medium">ate</div>
                <div className="flex-1">
                  <label className="block text-caption font-bold text-muted-foreground uppercase tracking-wider mb-1">Data final</label>
                  <input
                    type="date"
                    value={dateTo}
                    onChange={(e) => { setDateTo(e.target.value); setPreviewPage(0); }}
                    min={dateFrom || undefined}
                    className="w-full px-3 py-2 border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 bg-card"
                  />
                </div>
              </div>
              {isDateRangeActive && (
                <div className="mt-2 flex items-center gap-2">
                  <span className="inline-flex items-center gap-1 px-2.5 py-1 text-caption font-bold bg-primary-50 text-primary-700 border border-primary-200 rounded-lg">
                    <CalendarRange className="w-3 h-3" />
                    {effectiveData.length} de {config.data.length} registros no periodo
                  </span>
                  {effectiveData.length === 0 && <span className="text-caption text-status-critical font-medium">Nenhum registro encontrado neste periodo</span>}
                </div>
              )}
            </div>
          )}

          {effectiveSummary && effectiveSummary.length > 0 && (
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
              {effectiveSummary.map((sr, idx) => (
                <div key={idx} className="bg-card p-3 rounded-xl border border-border shadow-sm">
                  <p className="text-caption font-bold text-muted-foreground uppercase tracking-wider mb-1">{sr.label}</p>
                  <p className="text-sm font-bold" style={{ color: sr.color || "#0f172a" }}>
                    {sr.value}
                  </p>
                </div>
              ))}
            </div>
          )}

          <div>
            <label className="block text-sm font-bold text-foreground mb-2">Formato de Exportação</label>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <button
                onClick={() => setFormat("csv")}
                className={clsx(
                  "flex items-center gap-3 p-4 rounded-xl border-2 transition-all text-left",
                  format === "csv" ? "border-primary-600 bg-primary-50/50 ring-2 ring-primary-600/20" : "border-border bg-card hover:border-border",
                )}
              >
                <div className={clsx("p-2 rounded-lg", format === "csv" ? "bg-primary-100" : "bg-muted")}>
                  <FileSpreadsheet className={clsx("w-5 h-5", format === "csv" ? "text-primary-600" : "text-muted-foreground")} />
                </div>
                <div>
                  <p className={clsx("text-sm font-bold", format === "csv" ? "text-primary-700" : "text-foreground")}>CSV (Excel)</p>
                  <p className="text-caption text-muted-foreground">Planilha editavel, compativel com Excel e Google Sheets</p>
                </div>
              </button>
              <button
                onClick={() => setFormat("pdf")}
                className={clsx(
                  "flex items-center gap-3 p-4 rounded-xl border-2 transition-all text-left",
                  format === "pdf" ? "border-primary-600 bg-primary-50/50 ring-2 ring-primary-600/20" : "border-border bg-card hover:border-border",
                )}
              >
                <div className={clsx("p-2 rounded-lg", format === "pdf" ? "bg-primary-100" : "bg-muted")}>
                  <FileText className={clsx("w-5 h-5", format === "pdf" ? "text-primary-600" : "text-muted-foreground")} />
                </div>
                <div>
                  <p className={clsx("text-sm font-bold", format === "pdf" ? "text-primary-700" : "text-foreground")}>PDF (Impressão)</p>
                  <p className="text-caption text-muted-foreground">Documento formatado com cabeçalho e layout profissional</p>
                </div>
              </button>
            </div>
          </div>

          <div>
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2">
                <Eye className="w-4 h-4 text-muted-foreground" />
                <label className="text-sm font-bold text-foreground">Pré-visualização dos dados</label>
              </div>
              {totalPages > 1 && (
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => setPreviewPage((p) => Math.max(0, p - 1))}
                    disabled={previewPage === 0}
                    className="p-1 text-muted-foreground hover:text-muted-foreground disabled:opacity-30 rounded transition-colors"
                  >
                    <ChevronLeft className="w-4 h-4" />
                  </button>
                  <span className="text-xs text-muted-foreground">{previewPage + 1} / {totalPages}</span>
                  <button
                    onClick={() => setPreviewPage((p) => Math.min(totalPages - 1, p + 1))}
                    disabled={previewPage >= totalPages - 1}
                    className="p-1 text-muted-foreground hover:text-muted-foreground disabled:opacity-30 rounded transition-colors"
                  >
                    <ChevronRight className="w-4 h-4" />
                  </button>
                </div>
              )}
            </div>
            <div className="border border-border rounded-xl overflow-hidden">
              <div className="overflow-x-auto">
                <table className="w-full text-left border-collapse">
                  <thead>
                    <tr className="bg-muted border-b border-border">
                      {config.columns.map((col) => (
                        <th
                          key={col.key}
                          className={clsx(
                            "px-3 py-2.5 text-caption font-bold text-muted-foreground uppercase tracking-wider whitespace-nowrap",
                            col.align === "right" && "text-right",
                            col.align === "center" && "text-center",
                          )}
                        >
                          {col.label}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border">
                    {previewData.map((row, rowIdx) => (
                      <tr key={rowIdx} className="hover:bg-muted/50 transition-colors">
                        {config.columns.map((col) => (
                          <td
                            key={col.key}
                            className={clsx(
                              "px-3 py-2 text-xs text-foreground whitespace-nowrap",
                              col.align === "right" && "text-right",
                              col.align === "center" && "text-center",
                            )}
                          >
                            {formatCellValue(col, row)}
                          </td>
                        ))}
                      </tr>
                    ))}
                    {previewData.length === 0 && (
                      <tr>
                        <td colSpan={config.columns.length} className="px-3 py-6 text-center text-xs text-muted-foreground">
                          Nenhum dado para exportar
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
              {effectiveData.length > PREVIEW_PAGE_SIZE && (
                <div className="px-3 py-2 bg-muted border-t border-border text-caption text-muted-foreground flex items-center gap-1">
                  <Table2 className="w-3 h-3" />
                  Mostrando {previewData.length} de {effectiveData.length} registros - todos serão incluidos na exportação
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="px-6 py-4 border-t border-border bg-muted/80 flex items-center justify-between flex-shrink-0">
          <div className="text-xs text-muted-foreground">
            {format === "csv"
              ? "Arquivo .csv separado por ponto e virgula (;), compativel com Excel"
              : "Abrira uma nova aba com o relatório formatado para impressão/PDF"}
          </div>
          <div className="flex items-center gap-3">
            <Button variant="outline" onClick={onClose}>Cancelar</Button>
            <Button
              variant={exported ? "success" : "default"}
              onClick={handleExport}
              disabled={effectiveData.length === 0}
              loading={exporting}
              loadingText="Gerando..."
            >
              {exported ? (
                <>
                  <CheckCircle2 className="w-4 h-4" /> Exportado!
                </>
              ) : (
                <>
                  <Download className="w-4 h-4" /> Exportar {format.toUpperCase()}
                </>
              )}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

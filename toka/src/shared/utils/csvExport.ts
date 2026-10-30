/**
 * Exportação de CSV com encoding correto.
 *
 * O Excel para Windows abre CSV sem BOM como ANSI/Latin-1 e transforma
 * acentos em mojibake ("Não" → "NÃ£o", "Comissão" → "ComissÃ£o"). Declarar
 * `charset=utf-8` no MIME não resolve — quem decide é o BOM no início do
 * arquivo. Todo export CSV do app deve passar por aqui.
 */

/** Marca de ordem de bytes (U+FEFF) exigida pelo Excel. */
export const UTF8_BOM = "\uFEFF";

/** Prefixa o BOM uma única vez (idempotente). */
export function withUtf8Bom(content: string): string {
  return content.startsWith(UTF8_BOM) ? content : UTF8_BOM + content;
}

/** Dispara o download de um CSV já com o BOM de UTF-8. */
export function downloadCsv(filename: string, content: string): void {
  const blob = new Blob([withUtf8Bom(content)], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

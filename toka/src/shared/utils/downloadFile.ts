import { toast } from "sonner";

/**
 * Salva um arquivo já em memória (ex.: o .ics devolvido pela API).
 *
 * O mesmo esqueleto de âncora usado no CSV: cria o object URL, clica e revoga.
 */
export function saveBlobFile(filename: string, blob: Blob): void {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

/**
 * Baixa os bytes de um compromisso em `.ics` e avisa quando não dá.
 *
 * Falha aqui é recuperável: o compromisso continua na tela e o usuário pode
 * tentar de novo — por isso o erro vira aviso, não exceção solta na interface.
 */
export async function downloadBlobAsFile(
  filename: string,
  load: () => Promise<Blob>,
  failureMessage = "Não foi possível baixar o arquivo. Tente novamente.",
): Promise<boolean> {
  try {
    saveBlobFile(filename, await load());
    return true;
  } catch {
    toast.error(failureMessage);
    return false;
  }
}

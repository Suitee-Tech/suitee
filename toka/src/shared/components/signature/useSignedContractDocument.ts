import { useCallback, useRef, useState } from "react";
import { toast } from "sonner";

import { contractService } from "../../services/contract.service";
import type { Contract } from "../../contracts/types";

type Busy = "open" | "download" | null;

/**
 * Acesso ao PDF assinado do contrato.
 *
 * O `signed_document_url` que vem no contrato/envelope é um link PRÉ-ASSINADO do
 * storage do provedor de assinatura, com validade de ~5 minutos. Guardá-lo e
 * reusá-lo depois sempre falha: o navegador recebe o XML do S3 com
 * `<Code>AccessDenied</Code><Message>Request has expired</Message>` — foi o que
 * acontecia no "Baixar PDF assinado" da aba Assinatura, que era um `<a href>`
 * apontando para esse campo.
 *
 * Aqui toda ação pede uma URL NOVA a `GET /contracts/:id/signed-file`, que
 * garante a cópia durável no nosso storage (baixando do provedor quando ainda
 * não existe) e devolve um link recém-assinado. Nenhuma URL é guardada em
 * estado: um link velho na memória é o mesmo defeito, só mais difícil de ver.
 */
/**
 * Baixa uma URL pré-assinada como arquivo. O blob evita deixar o usuário numa
 * aba com o XML de erro do storage quando o link expira no meio do caminho.
 */
export async function downloadUrlAsFile(url: string, filename: string): Promise<boolean> {
  try {
    const fetched = await fetch(url, { credentials: "omit" });
    if (!fetched.ok) throw new Error(`status ${fetched.status}`);
    const blob = await fetched.blob();
    const objectUrl = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = objectUrl;
    a.download = filename.toLowerCase().endsWith(".pdf") ? filename : `${filename}.pdf`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(objectUrl);
    return true;
  } catch {
    return false;
  }
}

/**
 * @param contractId contrato do painel (API do operador).
 * @param getUrl     resolvedor alternativo da URL fresca — o PORTAL usa o
 *                   próprio serviço (`portalService.getDealSignature`), que já
 *                   devolve um link recém-assinado a cada consulta.
 */
export function useSignedContractDocument(
  contractId?: string | null,
  getUrl?: () => Promise<string | null>,
) {
  const [busy, setBusy] = useState<Busy>(null);
  // Um clique por vez: dois downloads simultâneos pediriam duas URLs e
  // disparariam dois `EnsureSignedPDFURL` (que baixa o PDF do provedor).
  const inFlight = useRef(false);

  const fetchFreshUrl = useCallback(
    async (action: Exclude<Busy, null>): Promise<string | null> => {
      if (inFlight.current) return null;
      if (!getUrl && !contractId) return null;
      inFlight.current = true;
      setBusy(action);
      try {
        const url = getUrl ? await getUrl() : (await contractService.getSignedFileUrl(contractId as string))?.url;
        if (!url) {
          toast.error("O PDF assinado ainda não está disponível. Tente novamente em alguns segundos.");
          return null;
        }
        return url;
      } catch {
        toast.error(
          action === "download"
            ? "Não foi possível baixar o documento assinado."
            : "Não foi possível abrir o documento assinado.",
        );
        return null;
      } finally {
        inFlight.current = false;
        setBusy(null);
      }
    },
    [contractId, getUrl],
  );

  /** URL pronta para o visualizador (o chamador decide onde abrir). */
  const open = useCallback(async (): Promise<string | null> => fetchFreshUrl("open"), [fetchFreshUrl]);

  /**
   * Baixa o PDF com o nome do arquivo do contrato. O download passa pelo blob
   * porque a URL expira: navegar direto para ela deixaria o usuário numa aba com
   * o XML de erro do storage se a assinatura demorasse.
   */
  const download = useCallback(
    async (filename: string): Promise<boolean> => {
      const url = await fetchFreshUrl("download");
      if (!url) return false;
      const ok = await downloadUrlAsFile(url, filename);
      if (!ok) toast.error("Não foi possível baixar o documento assinado.");
      return ok;
    },
    [fetchFreshUrl],
  );

  return { busy, open, download };
}

/** Nome do arquivo baixado — o mesmo rótulo que a lista de documentos assinados usa. */
export function signedContractFilename(contract: Pick<Contract, "id" | "display_id">): string {
  return `Contrato ${contract.display_id ?? contract.id}.pdf`;
}

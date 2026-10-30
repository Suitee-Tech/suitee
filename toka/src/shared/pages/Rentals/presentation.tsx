import { useEffect, useRef, useState, type ReactElement, type ReactNode } from "react";
import { Banknote, BarChart3, Droplet, Eye, FileText, Receipt } from "lucide-react";

export function RC({ height, children }: { height: number; children: ReactElement<Record<string, unknown>> }) {
 const ref = useRef<HTMLDivElement>(null);
 const [dims, setDims] = useState<{ w: number; h: number } | null>(null);

 useEffect(() => {
  const el = ref.current;
  if (!el) return;

  const measure = () => {
   const { width: w, height: h } = el.getBoundingClientRect();
   if (w > 0 && h > 0) {
    setDims((prev) =>
     prev && prev.w === Math.round(w) && prev.h === Math.round(h)
      ? prev
      : { w: Math.round(w), h: Math.round(h) },
    );
   }
  };

  const raf = requestAnimationFrame(measure);
  const observer = new ResizeObserver(measure);
  observer.observe(el);

  return () => {
   cancelAnimationFrame(raf);
   observer.disconnect();
  };
 }, []);

 const chart =
  dims && dims.w > 0 && dims.h > 0
   ? (() => {
      const { type: Chart, props } = children;
      return <Chart {...props} width={dims.w} height={dims.h} />;
     })()
   : null;

 return (
  <div ref={ref} style={{ width: "100%", height, minHeight: 1, minWidth: 1, position: "relative" }}>
   {chart}
  </div>
 );
}

export type ViewState = "list" | "detail";
export type ModalState = "none" | "cobrancas" | "reajuste" | "novaCobranca" | "vincularConta";
export type DetailTab = "geral" | "financeiro" | "contas" | "cobrancas" | "repasses" | "documentos" | "vistorias";

export const DETAIL_TABS: { id: DetailTab; label: string; icon: ReactNode }[] = [
 { id: "geral", label: "Visão Geral", icon: <Eye className="w-4 h-4" /> },
 { id: "financeiro", label: "Financeiro", icon: <BarChart3 className="w-4 h-4" /> },
 { id: "contas", label: "Contas", icon: <Droplet className="w-4 h-4" /> },
 { id: "cobrancas", label: "Cobranças", icon: <Receipt className="w-4 h-4" /> },
 { id: "repasses", label: "Repasses", icon: <Banknote className="w-4 h-4" /> },
 { id: "vistorias", label: "Vistorias", icon: <Eye className="w-4 h-4" /> },
 { id: "documentos", label: "Documentos", icon: <FileText className="w-4 h-4" /> },
];

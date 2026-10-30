import React, { useEffect, useRef, useState } from "react";
import { Eraser, Loader2, PenTool } from "lucide-react";
import { Button } from "../ui/button";

// Assinatura manuscrita (canvas) e formatação de tamanho — extraídos de
// DocumentsTab.tsx (P1-04/T-07) para manter o painel de documentos focado
// no fluxo de upload/assinatura.

/** Formata bytes em unidade legível (B/KB/MB). */
export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1048576).toFixed(1)} MB`;
}

export function SignaturePad({
 onSave,
 onCancel,
 isSaving = false,
}: {
 onSave: (dataUrl: string) => void;
 onCancel: () => void;
 isSaving?: boolean;
}) {
 const canvasRef = useRef<HTMLCanvasElement>(null);
 const [isDrawing, setIsDrawing] = useState(false);
 const [hasContent, setHasContent] = useState(false);

 useEffect(() => {
 const canvas = canvasRef.current;
 if (!canvas) return;
 const ctx = canvas.getContext("2d");
 if (!ctx) return;

 const rect = canvas.getBoundingClientRect();
 const dpr = window.devicePixelRatio || 1;
 canvas.width = rect.width * dpr;
 canvas.height = rect.height * dpr;
 ctx.scale(dpr, dpr);

 ctx.fillStyle = "#ffffff";
 ctx.fillRect(0, 0, rect.width, rect.height);
 ctx.strokeStyle = "#1e293b";
 ctx.lineWidth = 2.5;
 ctx.lineCap = "round";
 ctx.lineJoin = "round";

 // Dashed guideline
 ctx.setLineDash([4, 4]);
 ctx.strokeStyle = "#cbd5e1";
 ctx.lineWidth = 1;
 ctx.beginPath();
 ctx.moveTo(20, rect.height * 0.7);
 ctx.lineTo(rect.width - 20, rect.height * 0.7);
 ctx.stroke();
 ctx.setLineDash([]);
 ctx.strokeStyle = "#1e293b";
 ctx.lineWidth = 2.5;
 }, []);

 const getPos = (
 e: React.MouseEvent | React.TouchEvent
 ): { x: number; y: number } => {
 const canvas = canvasRef.current!;
 const rect = canvas.getBoundingClientRect();
 if ("touches" in e) {
 return {
 x: e.touches[0].clientX - rect.left,
 y: e.touches[0].clientY - rect.top,
 };
 }
 return { x: e.clientX - rect.left, y: e.clientY - rect.top };
 };

 const startDraw = (e: React.MouseEvent | React.TouchEvent) => {
 e.preventDefault();
 const ctx = canvasRef.current?.getContext("2d");
 if (!ctx) return;
 const pos = getPos(e);
 ctx.beginPath();
 ctx.moveTo(pos.x, pos.y);
 setIsDrawing(true);
 };

 const draw = (e: React.MouseEvent | React.TouchEvent) => {
 e.preventDefault();
 if (!isDrawing) return;
 const ctx = canvasRef.current?.getContext("2d");
 if (!ctx) return;
 const pos = getPos(e);
 ctx.lineTo(pos.x, pos.y);
 ctx.stroke();
 setHasContent(true);
 };

 const endDraw = () => setIsDrawing(false);

 const clearCanvas = () => {
 const canvas = canvasRef.current;
 if (!canvas) return;
 const ctx = canvas.getContext("2d");
 if (!ctx) return;
 const rect = canvas.getBoundingClientRect();
 const dpr = window.devicePixelRatio || 1;
 ctx.setTransform(1, 0, 0, 1, 0, 0);
 ctx.clearRect(0, 0, canvas.width, canvas.height);
 ctx.scale(dpr, dpr);
 ctx.fillStyle = "#ffffff";
 ctx.fillRect(0, 0, rect.width, rect.height);
 ctx.setLineDash([4, 4]);
 ctx.strokeStyle = "#cbd5e1";
 ctx.lineWidth = 1;
 ctx.beginPath();
 ctx.moveTo(20, rect.height * 0.7);
 ctx.lineTo(rect.width - 20, rect.height * 0.7);
 ctx.stroke();
 ctx.setLineDash([]);
 ctx.strokeStyle = "#1e293b";
 ctx.lineWidth = 2.5;
 ctx.lineCap = "round";
 ctx.lineJoin = "round";
 setHasContent(false);
 };

 const handleSave = () => {
 if (!hasContent || !canvasRef.current) return;
 onSave(canvasRef.current.toDataURL("image/png"));
 };

 return (
 <div className="space-y-4">
 <div className="border-2 border-dashed border-border rounded-xl overflow-hidden bg-card">
 <canvas
 ref={canvasRef}
 className="w-full cursor-crosshair touch-none"
 style={{ height: 180 }}
 onMouseDown={startDraw}
 onMouseMove={draw}
 onMouseUp={endDraw}
 onMouseLeave={endDraw}
 onTouchStart={startDraw}
 onTouchMove={draw}
 onTouchEnd={endDraw}
 />
 </div>
 <p className="text-micro text-muted-foreground text-center">
 Desenhe sua assinatura no campo acima utilizando o mouse ou toque
 </p>
 <div className="flex items-center gap-3">
 <Button variant="outline" onClick={clearCanvas} disabled={isSaving} className="flex-1">
 <Eraser className="w-4 h-4" /> Limpar
 </Button>
 <Button variant="outline" onClick={onCancel} disabled={isSaving} className="flex-1">
 Cancelar
 </Button>
 <Button onClick={handleSave} disabled={!hasContent || isSaving} className="flex-1">
 {isSaving ? <Loader2 className="w-4 h-4 animate-spin" /> : <PenTool className="w-4 h-4" />}
 {isSaving ? "Salvando..." : "Assinar"}
 </Button>
 </div>
 </div>
 );
}

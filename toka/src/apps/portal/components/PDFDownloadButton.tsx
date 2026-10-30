import { useState } from "react";
import { Download, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "../../../shared/ui/button";
import { API_URL, getPortalAccessToken } from "../services/portal-api";

interface PDFDownloadButtonProps {
  url: string;
  filename: string;
  label?: string;
  variant?: "default" | "outline" | "ghost" | "secondary";
  size?: "sm" | "default" | "lg" | "icon";
  className?: string;
}

export function PDFDownloadButton({
  url,
  filename,
  label = "Baixar PDF",
  variant = "outline",
  size = "sm",
  className,
}: PDFDownloadButtonProps) {
  const [loading, setLoading] = useState(false);

  const handleClick = async () => {
    setLoading(true);
    try {
      const token = getPortalAccessToken() ?? "";
      const absoluteUrl = url.startsWith("http") ? url : `${API_URL}${url}`;
      const res = await fetch(absoluteUrl, {
        headers: {
          Authorization: `Bearer ${token}`,
          Accept: "application/pdf",
        },
      });
      if (!res.ok) {
        if (res.status === 401) {
          throw new Error("UNAUTHORIZED");
        }
        throw new Error(`HTTP ${res.status}`);
      }
      const blob = await res.blob();
      const blobUrl = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = blobUrl;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(blobUrl);
    } catch (err) {
      const message =
        err instanceof Error && err.message === "UNAUTHORIZED"
          ? "Sua sessão expirou. Faça login novamente para baixar o PDF."
          : "Não foi possível baixar o PDF.";
      toast.error(message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Button variant={variant} size={size} onClick={handleClick} disabled={loading} className={className}>
      {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Download className="w-4 h-4" />}
      <span className="ml-2">{label}</span>
    </Button>
  );
}


import { useState, useRef } from "react";
import { UploadCloud, File as FileIcon, CheckCircle2, Loader2, X } from "lucide-react";
import { cn } from "../../../shared/ui/utils";

interface FileDropzoneProps {
  accept?: string;
  maxSize?: number;
  onUpload: (file: File) => Promise<void>;
  disabled?: boolean;
  currentFile?: { name: string; url: string } | null;
  className?: string;
}

export function FileDropzone({ accept, maxSize, onUpload, disabled, currentFile, className }: FileDropzoneProps) {
  const [isDragActive, setIsDragActive] = useState(false);
  const [isUploading, setIsUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const handleDragEnter = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (!disabled) setIsDragActive(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragActive(false);
  };

  const handleDrop = async (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragActive(false);
    if (disabled) return;

    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      await processFile(e.dataTransfer.files[0]);
    }
  };

  const handleChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      await processFile(e.target.files[0]);
    }
  };

  const processFile = async (file: File) => {
    setError(null);

    if (maxSize && file.size > maxSize) {
      setError(`O arquivo excede o limite de ${Math.round(maxSize / 1024 / 1024)}MB`);
      return;
    }

    setIsUploading(true);
    try {
      await onUpload(file);
    } catch {
      setError("Erro ao fazer upload do arquivo.");
    } finally {
      setIsUploading(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  };

  void FileIcon;
  void X;

  return (
    <div
      className={cn(
        "relative flex flex-col items-center justify-center p-5 border-2 border-dashed rounded-xl transition-colors cursor-pointer",
        isDragActive ? "border-status-ok bg-status-ok-soft/50" : "border-border hover:border-border",
        disabled ? "opacity-50 cursor-not-allowed" : "",
        className,
      )}
      onDragEnter={handleDragEnter}
      onDragOver={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      onClick={() => !disabled && inputRef.current?.click()}
    >
      <input
        type="file"
        ref={inputRef}
        onChange={handleChange}
        accept={accept}
        className="hidden"
        disabled={disabled || isUploading}
      />

      {isUploading ? (
        <div className="flex flex-col items-center gap-2 text-muted-foreground">
          <Loader2 className="w-8 h-8 animate-spin text-status-ok" />
          <span className="text-sm font-medium">Enviando arquivo...</span>
        </div>
      ) : currentFile ? (
        <div className="flex flex-col items-center gap-2">
          <CheckCircle2 className="w-8 h-8 text-status-ok" />
          <div className="flex items-center gap-2 text-sm">
            <span className="font-medium truncate max-w-[200px]">{currentFile.name}</span>
            <a
              href={currentFile.url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-status-ok hover:underline"
              onClick={(e) => e.stopPropagation()}
            >
              (Ver arquivo)
            </a>
          </div>
          {!disabled && <span className="text-xs text-muted-foreground mt-1">Clique ou arraste outro arquivo para substituir</span>}
        </div>
      ) : (
        <div className="flex flex-col items-center gap-2 text-muted-foreground">
          <UploadCloud className="w-8 h-8 mb-2" />
          <span className="text-sm font-medium">Clique ou arraste um arquivo</span>
          <span className="text-xs">
            {accept ? accept.split(",").join(", ") : "Qualquer arquivo"}
            {maxSize ? ` até ${Math.round(maxSize / 1024 / 1024)}MB` : ""}
          </span>
        </div>
      )}

      {error && <div className="absolute -bottom-6 left-0 text-xs text-destructive">{error}</div>}
    </div>
  );
}


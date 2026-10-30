import React, { useEffect, useMemo, useState } from "react";
import { resolveMediaUrl } from "../media";

export interface MediaImageProps
  extends Omit<React.ImgHTMLAttributes<HTMLImageElement>, "src"> {
  src?: string | null;
  fallback?: React.ReactNode;
  fallbackSrc?: string | null;
}

/**
 * Imagem remota com resolução central de URL e fallback previsível.
 *
 * O estado de erro é reiniciado quando a origem muda, o que permite que
 * previews recém-enviados apareçam sem remontar todo o componente.
 */
export function MediaImage({
  src,
  fallback = null,
  fallbackSrc,
  onError,
  loading = "lazy",
  decoding = "async",
  ...props
}: MediaImageProps) {
  const primarySrc = useMemo(() => resolveMediaUrl(src), [src]);
  const resolvedFallbackSrc = useMemo(
    () => resolveMediaUrl(fallbackSrc),
    [fallbackSrc],
  );
  const [activeSrc, setActiveSrc] = useState<string | null>(primarySrc);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    setActiveSrc(primarySrc);
    setFailed(false);
  }, [primarySrc]);

  if (!activeSrc || failed) {
    return <>{fallback}</>;
  }

  return (
    <img
      {...props}
      src={activeSrc}
      loading={loading}
      decoding={decoding}
      onError={(event) => {
        onError?.(event);
        if (resolvedFallbackSrc && activeSrc !== resolvedFallbackSrc) {
          setActiveSrc(resolvedFallbackSrc);
          return;
        }
        setFailed(true);
      }}
    />
  );
}

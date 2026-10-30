import React, { useEffect, useMemo, useState } from "react";
import suiteeLogo from "../../assets/logo.png";
import { getMediaUrl } from "../media";
import { getImgproxyUrl } from "../imgproxy";

type Props = {
  logoUrl?: string | null;
  alt?: string;
  className?: string;
};

export function CompanyLogo({ logoUrl, alt = "Logo da empresa", className }: Props) {
  const initialSrc = useMemo(() => {
    if (!logoUrl) return suiteeLogo;
    return getImgproxyUrl(getMediaUrl(logoUrl), { width: 160, height: 80, resizingType: "fit" });
  }, [logoUrl]);
  const [src, setSrc] = useState<string>(initialSrc);

  useEffect(() => {
    setSrc(initialSrc);
  }, [initialSrc]);

  return <img src={src} alt={alt} className={className} onError={() => setSrc(suiteeLogo)} />;
}

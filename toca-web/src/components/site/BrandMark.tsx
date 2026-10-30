'use client'

import { useEffect, useState } from 'react'
import type { Tenant } from '@/payload-types'
import {
  isSuiteeLogoUrl,
  tenantDisplayName,
  tenantTagline,
} from '@/lib/format'

interface BrandMarkProps {
  tenant: Tenant
  logoUrl: string | null
  fallbackLogoUrl?: string
  compact?: boolean
}

export function BrandMark({
  tenant,
  logoUrl,
  fallbackLogoUrl,
  compact = false,
}: BrandMarkProps) {
  const name = tenantDisplayName(tenant)
  const tagline = tenantTagline(tenant)
  const [failed, setFailed] = useState(false)
  const [fallbackFailed, setFallbackFailed] = useState(false)
  const fallback = fallbackLogoUrl || '/suitee-logo.png'
  const source = failed ? fallback : logoUrl
  const suiteeFallback = isSuiteeLogoUrl(source)
  const initial = name.charAt(0).toUpperCase() || 'S'

  useEffect(() => {
    setFailed(false)
    setFallbackFailed(false)
  }, [logoUrl])

  return (
    <span className={compact ? 'brand-mark brand-mark--compact' : 'brand-mark'}>
      {source && !fallbackFailed ? (
        <img
          className={
            suiteeFallback
              ? 'brand-mark__image brand-mark__image--suitee'
              : 'brand-mark__image'
          }
          src={source}
          alt={suiteeFallback ? 'Suitee' : name}
          onError={() => {
            if (failed) {
              setFallbackFailed(true)
            } else {
              setFailed(true)
            }
          }}
        />
      ) : (
        <span className="brand-mark__monogram" aria-hidden="true">
          {initial}
        </span>
      )}
      <span className="brand-mark__text">
        <span className="brand-mark__name">{name}</span>
        {!compact ? <span className="brand-mark__tagline">{tagline}</span> : null}
      </span>
    </span>
  )
}


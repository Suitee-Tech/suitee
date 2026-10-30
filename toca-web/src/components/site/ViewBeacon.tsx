'use client'

import { useEffect, useRef } from 'react'
import {
  currentPublicHost,
  currentUtmParams,
  getAnonymousSessionId,
  publicSiteApiEndpoint,
} from '@/lib/public-api'

interface ViewBeaconProps {
  propertyId: string
  siteSlug: string
}

/**
 * Fire-and-forget view beacon. It runs once per mount, never blocks rendering,
 * and never surfaces failures to the visitor. keepalive lets the request finish
 * if the visitor navigates away immediately.
 */
export function ViewBeacon({ propertyId, siteSlug }: ViewBeaconProps) {
  const sent = useRef(false)

  useEffect(() => {
    if (sent.current || !propertyId) {
      return
    }
    sent.current = true

    const controller = new AbortController()
    const timeout = window.setTimeout(() => controller.abort(), 8000)
    const sessionId = getAnonymousSessionId()

    void fetch(publicSiteApiEndpoint('views'), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Site-Host': currentPublicHost(),
      },
      body: JSON.stringify({
        property_id: propertyId,
        site_slug: siteSlug,
        path: window.location.pathname,
        referrer: document.referrer || undefined,
        session_id: sessionId || undefined,
        ...currentUtmParams(),
      }),
      cache: 'no-store',
      keepalive: true,
      signal: controller.signal,
    })
      .catch(() => {
        // Analytics must never degrade the storefront.
      })
      .finally(() => window.clearTimeout(timeout))

    // Do not abort on unmount: the beacon must be allowed to finish after an
    // immediate navigation. `keepalive` plus the timeout bound the request.
    return () => {
      window.clearTimeout(timeout)
    }
  }, [propertyId, siteSlug])

  return null
}

'use client'

import { useEffect } from 'react'

export default function SiteError({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  useEffect(() => {
    // Keep the client console useful in production without exposing details in UI.
    console.error(error)
  }, [error])

  return (
    <div className="host-error">
      <div className="host-error__card">
        <p className="eyebrow">Um instante</p>
        <h1>Não foi possível carregar esta página.</h1>
        <p>Tente novamente em instantes. Se o problema continuar, fale com a imobiliária.</p>
        <button type="button" className="button button--primary" onClick={reset}>
          Tentar novamente
        </button>
      </div>
    </div>
  )
}

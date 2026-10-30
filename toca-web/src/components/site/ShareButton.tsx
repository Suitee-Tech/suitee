'use client'

import { useState } from 'react'
import { ShareIcon } from './icons'

interface ShareButtonProps {
  title: string
}

export function ShareButton({ title }: ShareButtonProps) {
  const [copied, setCopied] = useState(false)

  const share = async () => {
    const url = window.location.href
    try {
      if (navigator.share) {
        await navigator.share({ title, url })
        return
      }
      await navigator.clipboard.writeText(url)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2500)
    } catch {
      // O usuário cancelou o share ou o clipboard foi bloqueado; não há erro
      // de produto a exibir.
    }
  }

  return (
    <button type="button" className="share-button" onClick={() => void share()}>
      <ShareIcon width={16} height={16} />
      {copied ? 'Link copiado' : 'Compartilhar'}
    </button>
  )
}

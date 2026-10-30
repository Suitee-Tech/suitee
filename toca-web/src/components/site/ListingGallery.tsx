'use client'

import { useMemo, useState } from 'react'
import type { Listing } from '@/payload-types'

interface GalleryImage {
  url: string
  alt?: string | null
  caption?: string | null
}

interface ListingGalleryProps {
  title: string
  images: GalleryImage[]
}

export function ListingGallery({ title, images }: ListingGalleryProps) {
  const ordered = useMemo(
    () => images.filter((image) => Boolean(image.url)),
    [images],
  )
  const [activeIndex, setActiveIndex] = useState(0)
  const active = ordered[activeIndex]

  if (!active) {
    return (
      <div className="listing-gallery listing-gallery--empty" role="img" aria-label={title}>
        <span>Fotos em breve</span>
      </div>
    )
  }

  const hasMultiple = ordered.length > 1
  const go = (nextIndex: number) => {
    setActiveIndex((nextIndex + ordered.length) % ordered.length)
  }

  return (
    <div className="listing-gallery">
      <div className="listing-gallery__main">
        <img
          src={active.url}
          alt={active.alt || title}
          className="listing-gallery__image"
          fetchPriority="high"
        />
        {hasMultiple ? (
          <>
            <button
              type="button"
              className="listing-gallery__nav listing-gallery__nav--prev"
              onClick={() => go(activeIndex - 1)}
              aria-label="Foto anterior"
            >
              ‹
            </button>
            <button
              type="button"
              className="listing-gallery__nav listing-gallery__nav--next"
              onClick={() => go(activeIndex + 1)}
              aria-label="Próxima foto"
            >
              ›
            </button>
            <span className="listing-gallery__counter" aria-live="polite">
              {activeIndex + 1} / {ordered.length}
            </span>
          </>
        ) : null}
        {active.caption ? <p className="listing-gallery__caption">{active.caption}</p> : null}
      </div>

      {hasMultiple ? (
        <div className="listing-gallery__thumbs" aria-label="Miniaturas das fotos">
          {ordered.map((image, index) => (
            <button
              key={`${image.url}-${index}`}
              type="button"
              className={
                index === activeIndex
                  ? 'listing-gallery__thumb listing-gallery__thumb--active'
                  : 'listing-gallery__thumb'
              }
              onClick={() => setActiveIndex(index)}
              aria-label={`Ver foto ${index + 1}`}
              aria-pressed={index === activeIndex}
            >
              <img src={image.url} alt="" loading="lazy" />
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}

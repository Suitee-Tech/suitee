import React, { useEffect, useMemo, useState } from 'react'
import { getImgproxyUrl, type ImgproxyOptions } from '../../services/imgproxy'
import { resolveMediaUrl } from '../../../shared/media'

const ERROR_IMG_SRC =
  'data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iODgiIGhlaWdodD0iODgiIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyIgc3Ryb2tlPSIjMDAwIiBzdHJva2UtbGluZWpvaW49InJvdW5kIiBvcGFjaXR5PSIuMyIgZmlsbD0ibm9uZSIgc3Ryb2tlLXdpZHRoPSIzLjciPjxyZWN0IHg9IjE2IiB5PSIxNiIgd2lkdGg9IjU2IiBoZWlnaHQ9IjU2IiByeD0iNiIvPjxwYXRoIGQ9Im0xNiA1OCAxNi0xOCAzMiAzMiIvPjxjaXJjbGUgY3g9IjUzIiBjeT0iMzUiIHI9IjciLz48L3N2Zz4KCg=='

export interface ImageWithFallbackProps extends React.ImgHTMLAttributes<HTMLImageElement> {
  imgproxy?: ImgproxyOptions
}

export function ImageWithFallback({
  src,
  alt,
  style,
  className,
  imgproxy: imgproxyOpts,
  onError,
  loading = 'lazy',
  decoding = 'async',
  ...rest
}: ImageWithFallbackProps) {
  const resolvedSrc = useMemo(() => {
    const safeSrc = typeof src === 'string' ? resolveMediaUrl(src) : null
    if (!safeSrc) return undefined
    return imgproxyOpts !== undefined ? getImgproxyUrl(safeSrc, imgproxyOpts) : safeSrc
  }, [imgproxyOpts, src])

  const [didError, setDidError] = useState(false)

  useEffect(() => {
    setDidError(false)
  }, [resolvedSrc])

  if (!resolvedSrc || didError) {
    return (
      <div
        className={`inline-block bg-muted text-center align-middle ${className ?? ''}`}
        style={style}
      >
        <div className="flex items-center justify-center w-full h-full">
          <img src={ERROR_IMG_SRC} alt="Error loading image" {...rest} data-original-url={src} />
        </div>
      </div>
    )
  }

  return (
    <img
      src={resolvedSrc}
      alt={alt}
      className={className}
      style={style}
      loading={loading}
      decoding={decoding}
      {...rest}
      onError={(event) => {
        onError?.(event)
        setDidError(true)
      }}
    />
  )
}

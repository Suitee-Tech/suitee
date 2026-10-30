import type { SerializedEditorState } from '@payloadcms/richtext-lexical/lexical'
import { RichText as LexicalRichText } from '@payloadcms/richtext-lexical/react'

interface RichTextProps {
  data: unknown
  className?: string
}

/**
 * Server-rendered Lexical content. Plain-text page bodies take precedence in
 * the pages, but the optional rich body and hero are supported.
 */
export function RichText({ data, className }: RichTextProps) {
  if (!data || typeof data !== 'object' || !('root' in data)) {
    return null
  }

  return (
    <LexicalRichText
      data={data as SerializedEditorState}
      className={['richtext', className].filter(Boolean).join(' ')}
    />
  )
}

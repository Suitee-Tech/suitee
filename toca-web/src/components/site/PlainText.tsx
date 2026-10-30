import { paragraphList } from '@/lib/format'

interface PlainTextProps {
  text: string | null | undefined
  className?: string
}

export function PlainText({ text, className }: PlainTextProps) {
  const paragraphs = paragraphList(text)
  if (paragraphs.length === 0) {
    return null
  }

  return (
    <div className={['prose-plain', className].filter(Boolean).join(' ')}>
      {paragraphs.map((paragraph, index) => (
        <p key={`${paragraph.slice(0, 24)}-${index}`}>{paragraph}</p>
      ))}
    </div>
  )
}

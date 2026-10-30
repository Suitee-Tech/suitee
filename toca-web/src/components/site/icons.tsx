import type { SVGProps } from 'react'

type IconProps = SVGProps<SVGSVGElement>

const baseProps: IconProps = {
  width: 20,
  height: 20,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.8,
  strokeLinecap: 'round',
  strokeLinejoin: 'round',
  'aria-hidden': true,
}

export function BedIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M3 20V9a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v11" />
      <path d="M3 16h18M7 7V5h10v2" />
    </svg>
  )
}

export function BathIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M4 12h16v2a5 5 0 0 1-5 5H9a5 5 0 0 1-5-5v-2Z" />
      <path d="M6 12V6a2 2 0 0 1 4 0M8 20l-1 2M16 20l1 2" />
    </svg>
  )
}

export function CarIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="m5 17-1-5 2-5h12l2 5-1 5" />
      <path d="M4 17h16v3H4zM7 14h.01M17 14h.01" />
    </svg>
  )
}

export function AreaIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M4 9V4h5M20 15v5h-5M4 4l6 6M20 20l-6-6" />
    </svg>
  )
}

export function PinIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M20 10c0 5-8 11-8 11S4 15 4 10a8 8 0 1 1 16 0Z" />
      <circle cx="12" cy="10" r="2.5" />
    </svg>
  )
}

export function PhoneIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M22 16.9v3a2 2 0 0 1-2.2 2 19.8 19.8 0 0 1-8.6-3.1 19.4 19.4 0 0 1-6-6A19.8 19.8 0 0 1 2.1 4.2 2 2 0 0 1 4.1 2h3a2 2 0 0 1 2 1.7c.1 1 .4 2 .7 2.9a2 2 0 0 1-.5 2.1L8 10.1a16 16 0 0 0 5.9 5.9l1.4-1.3a2 2 0 0 1 2.1-.5c.9.3 1.9.6 2.9.7a2 2 0 0 1 1.7 2Z" />
    </svg>
  )
}

export function MailIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <rect x="3" y="5" width="18" height="14" rx="2" />
      <path d="m3 7 9 6 9-6" />
    </svg>
  )
}

export function WhatsAppIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M20.5 11.6A8.5 8.5 0 0 1 7.2 19L3 20l1.2-4A8.5 8.5 0 1 1 20.5 11.6Z" />
      <path d="M8.4 7.7c.4-.1.8.1 1 .5l.7 1.3c.1.3.1.6-.1.8l-.6.7a6.7 6.7 0 0 0 3.6 3.6l.7-.6c.2-.2.5-.2.8-.1l1.3.7c.4.2.6.6.5 1-.2.9-.9 1.6-1.8 1.7-3.7.3-7.9-3.9-7.6-7.6.1-.9.7-1.6 1.5-1.8Z" />
    </svg>
  )
}

export function ArrowIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M5 12h14M13 6l6 6-6 6" />
    </svg>
  )
}

export function ArrowLeftIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M19 12H5M11 18l-6-6 6-6" />
    </svg>
  )
}

export function CloseIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M6 6l12 12M18 6 6 18" />
    </svg>
  )
}

export function MenuIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M4 7h16M4 12h16M4 17h16" />
    </svg>
  )
}

export function HomeIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="m3 11 9-7 9 7" />
      <path d="M5 10v10h14V10M10 20v-6h4v6" />
    </svg>
  )
}

export function ShieldIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M12 22s8-3.5 8-10V5l-8-3-8 3v7c0 6.5 8 10 8 10Z" />
      <path d="m9 12 2 2 4-5" />
    </svg>
  )
}

export function HandshakeIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="m11 17 2 2a2 2 0 0 0 3-3l-4-4" />
      <path d="m14 14 2 2a2 2 0 0 0 3-3l-3-3-4 4" />
      <path d="m8 11 3-3 3 3M3 8l4-4 5 5M21 8l-4-4-5 5" />
    </svg>
  )
}

export function ShareIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <circle cx="18" cy="5" r="2.5" />
      <circle cx="6" cy="12" r="2.5" />
      <circle cx="18" cy="19" r="2.5" />
      <path d="m8.2 10.8 7.6-4.2M8.2 13.2l7.6 4.2" />
    </svg>
  )
}

export function HeartIcon({ filled = false, ...props }: IconProps & { filled?: boolean }) {
  return (
    <svg {...baseProps} {...props} fill={filled ? 'currentColor' : 'none'}>
      <path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.6l-1-1a5.5 5.5 0 0 0-7.8 7.8l1 1L12 21.2l7.8-7.8 1-1a5.5 5.5 0 0 0 0-7.8Z" />
    </svg>
  )
}

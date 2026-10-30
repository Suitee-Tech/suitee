'use client'

import { useRef, useState, type FormEvent } from 'react'
import {
  currentPublicHost,
  currentUtmParams,
  publicSiteApiEndpoint,
} from '@/lib/public-api'
import { ArrowIcon, ShieldIcon } from './icons'

type SubmitState = 'idle' | 'submitting' | 'success' | 'error'
type FieldErrors = Partial<Record<'name' | 'email' | 'phone' | 'consent', string>>

interface LeadFormProps {
  propertyId: string
  siteSlug: string
  propertyTitle: string
}

function maskPhone(input: string): string {
  const digits = input.replace(/\D/g, '').slice(0, 11)
  if (!digits) return ''
  if (digits.length <= 2) return `(${digits}`
  const area = digits.slice(0, 2)
  const rest = digits.slice(2)
  if (rest.length <= 4) return `(${area}) ${rest}`
  const split = rest.length > 8 ? 5 : 4
  return `(${area}) ${rest.slice(0, split)}-${rest.slice(split, split + 4)}`
}

function validate(values: {
  name: string
  email: string
  phone: string
  consent: boolean
}): FieldErrors {
  const errors: FieldErrors = {}
  const name = values.name.trim()
  const nameParts = name.split(/\s+/).filter(Boolean)
  if (nameParts.length < 2) {
    errors.name = 'Informe nome e sobrenome.'
  } else if (nameParts.some((part) => part.length < 2)) {
    errors.name = 'Nome e sobrenome precisam ter pelo menos 2 letras.'
  } else if (!/^[A-Za-zÀ-ÖØ-öø-ÿ' -]+$/.test(name)) {
    errors.name = 'Use apenas letras, espaços, apóstrofo ou hífen.'
  }

  if (!/^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/.test(values.email.trim())) {
    errors.email = 'Informe um e-mail válido, como nome@exemplo.com.'
  }

  const digits = values.phone.replace(/\D/g, '')
  if (digits.length !== 10 && digits.length !== 11) {
    errors.phone = 'Informe DDD + telefone, com 10 ou 11 números.'
  }

  if (!values.consent) {
    errors.consent = 'Autorize o contato para enviar a mensagem.'
  }

  return errors
}

export function LeadForm({ propertyId, siteSlug, propertyTitle }: LeadFormProps) {
  const formRef = useRef<HTMLFormElement>(null)
  const [state, setState] = useState<SubmitState>('idle')
  const [errorMessage, setErrorMessage] = useState('')
  const [phone, setPhone] = useState('')
  const [errors, setErrors] = useState<FieldErrors>({})

  function clearError(field: keyof FieldErrors) {
    setErrors((previous) => {
      if (!previous[field]) return previous
      const next = { ...previous }
      delete next[field]
      return next
    })
    if (state === 'error') setState('idle')
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (state === 'submitting') return

    const form = event.currentTarget
    const data = new FormData(form)
    const values = {
      name: String(data.get('name') ?? '').trim(),
      email: String(data.get('email') ?? '').trim(),
      phone,
      consent: data.get('accepted_contact') === 'on',
    }
    const nextErrors = validate(values)
    setErrors(nextErrors)

    if (Object.keys(nextErrors).length > 0) {
      requestAnimationFrame(() => {
        form.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus()
      })
      return
    }

    const payload = {
      property_id: propertyId,
      site_slug: siteSlug,
      name: values.name,
      email: values.email,
      phone: maskPhone(values.phone),
      message: String(data.get('message') ?? '').trim(),
      accepted_contact: values.consent,
      website: String(data.get('website') ?? ''),
      ...currentUtmParams(),
    }

    setState('submitting')
    setErrorMessage('')

    try {
      const response = await fetch(publicSiteApiEndpoint('leads'), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Site-Host': currentPublicHost(),
        },
        body: JSON.stringify(payload),
        cache: 'no-store',
      })

      if (!response.ok) {
        let message = ''
        let field = ''
        try {
          const body = await response.json()
          const firstField = body?.data?.fields?.[0]
          message = firstField?.message || body?.message || ''
          field = firstField?.field || ''
        } catch {}
        const error = new Error(message || `lead request failed with ${response.status}`)
        ;(error as Error & { field?: string }).field = field
        throw error
      }

      form.reset()
      setPhone('')
      setErrors({})
      setState('success')
    } catch (error) {
      setState('error')
      const field = (error as Error & { field?: string }).field
      if (field === 'name') {
        setErrors((previous) => ({ ...previous, name: (error as Error).message }))
      }
      const message = (error as Error).message
      setErrorMessage(
        message && !message.startsWith('lead request failed with')
          ? message
          : 'Não foi possível enviar agora. Tente novamente ou fale pelo WhatsApp da imobiliária.',
      )
    }
  }

  if (state === 'success') {
    return (
      <div className="lead-success" role="status">
        <span className="lead-success__icon">
          <ArrowIcon width={22} height={22} />
        </span>
        <h2>Mensagem enviada</h2>
        <p>
          A imobiliária recebeu seu interesse em <strong>{propertyTitle}</strong> e vai entrar em
          contato pelos dados informados.
        </p>
        <button
          type="button"
          className="button button--ghost"
          onClick={() => {
            setState('idle')
            setErrors({})
          }}
        >
          Enviar outra mensagem
        </button>
      </div>
    )
  }

  return (
    <form ref={formRef} className="lead-form" onSubmit={handleSubmit} noValidate>
      <div className="lead-form__heading">
        <p className="eyebrow">Fale com a imobiliária</p>
        <h2>Tenho interesse neste imóvel</h2>
        <p>Preencha seus dados e receba o atendimento de um corretor.</p>
      </div>

      <div className="field">
        <label htmlFor="lead-name">Nome completo</label>
        <input
          id="lead-name"
          name="name"
          type="text"
          autoComplete="name"
          required
          maxLength={120}
          aria-invalid={Boolean(errors.name)}
          aria-describedby={errors.name ? 'lead-name-error' : undefined}
          onBlur={(event) => {
            const error = validate({
              name: event.target.value,
              email: '',
              phone: '0000000000',
              consent: true,
            }).name
            setErrors((previous) => ({ ...previous, ...(error ? { name: error } : {}) }))
          }}
          onChange={() => clearError('name')}
        />
        {errors.name ? (
          <p id="lead-name-error" className="field__error" role="alert">{errors.name}</p>
        ) : null}
      </div>

      <div className="lead-form__row">
        <div className="field">
          <label htmlFor="lead-email">E-mail</label>
          <input
            id="lead-email"
            name="email"
            type="email"
            autoComplete="email"
            required
            maxLength={180}
            aria-invalid={Boolean(errors.email)}
            aria-describedby={errors.email ? 'lead-email-error' : undefined}
            onBlur={(event) => {
              const error = validate({
                name: 'Nome',
                email: event.target.value,
                phone: '0000000000',
                consent: true,
              }).email
              setErrors((previous) => ({ ...previous, ...(error ? { email: error } : {}) }))
            }}
            onChange={() => clearError('email')}
          />
          {errors.email ? (
            <p id="lead-email-error" className="field__error" role="alert">{errors.email}</p>
          ) : null}
        </div>
        <div className="field">
          <label htmlFor="lead-phone">Telefone / WhatsApp</label>
          <input
            id="lead-phone"
            name="phone"
            type="tel"
            autoComplete="tel"
            required
            inputMode="numeric"
            maxLength={15}
            placeholder="(00) 00000-0000"
            value={phone}
            aria-invalid={Boolean(errors.phone)}
            aria-describedby={errors.phone ? 'lead-phone-error' : undefined}
            onChange={(event) => {
              setPhone(maskPhone(event.target.value))
              clearError('phone')
            }}
            onBlur={() => {
              const digits = phone.replace(/\D/g, '')
              if (digits.length !== 10 && digits.length !== 11) {
                setErrors((previous) => ({
                  ...previous,
                  phone: 'Informe DDD + telefone, com 10 ou 11 números.',
                }))
              }
            }}
          />
          {errors.phone ? (
            <p id="lead-phone-error" className="field__error" role="alert">{errors.phone}</p>
          ) : null}
        </div>
      </div>

      <div className="field">
        <label htmlFor="lead-message">Mensagem (opcional)</label>
        <textarea
          id="lead-message"
          name="message"
          rows={4}
          maxLength={1200}
          placeholder="Gostaria de saber mais sobre este imóvel."
        />
      </div>

      <div className="lead-form__honeypot" aria-hidden="true">
        <label htmlFor="lead-website">Não preencha este campo</label>
        <input id="lead-website" name="website" type="text" tabIndex={-1} autoComplete="off" />
      </div>

      <label className="consent">
        <input
          type="checkbox"
          name="accepted_contact"
          required
          aria-invalid={Boolean(errors.consent)}
          aria-describedby={errors.consent ? 'lead-consent-error' : undefined}
          onChange={() => clearError('consent')}
        />
        <span>
          Autorizo o contato desta imobiliária por telefone, e-mail ou WhatsApp sobre este imóvel.
        </span>
      </label>
      {errors.consent ? (
        <p id="lead-consent-error" className="field__error" role="alert">{errors.consent}</p>
      ) : null}

      {state === 'error' ? (
        <p className="form-error" role="alert">
          {errorMessage}
        </p>
      ) : null}

      <button type="submit" className="button button--primary button--block" disabled={state === 'submitting'}>
        {state === 'submitting' ? 'Enviando…' : 'Quero mais informações'}
      </button>

      <p className="lead-form__privacy">
        <ShieldIcon width={15} height={15} />
        Seus dados são enviados apenas para a imobiliária responsável por este anúncio.
      </p>
    </form>
  )
}

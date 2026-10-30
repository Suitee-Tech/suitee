// DESATIVADO em 2026-10-26: a simulação de financiamento fica oculta até
// existir uma integração bancária validada. O componente é mantido isolado
// para reutilização futura; não é importado por nenhuma página pública.
'use client'

import { useMemo, useState } from 'react'

interface MortgageSimulatorProps {
  price: number
  whatsappLink?: string
  agencyName: string
}

function brl(value: number): string {
  return new Intl.NumberFormat('pt-BR', {
    style: 'currency',
    currency: 'BRL',
    maximumFractionDigits: 0,
  }).format(Number.isFinite(value) ? value : 0)
}

export function MortgageSimulator({ price, whatsappLink, agencyName }: MortgageSimulatorProps) {
  const [downPayment, setDownPayment] = useState(Math.round(price * 0.2))
  const [years, setYears] = useState(30)
  const [annualRate, setAnnualRate] = useState(10.5)

  const result = useMemo(() => {
    const down = Math.min(Math.max(Number.isFinite(downPayment) ? downPayment : 0, 0), price)
    const financed = Math.max(price - down, 0)
    const months = Math.max(years * 12, 1)
    const monthlyRate = Math.max(annualRate, 0) / 100 / 12
    const installment =
      monthlyRate === 0
        ? financed / months
        : (financed * monthlyRate) / (1 - Math.pow(1 + monthlyRate, -months))
    return { down, financed, months, installment: Number.isFinite(installment) ? installment : 0 }
  }, [annualRate, downPayment, price, years])

  if (price <= 0) return null

  return (
    <section className="mortgage" aria-labelledby="mortgage-title">
      <div className="mortgage__header">
        <p className="eyebrow">Ferramenta</p>
        <h2 id="mortgage-title">Simule o financiamento</h2>
        <p>Faça uma estimativa antes de falar com a imobiliária. Os valores são aproximados.</p>
      </div>

      <div className="mortgage__controls">
        <label className="field">
          <span className="field__label">Entrada</span>
          <input
            type="number"
            min={0}
            max={price}
            step={1000}
            inputMode="numeric"
            value={downPayment}
            onChange={(event) => setDownPayment(Number(event.target.value))}
          />
          <span className="mortgage__hint">Imóvel: {brl(price)}</span>
        </label>
        <label className="field">
          <span className="field__label">Prazo</span>
          <select value={years} onChange={(event) => setYears(Number(event.target.value))}>
            {[10, 15, 20, 25, 30, 35].map((option) => (
              <option key={option} value={option}>{option} anos</option>
            ))}
          </select>
        </label>
        <label className="field">
          <span className="field__label">Taxa de juros (a.a.)</span>
          <input
            type="number"
            min={0}
            max={50}
            step={0.1}
            inputMode="decimal"
            value={annualRate}
            onChange={(event) => setAnnualRate(Number(event.target.value))}
          />
          <span className="mortgage__hint">% ao ano</span>
        </label>
      </div>

      <div className="mortgage__result" aria-live="polite">
        <div>
          <span>Parcela estimada</span>
          <strong>{brl(result.installment)}<small>/mês</small></strong>
        </div>
        <dl>
          <div><dt>Entrada</dt><dd>{brl(result.down)}</dd></div>
          <div><dt>Valor financiado</dt><dd>{brl(result.financed)}</dd></div>
          <div><dt>Parcelas</dt><dd>{result.months}x</dd></div>
        </dl>
      </div>

      <p className="mortgage__note">
        Simulação sem juros de seguro, taxas ou correção. As condições finais são definidas pela
        instituição financeira e pela imobiliária.
      </p>
      {whatsappLink ? (
        <a href={whatsappLink} target="_blank" rel="noreferrer" className="button button--primary">
          Falar com {agencyName} sobre este imóvel
        </a>
      ) : null}
    </section>
  )
}

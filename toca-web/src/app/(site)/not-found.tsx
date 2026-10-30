import Link from 'next/link'

export default function HostNotFoundPage() {
  return (
    <div className="host-error">
      <div className="host-error__card">
        <p className="eyebrow">Site de imóveis</p>
        <h1>Este endereço não está disponível.</h1>
        <p>
          O site da imobiliária pode ter sido desativado, movido ou o endereço foi digitado
          incorretamente.
        </p>
        <Link href="/" className="button button--primary">
          Voltar ao início
        </Link>
      </div>
    </div>
  )
}

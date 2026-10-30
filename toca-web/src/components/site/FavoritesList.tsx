'use client'

import Link from 'next/link'
import { useFavorites } from './useFavorites'
import { ArrowIcon, HeartIcon, PinIcon } from './icons'

export function FavoritesList({ tenant }: { tenant: string }) {
  const { items, ready, toggle } = useFavorites(tenant)

  return (
    <section className="section section--favorites">
      <div className="site-container">
        <nav className="breadcrumbs" aria-label="Navegação">
          <Link href="/">Início</Link>
          <span>/</span>
          <span aria-current="page">Favoritos</span>
        </nav>
        <div className="page-hero__headline favorites-heading">
          <div>
            <p className="eyebrow">Sua seleção</p>
            <h1>Imóveis favoritos</h1>
            <p>Os imóveis que você salvou neste navegador. Você pode voltar a eles quando quiser.</p>
          </div>
        </div>

        {!ready ? (
          <p className="muted" role="status">Carregando favoritos…</p>
        ) : items.length === 0 ? (
          <div className="empty-state">
            <HeartIcon width={28} height={28} />
            <h3>Nenhum favorito ainda</h3>
            <p>Toque no coração de um imóvel para guardar sua seleção aqui.</p>
            <Link href="/imoveis" className="button button--primary">
              Ver imóveis
              <ArrowIcon width={17} height={17} />
            </Link>
          </div>
        ) : (
          <div className="favorites-grid">
            {items.map((item) => (
              <article className="favorite-card" key={item.id}>
                <Link href={`/imoveis/${item.slug}`} className="favorite-card__media">
                  {item.imageUrl ? (
                    <img src={item.imageUrl} alt={item.title} loading="lazy" />
                  ) : (
                    <span aria-hidden="true">Imóvel</span>
                  )}
                </Link>
                <div className="favorite-card__body">
                  <p className="listing-card__category">{item.listingType === 'rent' ? 'Locação' : 'Venda'}</p>
                  <h2><Link href={`/imoveis/${item.slug}`}>{item.title}</Link></h2>
                  {item.location ? (
                    <p className="listing-card__location"><PinIcon width={15} height={15} />{item.location}</p>
                  ) : null}
                  <div className="favorite-card__footer">
                    <strong>{item.price}</strong>
                    <button type="button" className="favorites-remove" onClick={() => toggle(item)}>
                      Remover
                    </button>
                  </div>
                </div>
              </article>
            ))}
          </div>
        )}
      </div>
    </section>
  )
}

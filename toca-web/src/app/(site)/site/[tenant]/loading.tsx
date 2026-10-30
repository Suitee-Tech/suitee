export default function SiteLoading() {
  return (
    <div className="site-loading" role="status" aria-live="polite" aria-label="Carregando página">
      <div className="site-container">
        <div className="site-loading__hero">
          <span className="skeleton skeleton--eyebrow" />
          <span className="skeleton skeleton--title" />
          <span className="skeleton skeleton--title skeleton--short" />
          <span className="skeleton skeleton--text" />
        </div>
        <div className="site-loading__grid">
          {[0, 1, 2].map((item) => (
            <div className="skeleton-card" key={item}>
              <span className="skeleton skeleton--media" />
              <span className="skeleton skeleton--text" />
              <span className="skeleton skeleton--text skeleton--short" />
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

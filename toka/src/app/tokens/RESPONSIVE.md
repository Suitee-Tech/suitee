# Responsive Design Patterns

Guide for building mobile-first views in the Suitee admin. The portal is
already mobile-first by construction (`max-w-lg mx-auto`); this document is
about the admin (CRM) and shared components.

## Breakpoints

We use the Tailwind defaults — no custom breakpoints.

| Prefix | Min width | Use it for |
|---|---|---|
| (none) | — | Mobile-first base styles. Always start here. |
| `sm:` | 640px | Form fields side-by-side, inline buttons, 2-col grids when content is short. |
| `md:` | 768px | Tablets and up. **Default "desktop layout" breakpoint** (kanban, multi-pane, full tables). |
| `lg:` | 1024px | Sidebar expanded, 3-col grids, content shoulders. |
| `xl:` | 1280px | Wide screens. 4-col KPI grids, side panels with content shoulders. |

## Core principle: mobile-first

Every utility must start at `grid-cols-1` (or its single-column equivalent) and
grow up. Reaching for `grid-cols-3` without a mobile fallback breaks the page.

```tsx
// ❌ Bad — fixed 3 cols on every viewport
<div className="grid grid-cols-3 gap-4">

// ✅ Good — single column on mobile, 3 on tablet+
<div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
```

## Tables: use `<ResponsiveTable>`

Tables are dense by nature and don't survive scaled-down viewports. Use the
shared helper at `components/ui/responsive-table.tsx`.

```tsx
import { ResponsiveTable } from "../components/ui/responsive-table";

<ResponsiveTable
  data={charges}
  getRowKey={(row) => row.id}
  columns={[
    { key: "tenant", header: "Inquilino", cell: (r) => r.tenantName, primary: true },
    { key: "value", header: "Valor", cell: (r) => formatCurrencyBRL(r.value) },
    { key: "due", header: "Vencimento", cell: (r) => r.dueDate },
    { key: "status", header: "Status", cell: (r) => <StatusPill tone={statusTone(r)}>{statusLabel(r)}</StatusPill> },
    { key: "actions", header: "", cell: (r) => <RowMenu row={r} />, hideOnMobile: true },
  ]}
  empty="Nenhuma cobrança encontrada."
/>
```

At `≥md` it renders a `<table>`. Below `md` it renders cards with the `primary`
column as the title and the others as a label/value grid. Pass `mobileCard` to
fully customize the card.

**Anti-pattern**: `min-w-[1000px]` on a `<table>` forces horizontal scroll and
hides scannability. Migrate to `ResponsiveTable` instead.

## Kanban: list view in mobile

Use `useMediaQuery` (or the `useIsAtLeastMd` preset) to force list mode on small
screens.

```tsx
import { useIsAtLeastMd } from "../hooks/useMediaQuery";

const isDesktop = useIsAtLeastMd();
const effectiveViewMode = isDesktop ? viewMode : "list";

// Render the toggle only on desktop
{isDesktop && <ViewToggle value={viewMode} onChange={setViewMode} />}
```

Drag-and-drop relies on HTML5 backend which doesn't work with touch. Disable
DnD on mobile (`isDesktop` gate) and offer a "Mover para…" menu instead.

## Forms and modals

Default to single column under `sm`. Most form fields can pair up at `sm:` and
above.

```tsx
<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
  <Input name="firstName" />
  <Input name="lastName" />
</div>
```

For long forms or step-based flows on mobile, prefer the `Drawer` over `Dialog`
— drawers are scroll-friendly and feel native on touch.

## Side panels

`ExpandableSidePanel` is `fixed inset-0 flex justify-end` so the panel always
fills the viewport on small screens. The `defaultSize` / `expandedSize` only
constrain `max-width` — at <640px the panel naturally hits 100vw. Nothing to
do here for new panels; keep using the helper.

## Touch targets

Buttons and tappable elements should be ≥40px on a side. Tailwind's `h-10`
maps to 40px. The shared `Button` component already starts at 40px (`size: default`).
For icon-only buttons in dense layouts, use `size="icon"` (40×40).

```tsx
// ❌ Bad — 32px touch target on mobile
<button className="h-8 px-2">⋯</button>

// ✅ Good — 40px (or use Button size="icon")
<Button size="icon" variant="ghost"><MoreHorizontal className="w-4 h-4" /></Button>
```

## Typography that scales

For very large headers (`text-section` = 22px / `text-display` = 72px), drop
one tier on mobile when the header risks truncation:

```tsx
<h1 className="text-title-lg sm:text-section">Página de Aluguéis</h1>
```

Keep body copy and labels at their token sizes (they're already small enough).

## `useMediaQuery` for runtime decisions

When a layout decision needs JS (not just CSS), use the hook from
`hooks/useMediaQuery.ts`. Presets:

| Hook | Query |
|---|---|
| `useIsMobile()` | `(max-width: 767px)` |
| `useIsTablet()` | `(min-width: 768px) and (max-width: 1023px)` |
| `useIsDesktop()` | `(min-width: 1024px)` |
| `useIsAtLeastMd()` | `(min-width: 768px)` (most common) |

```tsx
const isMobile = useIsMobile();
return isMobile ? <ListView /> : <KanbanBoard />;
```

## Anti-patterns

- ❌ `min-w-[Npx]` inside a table wrapper — forces scroll, hides content.
- ❌ `grid-cols-3` (or 4, 5, 6) without `sm:` / `md:` fallback to `1` or `2`.
- ❌ Hardcoded `text-[Npx]` in headers — use `text-title-lg sm:text-section`.
- ❌ Side-by-side panes (`grid-cols-3` chat split) without a mobile single-pane fallback.
- ❌ `h-8` buttons in primary actions — too small for touch.
- ❌ Reaching for `window.innerWidth` directly — use `useMediaQuery`.

## Checking your work

In Chrome DevTools, test at:

- **375px** (iPhone SE) — must not overflow horizontally.
- **768px** (iPad portrait) — desktop layout starts here.
- **1024px** (laptop) — sidebar can expand.

Use the device toolbar and rotate to portrait/landscape to confirm both axes.

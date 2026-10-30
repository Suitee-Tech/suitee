import type { Field } from 'payload'

/**
 * Shared SEO fields for CMS pages. Listing metadata is derived from the listing
 * projection itself (title, first image, agency) so toca-sites never needs to
 * send extra SEO machinery.
 */
export const pageSeoFields: Field = {
  name: 'seo',
  type: 'group',
  label: 'SEO',
  admin: {
    description: 'Opcional. Quando vazio, usamos o título e o subtítulo da página.',
  },
  fields: [
    {
      name: 'metaTitle',
      type: 'text',
      label: 'Título (meta title)',
      maxLength: 70,
    },
    {
      name: 'metaDescription',
      type: 'textarea',
      label: 'Descrição (meta description)',
      maxLength: 180,
    },
    {
      name: 'ogImage',
      type: 'upload',
      label: 'Imagem de compartilhamento',
      relationTo: 'media',
    },
    {
      name: 'noindex',
      type: 'checkbox',
      label: 'Não indexar esta página',
      defaultValue: false,
    },
  ],
}

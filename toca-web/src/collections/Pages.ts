import type { CollectionConfig } from 'payload'
import {
  internalCreateAccess,
  internalUpdateAccess,
  publishedTenantReadAccess,
  superAdminAccess,
} from '@/access'
import { pageSeoFields } from '@/fields/seo'
import { forceTenantFromInternal } from '@/hooks/tenant'

export const Pages: CollectionConfig = {
  slug: 'pages',
  labels: {
    singular: 'Página',
    plural: 'Páginas',
  },
  admin: {
    useAsTitle: 'title',
    group: 'Sites',
    defaultColumns: ['title', 'pageKey', 'status', 'updatedAt'],
    description: 'Conteúdo das três páginas institucionais por imobiliária.',
  },
  access: {
    create: internalCreateAccess,
    read: publishedTenantReadAccess('pages'),
    update: internalUpdateAccess,
    delete: superAdminAccess,
  },
  hooks: {
    beforeValidate: [forceTenantFromInternal],
  },
  indexes: [{ unique: true, fields: ['tenant', 'pageKey'] }],
  fields: [
    {
      name: 'pageKey',
      label: 'Página',
      type: 'select',
      required: true,
      options: [
        { label: 'Home', value: 'home' },
        { label: 'Sobre', value: 'about' },
        { label: 'Contato', value: 'contact' },
      ],
    },
    {
      name: 'title',
      label: 'Título',
      type: 'text',
      required: true,
      maxLength: 140,
    },
    {
      name: 'subtitle',
      label: 'Subtítulo',
      type: 'textarea',
      maxLength: 320,
    },
    {
      name: 'hero',
      label: 'Destaque (rich text)',
      type: 'richText',
      admin: {
        description: 'Conteúdo curto opcional exibido junto ao título do topo.',
      },
    },
    {
      name: 'body',
      label: 'Corpo (texto)',
      type: 'textarea',
      admin: {
        description: 'Texto simples editado pelo Toka. Quebras de linha viram parágrafos.',
        rows: 14,
      },
    },
    {
      name: 'bodyRich',
      label: 'Corpo (rich text avançado)',
      type: 'richText',
      admin: {
        description: 'Opcional. Usado apenas quando o corpo em texto simples está vazio.',
      },
    },
    {
      name: 'status',
      label: 'Status',
      type: 'select',
      required: true,
      defaultValue: 'draft',
      index: true,
      options: [
        { label: 'Rascunho', value: 'draft' },
        { label: 'Publicada', value: 'published' },
      ],
    },
    {
      name: 'publishedAt',
      label: 'Publicada em',
      type: 'date',
      admin: {
        date: { pickerAppearance: 'dayAndTime' },
      },
    },
    pageSeoFields,
  ],
}

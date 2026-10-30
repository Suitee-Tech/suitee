import type { CollectionConfig } from 'payload'
import {
  internalCreateAccess,
  internalUpdateAccess,
  publishedTenantReadAccess,
  superAdminAccess,
} from '@/access'
import {
  forceTenantFromInternal,
  setListingPublishedAt,
  validateSeoSlug,
  validateUuid,
} from '@/hooks/tenant'

const listingTypeOptions = [
  { label: 'Venda', value: 'sale' },
  { label: 'Locação', value: 'rent' },
]

const propertyTypeOptions = [
  { label: 'Apartamento', value: 'apartment' },
  { label: 'Casa', value: 'house' },
  { label: 'Terreno', value: 'land' },
  { label: 'Comercial', value: 'commercial' },
]

const statusOptions = [
  { label: 'Publicado', value: 'published' },
  { label: 'Despublicado', value: 'unpublished' },
  { label: 'Rascunho', value: 'draft' },
]

export const Listings: CollectionConfig = {
  slug: 'listings',
  labels: {
    singular: 'Anúncio',
    plural: 'Anúncios',
  },
  admin: {
    useAsTitle: 'title',
    group: 'Sites',
    defaultColumns: ['title', 'seoSlug', 'status', 'listingType', 'address.city', 'updatedAt'],
    listSearchableFields: ['title', 'seoSlug', 'propertyId', 'address.city'],
    description:
      'Projeção pública dos imóveis do toca-core. O Go mantém os dados; o Payload não é source of truth.',
  },
  access: {
    create: internalCreateAccess,
    read: publishedTenantReadAccess('listings'),
    update: internalUpdateAccess,
    delete: superAdminAccess,
  },
  hooks: {
    beforeValidate: [forceTenantFromInternal, setListingPublishedAt],
  },
  indexes: [
    { unique: true, fields: ['tenant', 'propertyId'] },
    { unique: true, fields: ['tenant', 'seoSlug'] },
    { fields: ['tenant', 'status'] },
  ],
  fields: [
    {
      name: 'propertyId',
      label: 'UUID do imóvel (toca-core)',
      type: 'text',
      required: true,
      index: true,
      validate: validateUuid,
    },
    {
      name: 'seoSlug',
      label: 'Slug do anúncio',
      type: 'text',
      required: true,
      index: true,
      validate: validateSeoSlug,
    },
    {
      name: 'title',
      label: 'Título',
      type: 'text',
      required: true,
      maxLength: 180,
    },
    {
      name: 'description',
      label: 'Descrição (texto)',
      type: 'textarea',
      required: true,
      admin: {
        description: 'Texto simples enviado pelo Go; quebras de linha viram parágrafos.',
        rows: 12,
      },
    },
    {
      name: 'listingType',
      label: 'Tipo de anúncio',
      type: 'select',
      required: true,
      index: true,
      options: listingTypeOptions,
    },
    {
      name: 'propertyType',
      label: 'Tipo de imóvel',
      type: 'select',
      required: true,
      index: true,
      options: propertyTypeOptions,
    },
    {
      type: 'row',
      fields: [
        {
          name: 'salePrice',
          label: 'Preço de venda (R$)',
          type: 'number',
          min: 0,
          admin: { width: '25%' },
        },
        {
          name: 'rentPrice',
          label: 'Aluguel (R$/mês)',
          type: 'number',
          min: 0,
          admin: { width: '25%' },
        },
        {
          name: 'condoFee',
          label: 'Condomínio (R$)',
          type: 'number',
          min: 0,
          admin: { width: '25%' },
        },
        {
          name: 'propertyTax',
          label: 'IPTU (R$)',
          type: 'number',
          min: 0,
          admin: { width: '25%' },
        },
      ],
    },
    {
      name: 'address',
      label: 'Endereço',
      type: 'group',
      fields: [
        {
          type: 'row',
          fields: [
            { name: 'street', label: 'Logradouro', type: 'text', admin: { width: '60%' } },
            { name: 'number', label: 'Número', type: 'text', admin: { width: '20%' } },
            { name: 'complement', label: 'Complemento', type: 'text', admin: { width: '20%' } },
          ],
        },
        {
          type: 'row',
          fields: [
            { name: 'neighborhood', label: 'Bairro', type: 'text', admin: { width: '40%' } },
            { name: 'city', label: 'Cidade', type: 'text', index: true, admin: { width: '30%' } },
            { name: 'state', label: 'UF', type: 'text', maxLength: 2, admin: { width: '10%' } },
            { name: 'postalCode', label: 'CEP', type: 'text', admin: { width: '20%' } },
          ],
        },
        {
          name: 'showExactAddress',
          label: 'Exibir endereço exato',
          type: 'checkbox',
          defaultValue: false,
          admin: {
            description:
              'Quando desmarcado, o site mostra apenas bairro/cidade. Recomendado para privacidade.',
          },
        },
      ],
    },
    {
      name: 'features',
      label: 'Características',
      type: 'group',
      fields: [
        {
          type: 'row',
          fields: [
            { name: 'bedrooms', label: 'Quartos', type: 'number', min: 0, admin: { width: '25%' } },
            { name: 'bathrooms', label: 'Banheiros', type: 'number', min: 0, admin: { width: '25%' } },
            { name: 'suites', label: 'Suítes', type: 'number', min: 0, admin: { width: '25%' } },
            {
              name: 'parkingSpaces',
              label: 'Vagas',
              type: 'number',
              min: 0,
              admin: { width: '25%' },
            },
          ],
        },
        {
          type: 'row',
          fields: [
            {
              name: 'areaTotal',
              label: 'Área total (m²)',
              type: 'number',
              min: 0,
              admin: { width: '50%' },
            },
            {
              name: 'areaBuilt',
              label: 'Área construída (m²)',
              type: 'number',
              min: 0,
              admin: { width: '50%' },
            },
          ],
        },
      ],
    },
    {
      name: 'amenities',
      label: 'Comodidades',
      type: 'text',
      hasMany: true,
      admin: {
        description: 'Lista de itens, por exemplo Piscina, Academia, Elevador.',
      },
    },
    {
      name: 'images',
      label: 'Fotos',
      type: 'array',
      admin: {
        description: 'URLs externas do Storage. A primeira imagem (order 0) é a capa.',
      },
      fields: [
        {
          name: 'url',
          label: 'URL',
          type: 'text',
          required: true,
        },
        {
          name: 'alt',
          label: 'Texto alternativo',
          type: 'text',
        },
        {
          name: 'caption',
          label: 'Legenda',
          type: 'text',
        },
        {
          name: 'order',
          label: 'Ordem',
          type: 'number',
          min: 0,
          defaultValue: 0,
        },
      ],
    },
    {
      name: 'status',
      label: 'Status',
      type: 'select',
      required: true,
      defaultValue: 'draft',
      index: true,
      options: statusOptions,
    },
    {
      type: 'row',
      fields: [
        {
          name: 'publishedAt',
          label: 'Publicado em',
          type: 'date',
          index: true,
          admin: { width: '50%', date: { pickerAppearance: 'dayAndTime' } },
        },
        {
          name: 'sourceUpdatedAt',
          label: 'Atualizado no toca-core',
          type: 'date',
          admin: { width: '50%', date: { pickerAppearance: 'dayAndTime' } },
        },
      ],
    },
  ],
}

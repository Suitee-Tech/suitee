import type { CollectionConfig } from 'payload'
import {
  isSuperAdminUser,
  tenantsCreateAccess,
  tenantsDeleteAccess,
  tenantsReadAccess,
  tenantsUpdateAccess,
} from '@/access'
import { getInternalServiceUser } from '@/access/internal-auth'
import {
  forceSuiteeTenantIdFromInternal,
  validateHexColor,
  validatePublicDomain,
  validateSiteSlug,
  validateUuid,
} from '@/hooks/tenant'

const themeOptions = [
  { label: 'Claro', value: 'light' },
  { label: 'Escuro', value: 'dark' },
]

export const Tenants: CollectionConfig = {
  slug: 'tenants',
  labels: {
    singular: 'Site da imobiliária',
    plural: 'Sites das imobiliárias',
  },
  admin: {
    useAsTitle: 'name',
    group: 'Sites',
    defaultColumns: ['name', 'slug', 'enabled', 'updatedAt'],
    description:
      'Um tenant por imobiliária. O Go (toca-sites) mantém este registro via API interna.',
  },
  access: {
    create: tenantsCreateAccess,
    read: tenantsReadAccess,
    update: tenantsUpdateAccess,
    delete: tenantsDeleteAccess,
  },
  hooks: {
    beforeValidate: [forceSuiteeTenantIdFromInternal],
  },
  fields: [
    {
      name: 'suiteeTenantId',
      label: 'Company UUID (Toka)',
      type: 'text',
      required: true,
      unique: true,
      index: true,
      validate: validateUuid,
      access: {
        // Internal/admin only. Public REST can read the tenant's public
        // branding/contact but never the company UUID.
        read: ({ req }) =>
          Boolean(getInternalServiceUser(req.user)) || isSuperAdminUser(req.user),
      },
      admin: {
        description: 'UUID da company no Toka. Imutável para chamadas internas.',
      },
    },
    {
      name: 'slug',
      label: 'Marca (subdomínio)',
      type: 'text',
      required: true,
      unique: true,
      index: true,
      validate: validateSiteSlug,
      admin: {
        description: 'Vira {marca}.imoveis.suitee.com.br. Não use palavras reservadas.',
      },
    },
    {
      name: 'name',
      label: 'Nome fantasia',
      type: 'text',
      required: true,
      admin: {
        description: 'Marca exibida no site, OG image e JSON-LD.',
      },
    },
    {
      name: 'enabled',
      label: 'Site ativo',
      type: 'checkbox',
      defaultValue: false,
      index: true,
      admin: {
        description: 'Quando desligado, o host público retorna 404.',
      },
    },
    {
      name: 'domain',
      label: 'Domínio próprio (fase 2)',
      type: 'text',
      validate: validatePublicDomain,
      admin: {
        description: 'Reservado para domínio próprio futuro. A vitrine usa o subdomínio da marca.',
      },
    },
    {
      name: 'domains',
      label: 'Domínios públicos',
      type: 'array',
      admin: {
        description: 'Inclui o host canônico {marca}.imoveis.suitee.com.br usado pelo Go.',
      },
      fields: [
        {
          name: 'domain',
          label: 'Domínio',
          type: 'text',
          required: true,
          validate: validatePublicDomain,
        },
      ],
    },
    {
      name: 'branding',
      label: 'Identidade visual',
      type: 'group',
      fields: [
        {
          name: 'logoUrl',
          label: 'URL do logo',
          type: 'text',
          admin: {
            description: 'URL externa tem prioridade sobre o upload abaixo.',
          },
        },
        {
          name: 'logo',
          label: 'Logo (upload)',
          type: 'upload',
          relationTo: 'media',
        },
        {
          name: 'coverUrl',
          label: 'URL da capa',
          type: 'text',
          admin: {
            description: 'Imagem principal da home e compartilhamento.',
          },
        },
        {
          name: 'cover',
          label: 'Capa (upload)',
          type: 'upload',
          relationTo: 'media',
        },
        {
          name: 'primaryColor',
          label: 'Cor primária',
          type: 'text',
          defaultValue: '#0f172a',
          validate: validateHexColor,
        },
        {
          name: 'accentColor',
          label: 'Cor de destaque',
          type: 'text',
          defaultValue: '#f59e0b',
          validate: validateHexColor,
        },
        {
          name: 'theme',
          label: 'Tema',
          type: 'select',
          defaultValue: 'light',
          options: themeOptions,
        },
        {
          name: 'tagline',
          label: 'Assinatura curta',
          type: 'text',
          maxLength: 160,
        },
      ],
    },
    {
      name: 'contact',
      label: 'Contato',
      type: 'group',
      fields: [
        { name: 'email', label: 'E-mail', type: 'email' },
        { name: 'phone', label: 'Telefone', type: 'text' },
        { name: 'whatsapp', label: 'WhatsApp', type: 'text' },
        { name: 'addressLine', label: 'Endereço', type: 'text' },
        { name: 'city', label: 'Cidade', type: 'text' },
        { name: 'state', label: 'UF', type: 'text' },
        { name: 'creci', label: 'CRECI', type: 'text' },
      ],
    },
  ],
}

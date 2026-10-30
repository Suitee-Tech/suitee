-- tenant_financial_settings — popular admin_fee_pct = 10.0 em tenants
-- existentes que ainda não tenham a chave em metadata.
--
-- Background: o cálculo do Transfer mensal ao proprietário (RENT/SALE)
-- precisa do AdminFeePct para descontar a taxa de administração do
-- gross_rent antes do net_amount repassado. Até a FASE D, o pipeline
-- usava commission_pct do contrato como proxy de admin_fee no event
-- handler — um bug semântico que confundia commission de broker com
-- taxa adm de imobiliária. A FASE D separa: contract.commission_pct
-- segue exclusivo de comissão de broker; tenant_financial_settings
-- .admin_fee_pct passa a alimentar o Transfer mensal.
--
-- Default 10% alinhado ao mercado brasileiro. Cada tenant ajusta via
-- PUT /financial-settings depois.
--
-- Domain: toca-financial/internal/domain/tenant_financial_settings/settings.go
--         (Settings.AdminFeePct + DefaultAdminFeePct = 10.0 no New())

UPDATE financial.tenant_financial_settings
   SET metadata = jsonb_set(metadata, '{admin_fee_pct}', '10.0'::jsonb, true),
       updated_at = now()
 WHERE deleted_at IS NULL
   AND NOT (metadata ? 'admin_fee_pct');

-- Baseline append-only para o gate de migrations do deploy direto.
-- Cada versao Supabase aplicada precisa conservar o SHA-256 do arquivo
-- revisado. O gate usa uma credencial exclusiva de migration; a aplicacao nao
-- recebe acesso ao ledger. O trigger rejeita mutacao acidental pelo owner.
CREATE TABLE ops.deployment_migration_checksums (
    version         TEXT PRIMARY KEY CHECK (version ~ '^[0-9]{14}$'),
    sha256          TEXT NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    migration_name  TEXT NOT NULL
        CHECK (migration_name ~ ('^' || version || '_[A-Za-z0-9][A-Za-z0-9_-]*[.]sql$')),
    provenance      TEXT NOT NULL CHECK (provenance IN ('bootstrap', 'gate')),
    release_tag     TEXT NOT NULL,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (
        (provenance = 'bootstrap' AND release_tag = '20260721190000')
        OR
        (provenance = 'gate' AND release_tag ~ '^([0-9a-f]{40}|[0-9a-f]{64})$')
    )
);

ALTER TABLE ops.deployment_migration_checksums ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE ops.deployment_migration_checksums
    FROM PUBLIC, anon, authenticated, service_role;

CREATE FUNCTION ops.reject_deployment_migration_checksum_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = ''
AS $$
BEGIN
    RAISE EXCEPTION 'ops.deployment_migration_checksums is append-only';
    RETURN NULL;
END;
$$;

REVOKE ALL ON FUNCTION ops.reject_deployment_migration_checksum_mutation()
    FROM PUBLIC, anon, authenticated, service_role;

CREATE TRIGGER deployment_migration_checksums_append_only
BEFORE UPDATE OR DELETE OR TRUNCATE
ON ops.deployment_migration_checksums
FOR EACH STATEMENT
EXECUTE FUNCTION ops.reject_deployment_migration_checksum_mutation();

COMMENT ON TABLE ops.deployment_migration_checksums IS
    'Ledger append-only do gate; trigger rejeita UPDATE, DELETE e TRUNCATE, e o gate detecta drift.';

INSERT INTO ops.deployment_migration_checksums
    (version, sha256, migration_name, provenance, release_tag)
VALUES
    ('20231026000000', 'ea252c7894a7711d28ca8694abab42bf8551e28e4fcfc87dd0ec11084176127e', '20231026000000_init_jsonb_pilot.sql', 'bootstrap', '20260721190000'),
    ('20260421120000', 'ba96e25dac6e8864a36bcdb8b4a85c21c9055720ee68691c26ef1ab0770804df', '20260421120000_finalize_jsonb_rls.sql', 'bootstrap', '20260721190000'),
    ('20260422170000', '8824cf7067fcb8fb1d9608e84d791d4374f0e122824177346fb134191a0da087', '20260422170000_create_comm_tables.sql', 'bootstrap', '20260721190000'),
    ('20260424120000', 'e122d3b4cd114501390fc75478d25ac69dd98470a44aac9b7a1c8c527a87082e', '20260424120000_create_crm_roulette_audit.sql', 'bootstrap', '20260721190000'),
    ('20260425120000', '8582a03e884de9d9c5ca4fb4f5f508e84f9d8b3669e5cdde74a0bfe4543f3f8d', '20260425120000_create_ai_tables.sql', 'bootstrap', '20260721190000'),
    ('20260426120000', '63b08b848842ab5515d30340ad22682ef7072f6ea2d874b504ed0d255ca12d07', '20260426120000_create_ai_tool_invocations.sql', 'bootstrap', '20260721190000'),
    ('20260429120000', '2e0e46a9e2953650ffea1509cd0ce643239d3d8b24d827e7ac814d55c070ba8f', '20260429120000_search_indexes.sql', 'bootstrap', '20260721190000'),
    ('20260510120000', 'c3d01167ae6754f13195ea7a915c149343b677a9dc1a2ec30cf7f0d6d2dfc89b', '20260510120000_charge_deal_lead_indexes.sql', 'bootstrap', '20260721190000'),
    ('20260510120100', 'd77fc3dcd5fdcbcbcf87fec38dd0890a738d52900dcb6e38c95191e8d89ac055', '20260510120100_maintenance_rental_lead_indexes.sql', 'bootstrap', '20260721190000'),
    ('20260510120200', '686706017f327a919d58cd56a0633444cf092428b402e60ddb8e1cefb0b7e07c', '20260510120200_tenant_commission_rules.sql', 'bootstrap', '20260721190000'),
    ('20260510120300', '8b33f0b33a8603281ed1a491aaa52731e46991b0425b1137629c289773fb83aa', '20260510120300_create_financial_rentals.sql', 'bootstrap', '20260721190000'),
    ('20260510120400', '0374176aa38cfa4845db813f3e27fa27b5ba813affa845845bd1e51a52592f81', '20260510120400_create_signature_schema.sql', 'bootstrap', '20260721190000'),
    ('20260513120000', '9b7197faf07836346caa99e57c51f92c6e1d5a5099cd746de9c707be5e5b0f23', '20260513120000_create_financial_utility_accounts.sql', 'bootstrap', '20260721190000'),
    ('20260513120100', 'e36d0a77c0c3c485fd47ddb5ae69c5b80b6e3612838893003060e8ae5b9248b4', '20260513120100_create_financial_reminder_rules.sql', 'bootstrap', '20260721190000'),
    ('20260514120000', 'b44a4c588aa68aecd0148ee9adff8f4aae12d35f5382773d078bf4bac62e0854', '20260514120000_rls_signature_webhook_events.sql', 'bootstrap', '20260721190000'),
    ('20260514120100', 'bd3de1cac96fd865cb9505a6546be273a698426a88aa1722238391b497eb5c8f', '20260514120100_outbox_backoff_and_status.sql', 'bootstrap', '20260721190000'),
    ('20260515120000', '3503c91b00a98bad5e8b0b99162fdf15d6eb35cac12e8d1f19c084282b076e3c', '20260515120000_create_comm_sent_externals.sql', 'bootstrap', '20260721190000'),
    ('20260515120100', '845e5e5792355b69ded622f876e216956b4d49fe3a8d8fef739cefc3c6075297', '20260515120100_prevent_dangling_lead_deletes.sql', 'bootstrap', '20260721190000'),
    ('20260515120200', 'e55de3f8568b0451ef57caa5f951db3a43a734e19a42ebc4995140482d2cc63e', '20260515120200_create_audit_schema.sql', 'bootstrap', '20260721190000'),
    ('20260517120000', '52856b09746d7bd6a8307a7c0b526d4338b3b0ad4331513aaedbaae1d3e34360', '20260517120000_lead_updated_seq.sql', 'bootstrap', '20260721190000'),
    ('20260517120100', 'ea3e48962a8a7f07df78906e78e2dd634bf1afdc320120125742008d1b2e1fa3', '20260517120100_lead_phone_quarantine.sql', 'bootstrap', '20260721190000'),
    ('20260518120000', '866c0064e6a4a4dff4465bc2e481ae37be7d119e97df5e2e92e5b04a75881047', '20260518120000_charges_source_event_id_idx.sql', 'bootstrap', '20260721190000'),
    ('20260519130000', '4dc9acd1423ce43da46e7e7ce5f5c4d193b69da1730da3c60c2b586fcc3d5496', '20260519130000_export_jobs.sql', 'bootstrap', '20260721190000'),
    ('20260520120000', '8fb7173ba5d4c002edc293343e4db9545532561d5cef23c5691837fe5aaa54ed', '20260520120000_signature_envelope_kind.sql', 'bootstrap', '20260721190000'),
    ('20260521120000', 'a0ee7870fd0fdf2d6a0d992c765ba74523a28579b9b46e15aad6e5b50b15e969', '20260521120000_partnerships_soft_delete_columns.sql', 'bootstrap', '20260721190000'),
    ('20260522120000', 'c04bdbc3e32afa6ef81d9a971cb000926a37c4692db950ea4f2e625a0843f5ca', '20260522120000_create_abacate_webhook_events.sql', 'bootstrap', '20260721190000'),
    ('20260522120001', '305e7b422c008a679b58c49c87ecadad3a9e2b9072b50887ad623a13c0366d15', '20260522120001_partner_royalties.sql', 'bootstrap', '20260721190000'),
    ('20260522120100', 'ae518d239ce1ba0a429ecb67f303acd629b206c26815264b7901651cd9ca2305', '20260522120100_payments_v1_freeze.sql', 'bootstrap', '20260721190000'),
    ('20260522120200', 'a73c6132476c07cd702783e57f7aa0da981060054452a0abb04057af89d833a4', '20260522120200_charges_abacate_checkout_idx.sql', 'bootstrap', '20260721190000'),
    ('20260522120300', 'b45e25e1ec564ac6f5b2533d8bcc970b6f05e0e8192fbb9382cacae152ea426e', '20260522120300_create_withdrawals.sql', 'bootstrap', '20260721190000'),
    ('20260522120600', '8108a27dbb016d1840953f4a7c8e15e6aa10a16a856b99592cfd95e548fa88bc', '20260522120600_create_platform_subscriptions.sql', 'bootstrap', '20260721190000'),
    ('20260526120000', '829919a797783dfe0409222795d0a96fc0caa654b9532c18efc7912b0ea78d43', '20260526120000_create_sale_billing_plan.sql', 'bootstrap', '20260721190000'),
    ('20260526120100', '3507893fe56ca54bfa0d96a450acfde5d61b77c550ab69e18fc49aa9d7b97580', '20260526120100_create_payout_orders.sql', 'bootstrap', '20260721190000'),
    ('20260527120000', '0b9f666efed6f1969e980b63822eeafc9ec24da9416960050c3e303a2103c3f5', '20260527120000_create_tenant_financial_settings.sql', 'bootstrap', '20260721190000'),
    ('20260527120100', '3630accb3408f5ccd4729e6064275771401626228f611ad7311a1b677f1d8756', '20260527120100_commission_role_deductions_indexes.sql', 'bootstrap', '20260721190000'),
    ('20260527120200', 'd6fa9820b8ec0a8766eab435f908a95f185a479f1380897a2f045ea5984ec045', '20260527120200_charges_last_reconciled_at.sql', 'bootstrap', '20260721190000'),
    ('20260528120000', '7e35c8d06bc6b5de1afdbd26d122772c206de58d799fda2a29cdc72747199d58', '20260528120000_create_sale_billing_plan_drafts.sql', 'bootstrap', '20260721190000'),
    ('20260528120100', 'ed373cd66ad789455c8c1ae02eb637b12b56d6e52db26c832361eeb296af4eed', '20260528120100_transfers_monthly_unique.sql', 'bootstrap', '20260721190000'),
    ('20260528120200', '6d240424922e0b66b10628768c6fee598485cf450cb74b00804fb41e97f16213', '20260528120200_tenant_settings_admin_fee.sql', 'bootstrap', '20260721190000'),
    ('20260528130000', 'd31a9566b8fcedb7fe88a61add3bcfc551eb9351346e55f6fea6b6374b5c8ad4', '20260528130000_drop_abacate_gateway_artifacts.sql', 'bootstrap', '20260721190000'),
    ('20260529120000', '85a99e0068d0fc03f593ceca06a029abc0fef0a75e04dff4ba2ba7b27017e93f', '20260529120000_processed_events_consumer_pk.sql', 'bootstrap', '20260721190000'),
    ('20260529121000', 'ac5f81e35a25d72002b81b9cf4025b56673e89b877534035475c084fa89b7b91', '20260529121000_transactions_source_event_id_unique.sql', 'bootstrap', '20260721190000'),
    ('20260529121100', '3bbaeffdbd525ffe9efedcc0964e2026d1e66b8febf30c6628b88b6fa306d9f7', '20260529121100_commissions_source_event_id_unique.sql', 'bootstrap', '20260721190000'),
    ('20260529121200', '33c280e1d064cc1c8a5be43d1fd894e18f7e1959dc41317141d21b5538083074', '20260529121200_payout_orders_source_event_id_unique.sql', 'bootstrap', '20260721190000'),
    ('20260529121300', 'c971cb5c4fb5f191ce4923d611851b6e639296d6ee62b0cdb425ee36529d2a57', '20260529121300_partner_royalties_source_event_id_unique.sql', 'bootstrap', '20260721190000'),
    ('20260529160000', '5ac1c4a22998baf94f63a26823952b5739838489d95d1cd3a56a886cdedd00e2', '20260529160000_withdrawals_external_id_unique.sql', 'bootstrap', '20260721190000'),
    ('20260606120000', 'f5653dff6b6e316c9a8479e36c1e44ecdab397fc6ff3366d16af8c2aa88991cf', '20260606120000_dlq_events_replayed_at.sql', 'bootstrap', '20260721190000'),
    ('20260718052612', '51329ae42feac3dd45cd33b3cf1f51fe092cda797b016fe43a6232c3e249a00f', '20260718052612_tenant_approval_policy.sql', 'bootstrap', '20260721190000'),
    ('20260718052718', 'c36e0f182db117d3e700d6de4200d0aa53131b3a7f73f570612b3f8d3e7d11b2', '20260718052718_fix_tenant_settings_rls_policies.sql', 'bootstrap', '20260721190000'),
    ('20260718150531', 'c91332ce9057863d94366d5bc8830bd1269292aea83bd2e8d236829d6b255954', '20260718150531_retrofit_tenant_rls_empty_tables.sql', 'bootstrap', '20260721190000'),
    ('20260718150558', 'b45692f578c577082c580524d5209d1324ad93bcba35901f398c5879c4944667', '20260718150558_retrofit_tenant_rls_small_tables.sql', 'bootstrap', '20260721190000'),
    ('20260718150627', '45cd69110a984e8d98e025b462b327e3e0eadd4a0aed60aff241cd61419a1d0f', '20260718150627_retrofit_tenant_rls_large_tables.sql', 'bootstrap', '20260721190000'),
    ('20260718150656', '1fffa57c39524e73485009d376d24255d63d958fa8023421118fa7bf6a9b9042', '20260718150656_retrofit_auxiliary_tenant_rls.sql', 'bootstrap', '20260721190000'),
    ('20260718150712', 'c7b6f3db3c27a4af72b998a97efdbe80e9d61ea14af7a391afa10cc1ce5335cb', '20260718150712_fix_function_search_paths.sql', 'bootstrap', '20260721190000'),
    ('20260718150751', '379cb9bbc368b1f807d342ea8331a78eb330b2a8c166d64beeef79591dc1b0c9', '20260718150751_reconcile_20260522_history_collision.sql', 'bootstrap', '20260721190000'),
    ('20260718152529', '615f61d96d2702bf404d0c8649a51ffa41d7be1d52b972413cf57ffca21da65a', '20260718152529_maintain_audit_event_partitions.sql', 'bootstrap', '20260721190000'),
    ('20260718154005', '072fc0e88de16d55bbfbee2c8d226ae1baf516592ce712afeaa4e42fffbb81eb', '20260718154005_optimize_tenant_rls_policies.sql', 'bootstrap', '20260721190000');

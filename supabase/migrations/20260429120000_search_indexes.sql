CREATE INDEX IF NOT EXISTS idx_brokers_name_lower ON crm.brokers ((LOWER(metadata->>'Name')) text_pattern_ops) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_brokers_email_lower ON crm.brokers ((LOWER(metadata->>'Email')) text_pattern_ops) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_brokers_active ON crm.brokers ((metadata->>'Active')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_brokers_specialties_gin ON crm.brokers USING GIN ((metadata->'Specialties')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_brokers_regions_gin ON crm.brokers USING GIN ((metadata->'Regions')) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_leads_name_lower ON crm.leads ((LOWER(metadata->>'Name')) text_pattern_ops) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_email_lower ON crm.leads ((LOWER(metadata->>'Email')) text_pattern_ops) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_phone ON crm.leads ((metadata->>'Phone')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_display_id ON crm.leads ((metadata->>'DisplayID')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_account_id ON crm.leads ((metadata->>'AccountID')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_status ON crm.leads ((metadata->>'Status')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_role ON crm.leads ((metadata->>'Role')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_is_client ON crm.leads ((metadata->>'IsClient')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_broker ON crm.leads ((metadata->>'AssignedBrokerID')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_source ON crm.leads ((metadata->>'Source')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_last_interaction ON crm.leads ((metadata->>'LastInteraction')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_expires_at ON crm.leads ((metadata->>'ExpiresAt')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_assigned_at ON crm.leads ((metadata->>'AssignedAt')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_captacao ON crm.leads ((metadata->>'CaptacaoPropertyID')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_property_ids_gin ON crm.leads USING GIN ((metadata->'PropertyIDs')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_interest_type ON crm.leads ((metadata#>>'{PropertyInterest,type}')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_interest_type_legacy ON crm.leads ((metadata#>>'{PropertyInterest,Type}')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_interest_location ON crm.leads ((LOWER(metadata#>>'{PropertyInterest,preferred_location}')) text_pattern_ops) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_interest_location_legacy ON crm.leads ((LOWER(metadata#>>'{PropertyInterest,PreferredLocation}')) text_pattern_ops) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_interest_price_max ON crm.leads (((metadata#>>'{PropertyInterest,price_range,max}')::numeric)) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_interest_price_max_legacy ON crm.leads (((metadata#>>'{PropertyInterest,PriceRange,Max}')::numeric)) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_leads_history_gin ON crm.leads USING GIN ((metadata->'History')) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_deals_status ON deal.deals ((metadata->>'status')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_deals_type ON deal.deals ((metadata->>'type')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_deals_value ON deal.deals (((metadata->>'value')::numeric)) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_deals_broker ON deal.deals ((metadata->>'broker_id')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_deals_property ON deal.deals ((metadata->>'property_id')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_deals_signers_gin ON deal.deals USING GIN ((metadata->'signers')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_deals_parties_gin ON deal.deals USING GIN ((metadata->'parties')) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_properties_title_lower ON core.properties ((LOWER(metadata->>'title')) text_pattern_ops) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_neighborhood ON core.properties ((LOWER(metadata#>>'{address,neighborhood}')) text_pattern_ops) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_city ON core.properties ((LOWER(metadata#>>'{address,city}')) text_pattern_ops) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_status ON core.properties ((metadata->>'status')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_listing_type ON core.properties ((metadata->>'listing_type')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_owners_gin ON core.properties USING GIN ((metadata->'owners')) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_charges_status ON financial.charges ((metadata->>'status')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_charges_customer ON financial.charges ((metadata->>'customer_id')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_charges_customer_party ON financial.charges ((metadata->>'tenant_party_id')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_charges_deal ON financial.charges ((metadata->>'deal_id')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_charges_due_date ON financial.charges ((metadata->>'due_date')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_charges_amount ON financial.charges (((metadata->>'amount')::numeric)) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_charges_total_amount ON financial.charges (((metadata->>'total_amount')::numeric)) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_appointments_agent ON calendar.appointments ((metadata->>'agent_id')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_appointments_lead ON calendar.appointments ((metadata->>'lead_id')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_appointments_property ON calendar.appointments ((metadata->>'property_id')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_appointments_status ON calendar.appointments ((metadata->>'status')) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_appointments_date ON calendar.appointments ((metadata->>'date')) WHERE deleted_at IS NULL;

-- Synthetic draft and parties, only in the script-owned Supabase database.
INSERT INTO core.properties (id,tenant_id,title,status,type,metadata) VALUES
 ('11111111-1111-4111-8111-111111111104','11111111-1111-4111-8111-111111111101','Imóvel QA','AVAILABLE','APARTMENT',
 '{"id":"11111111-1111-4111-8111-111111111104","tenant_id":"11111111-1111-4111-8111-111111111101"}');
INSERT INTO deal.deals (id,tenant_id,metadata) VALUES
 ('33333333-3333-4333-8333-333333333301','11111111-1111-4111-8111-111111111101',
 '{"id":"33333333-3333-4333-8333-333333333301","tenant_id":"11111111-1111-4111-8111-111111111101",
 "type":"RENT","status":"CONTRACT","value":2000,"commission":100,
 "property_id":"11111111-1111-4111-8111-111111111104","customer_id":"11111111-1111-4111-8111-111111111103",
 "owner_id":"33333333-3333-4333-8333-333333333302","broker_id":"11111111-1111-4111-8111-111111111105",
 "broker_participations":[{"broker_id":"11111111-1111-4111-8111-111111111105","role":"fechamento","percentage":100}]}'::jsonb);
INSERT INTO deal.contracts (id,tenant_id,deal_id,metadata) VALUES
 ('33333333-3333-4333-8333-333333333303','11111111-1111-4111-8111-111111111101','33333333-3333-4333-8333-333333333301',
 jsonb_build_object('id','33333333-3333-4333-8333-333333333303','tenant_id','11111111-1111-4111-8111-111111111101',
 'deal_id','33333333-3333-4333-8333-333333333301','status','DRAFT','type','LOCACAO','value',2000,
 'start_date',to_char(now()+interval '1 day','YYYY-MM-DD"T"00:00:00"Z"'),
 'end_date',to_char(now()+interval '2 months','YYYY-MM-DD"T"00:00:00"Z"'),
 'signatories',jsonb_build_array(jsonb_build_object('id','33333333-3333-4333-8333-333333333304',
 'name','Signatário QA','email','signer@example.invalid','role','inquilino','status','PENDING'))));

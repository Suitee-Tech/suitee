-- Only consumed by scripts/test-stack.sh, in its disposable Compose project.
DO $$ BEGIN
  IF current_database() <> 'toca_test' THEN
    RAISE EXCEPTION 'refusing to seed a non-test database';
  END IF;
END $$;

INSERT INTO crm.leads (id, tenant_id, metadata)
VALUES ('11111111-1111-4111-8111-111111111103', '11111111-1111-4111-8111-111111111101',
  '{"ID":"11111111-1111-4111-8111-111111111103",
    "TenantID":"11111111-1111-4111-8111-111111111101",
    "AccountID":"11111111-1111-4111-8111-111111111102",
    "Name":"Cliente de teste", "Email":"client@example.invalid", "Phone":"11900000000",
    "Source":"MANUAL", "Status":"QUALIFIED", "IsClient":true,
    "AssignedBrokerID":"11111111-1111-4111-8111-111111111105",
    "recommended_property_id":"11111111-1111-4111-8111-111111111104",
    "recommended_property_snapshot":{"id":"11111111-1111-4111-8111-111111111104", "title":"Imóvel de teste"}}'::jsonb);

-- P1-08: imóvel + proprietário para as jornadas deal-pipeline (esteira).
INSERT INTO core.properties (id, tenant_id, title, price, status, type, metadata)
VALUES ('11111111-1111-4111-8111-111111111104',
        '11111111-1111-4111-8111-111111111101',
        'Imóvel de teste',
        2500,
        'in_portfolio',
        'rent',
        '{"id":"11111111-1111-4111-8111-111111111104","title":"Imóvel de teste",
          "status":"in_portfolio","publication_status":"published",
          "property_type":"apartment","listing_type":"rent",
          "address":{"street":"Rua do Teste","number":"100","neighborhood":"Centro","city":"São Paulo","state":"SP"},
          "pricing":{"rent_price":{"current_amount":2500}},
          "features":{"bedrooms":2,"bathrooms":1,"parking_spaces":1},
          "areas":{"total_area_m2":70,"usable_area_m2":60},
          "media":{"images":[{"url":"https://img.test.example.invalid/p1.jpg","description":"fachada"}]},
          "description":"Imóvel de teste da esteira isolada"}'::jsonb);

INSERT INTO crm.leads (id, tenant_id, metadata)
VALUES ('11111111-1111-4111-8111-111111111110', '11111111-1111-4111-8111-111111111101',
  '{"ID":"11111111-1111-4111-8111-111111111110",
    "TenantID":"11111111-1111-4111-8111-111111111101",
    "Name":"Proprietário de teste", "Email":"owner@example.invalid", "Phone":"11900000001",
    "Source":"MANUAL", "Status":"QUALIFIED", "IsClient":false}'::jsonb);

-- P1-08: corretor real para a roleta (eligibilidade exige crm.brokers).
INSERT INTO crm.brokers (id, tenant_id, metadata)
VALUES ('11111111-1111-4111-8111-111111111105',
        '11111111-1111-4111-8111-111111111101',
        '{"_id":"11111111-1111-4111-8111-111111111105",
          "tenant_id":"11111111-1111-4111-8111-111111111101",
          "name":"Corretor de teste", "email":"agent@example.invalid",
          "role":"corretor", "active":true, "paused":false, "weight":1}'::jsonb);

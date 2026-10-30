-- Comprovantes financeiros sao gravados exclusivamente pelo servico
-- financeiro usando a service role. O bucket permanece privado; o acesso
-- do navegador ocorre somente por URL assinada de curta duracao.
DO $$
BEGIN
  IF to_regclass('storage.buckets') IS NULL THEN
    RETURN;
  END IF;

  INSERT INTO storage.buckets (id, name, public)
  VALUES ('financial-proofs', 'financial-proofs', false)
  ON CONFLICT (id) DO UPDATE
  SET public = false;
END $$;

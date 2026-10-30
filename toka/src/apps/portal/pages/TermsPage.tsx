import { LegalDocumentPage } from "../../../shared/components/LegalDocumentPage";
import { CURRENT_TERMS_DOCUMENT_VERSION, TERMS_OF_USE_URL } from "../../../shared/config/lgpd";

export function TermsPage() {
  return <LegalDocumentPage title="Termos de Uso" documentUrl={TERMS_OF_USE_URL} version={CURRENT_TERMS_DOCUMENT_VERSION} />;
}

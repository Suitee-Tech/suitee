import { LegalDocumentPage } from "../../../shared/components/LegalDocumentPage";
import { CURRENT_PRIVACY_VERSION, PRIVACY_POLICY_URL } from "../../../shared/config/lgpd";

export function PrivacyPage() {
  return (
    <LegalDocumentPage title="Política de Privacidade" documentUrl={PRIVACY_POLICY_URL} version={CURRENT_PRIVACY_VERSION} />
  );
}

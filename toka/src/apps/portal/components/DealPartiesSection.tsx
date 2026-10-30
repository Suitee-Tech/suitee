import { CheckCircle2, Clock, Mail, Phone, Shield, ShieldCheck, ShieldOff, User, XCircle } from "lucide-react";
import { clsx } from "clsx";
import type { PartyDTO, Signer } from "../../../shared/services/deal.service";
import { PortalSection } from "./PortalState";
import { SignatureStatusBadge } from "../../../shared/components/signature/SignatureStatusBadge";
import { portalRoleLabel } from "../lib/portalPersona";

function maskDocument(doc?: string): string | null {
  if (!doc) return null;
  const digits = doc.replace(/\D/g, "");
  if (digits.length === 11) {
    return `***.${digits.slice(3, 6)}.${digits.slice(6, 9)}-**`;
  }
  if (digits.length === 14) {
    return `**.${digits.slice(2, 5)}.${digits.slice(5, 8)}/****-**`;
  }
  return doc;
}

function isPartySignatureRequired(party: PartyDTO): boolean {
  return party.signature_required ?? Boolean(party.is_primary);
}

function isSignerRequired(signer: Signer): boolean {
  return signer.is_required ?? true;
}

function PartyStatusPill({ status }: { status?: PartyDTO["sign_status"] | string }) {
  if (!status) return null;
  return <SignatureStatusBadge kind="signer" status={status as PartyDTO["sign_status"]} size="sm" />;
}

export interface SignatureSnapshot {
  envelope_status?: string;
  my_signer?: { name?: string; status?: string; signed_at?: string };
  other_signers?: Array<{ name?: string; status?: string; signed_at?: string }>;
}

function RequiredSignaturePill({ required }: { required: boolean }) {
  if (required) {
    return (
      <span className="inline-flex items-center gap-1 text-micro font-bold px-2 py-0.5 rounded-full bg-status-critical-soft text-status-critical border border-status-critical/20">
        <ShieldCheck className="w-3 h-3" aria-hidden="true" />
        Assinatura obrigatória
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 text-micro font-bold px-2 py-0.5 rounded-full bg-muted text-muted-foreground border border-border">
      <ShieldOff className="w-3 h-3" aria-hidden="true" />
      Opcional
    </span>
  );
}

interface DealPartiesSectionProps {
  parties?: PartyDTO[];
  signers?: Signer[];
  myPartyId?: string;
  myEmail?: string;
  signatureSnapshot?: SignatureSnapshot;
}

function effectiveSignStatusForParty(
  party: PartyDTO,
  snapshot: SignatureSnapshot | undefined,
  myEmail: string | undefined,
): PartyDTO["sign_status"] | undefined {
  const envStatus = (snapshot?.envelope_status ?? "").toUpperCase();
  const partyEmail = party.email?.toLowerCase();
  const matchesMe = !!partyEmail && !!myEmail && partyEmail === myEmail.toLowerCase();
  if (snapshot) {
    const fromSnapshot = matchesMe ? snapshot.my_signer : undefined;
    const fromOthers =
      (!matchesMe || !fromSnapshot)
        ? snapshot.other_signers?.find(
            (s) => (s.name ?? "").trim().toLowerCase() === (party.name ?? "").trim().toLowerCase(),
          )
        : undefined;
    const candidate = (fromSnapshot ?? fromOthers)?.status?.toUpperCase();
    if (candidate === "SIGNED" || candidate === "REJECTED" || candidate === "REFUSED") {
      return (candidate === "REFUSED" ? "REJECTED" : candidate) as PartyDTO["sign_status"];
    }
  }
  if (envStatus === "FINISHED") {
    return "SIGNED";
  }
  return party.sign_status;
}

export function DealPartiesSection({ parties, signers, myPartyId, myEmail, signatureSnapshot }: DealPartiesSectionProps) {
  if (!parties || parties.length === 0) {
    if (import.meta.env.DEV && parties === undefined) {
      console.warn("[portal] DealPartiesSection received undefined parties; check ClientPortalDealDTO");
    }
    return null;
  }

  return (
    <PortalSection title="Partes envolvidas" subtitle="Quem está envolvido nesta transação">
      <ul className="space-y-3">
        {parties.map((party) => {
          const isMe = (myPartyId && party.id === myPartyId) || (myEmail && party.email && party.email === myEmail);
          const required = isPartySignatureRequired(party);
          const document = maskDocument(party.document);
          const associatedSigner = signers?.find((s) => s.email === party.email);
          return (
            <li
              key={party.id}
              className={clsx(
                "rounded-xl border p-3 flex flex-col gap-2",
                isMe ? "border-status-ok/30 bg-status-ok-soft/50" : "border-border bg-card",
              )}
            >
              <div className="flex items-start gap-3">
                <div className="w-9 h-9 rounded-full bg-muted flex items-center justify-center text-muted-foreground flex-shrink-0">
                  <User className="w-4 h-4" aria-hidden="true" />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 flex-wrap">
                    <p className="text-sm font-bold text-foreground truncate">{party.name}</p>
                    {isMe && (
                      <span className="text-micro font-bold px-2 py-0.5 rounded-full bg-status-ok text-status-ok-foreground">Você</span>
                    )}
                  </div>
                  <p className="text-xs text-muted-foreground">{portalRoleLabel(party.role)}</p>
                  {document && <p className="text-caption text-muted-foreground/80 mt-0.5">{document}</p>}
                  <div className="flex items-center gap-3 mt-1 text-caption text-muted-foreground flex-wrap">
                    {party.email && (
                      <span className="inline-flex items-center gap-1">
                        <Mail className="w-3 h-3" aria-hidden="true" />
                        <span className="truncate max-w-[180px]">{party.email}</span>
                      </span>
                    )}
                    {party.phone && (
                      <span className="inline-flex items-center gap-1">
                        <Phone className="w-3 h-3" aria-hidden="true" />
                        {party.phone}
                      </span>
                    )}
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-2 flex-wrap pl-12">
                <RequiredSignaturePill required={required} />
                <PartyStatusPill status={effectiveSignStatusForParty(party, signatureSnapshot, myEmail)} />
                {associatedSigner && associatedSigner.is_required === false && (
                  <span className="inline-flex items-center gap-1 text-micro font-bold px-2 py-0.5 rounded-full bg-muted text-muted-foreground border border-border">
                    <Shield className="w-3 h-3" aria-hidden="true" />
                    Signatário opcional
                  </span>
                )}
              </div>
            </li>
          );
        })}
      </ul>
    </PortalSection>
  );
}

interface DealSignersListProps {
  signers?: Signer[];
  myEmail?: string;
  signatureSnapshot?: SignatureSnapshot;
}

export function DealSignersList({ signers, myEmail, signatureSnapshot }: DealSignersListProps) {
  if (!signers || signers.length === 0) return null;
  const others = signers.filter((s) => s.email !== myEmail);
  if (others.length === 0) return null;
  const envFinished = (signatureSnapshot?.envelope_status ?? "").toUpperCase() === "FINISHED";
  const snapshotByName = new Map<string, { status?: string }>();
  for (const o of signatureSnapshot?.other_signers ?? []) {
    if (o.name) snapshotByName.set(o.name.trim().toLowerCase(), o);
  }
  return (
    <div>
      <h4 className="text-caption font-bold text-muted-foreground uppercase tracking-wider mb-2 ml-1">Outros Assinantes</h4>
      <div className="space-y-2">
        {others.map((s) => {
          const required = isSignerRequired(s);
          const fromSnapshot = snapshotByName.get((s.name ?? "").trim().toLowerCase())?.status?.toUpperCase();
          const effectiveStatus = envFinished ? "SIGNED" : fromSnapshot ?? s.status;
          return (
            <div key={s.id} className="flex items-center justify-between bg-card border border-border rounded-lg p-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <p className="text-label font-bold text-foreground truncate">{s.name}</p>
                  <RequiredSignaturePill required={required} />
                </div>
                <p className="text-caption text-muted-foreground capitalize">{s.role}</p>
              </div>
              {effectiveStatus === "SIGNED" && <CheckCircle2 className="w-4 h-4 text-status-ok flex-shrink-0" aria-label="Assinado" />}
              {effectiveStatus === "PENDING" && <Clock className="w-4 h-4 text-status-pending flex-shrink-0" aria-label="Pendente" />}
              {(effectiveStatus === "REJECTED" || effectiveStatus === "REFUSED") && (
                <XCircle className="w-4 h-4 text-status-critical flex-shrink-0" aria-label="Rejeitado" />
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}


package clicksign

import "testing"

// TestSignedContractURL_PicksContractDocByFilename mirrors the real Clicksign
// v3 GET /api/v3/envelopes/:id/documents response: the signed-PDF URL lives
// under links.files.signed (NOT in attributes), and the attachment can appear
// before the contract in the array.
func TestSignedContractURL_PicksContractDocByFilename(t *testing.T) {
	const contractID = "a14fe2f4-52d5-41ac-b43c-343790fde8a2"
	docs := []DocumentResource{
		{ // attachment — Clicksign returned it first
			ID:         "735b8a70-d47e-4001-93d3-21b3671d6557",
			Type:       "documents",
			Attributes: DocumentAttributes{Filename: "35503082261343036000162.pdf"},
			Links:      DocumentLinks{Files: DocumentLinkFiles{Signed: "https://s3/anexo-signed.pdf"}},
		},
		{ // the contract — second in the array
			ID:         "7a122ddc-00d9-43d0-89b4-6736b89d66f0",
			Type:       "documents",
			Attributes: DocumentAttributes{Filename: "contrato-" + contractID + ".pdf"},
			Links:      DocumentLinks{Files: DocumentLinkFiles{Signed: "https://s3/contrato-signed.pdf"}},
		},
	}
	if got := SignedContractURL(docs, contractID); got != "https://s3/contrato-signed.pdf" {
		t.Errorf("SignedContractURL = %q, want the contract document's signed URL", got)
	}
}

// TestSignedContractURL_FallsBackToFirstSigned: a single-document envelope has
// no attachment, so the only document is the contract and the filename match
// falls through to "first document with a signed URL".
func TestSignedContractURL_FallsBackToFirstSigned(t *testing.T) {
	docs := []DocumentResource{{
		ID:         "doc-1",
		Attributes: DocumentAttributes{Filename: "qualquer-nome.pdf"},
		Links:      DocumentLinks{Files: DocumentLinkFiles{Signed: "https://s3/only-signed.pdf"}},
	}}
	if got := SignedContractURL(docs, "unmatched-contract-id"); got != "https://s3/only-signed.pdf" {
		t.Errorf("SignedContractURL = %q, want fallback to the first signed URL", got)
	}
}

// TestSignedContractURL_EmptyWhenNoSignedFile: an envelope still being signed
// only exposes the original (unsigned) upload — that must never be returned
// as the signed contract.
func TestSignedContractURL_EmptyWhenNoSignedFile(t *testing.T) {
	docs := []DocumentResource{{
		ID:         "doc-1",
		Attributes: DocumentAttributes{Filename: "contrato-x.pdf"},
		Links:      DocumentLinks{Files: DocumentLinkFiles{Original: "https://s3/original.pdf"}},
	}}
	if got := SignedContractURL(docs, "x"); got != "" {
		t.Errorf("SignedContractURL = %q, want empty (must not return the unsigned original)", got)
	}
}

// TestDocumentResource_SignedURL: SignedURL reads links.files.signed (the v3
// location) and the legacy attributes.signed_file_url fallback.
func TestDocumentResource_SignedURL(t *testing.T) {
	fromLinks := DocumentResource{Links: DocumentLinks{Files: DocumentLinkFiles{Signed: "https://s3/from-links.pdf"}}}
	if got := fromLinks.SignedURL(); got != "https://s3/from-links.pdf" {
		t.Errorf("SignedURL from links = %q, want the links.files.signed URL", got)
	}
	fromAttrs := DocumentResource{Attributes: DocumentAttributes{SignedFileURL: "https://legacy/signed.pdf"}}
	if got := fromAttrs.SignedURL(); got != "https://legacy/signed.pdf" {
		t.Errorf("SignedURL legacy fallback = %q, want the attributes.signed_file_url URL", got)
	}
	if got := (DocumentResource{}).SignedURL(); got != "" {
		t.Errorf("SignedURL with no URLs = %q, want empty", got)
	}
}

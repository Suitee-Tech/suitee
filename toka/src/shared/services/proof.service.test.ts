import { beforeEach, describe, expect, it, vi } from "vitest";

import api from "./api";
import { getProofBrowserURL, getProofContentPath, proofService } from "./proof.service";

vi.mock("./api", () => ({
  API_URL: "http://localhost:8080/api/v1",
  default: {
    get: vi.fn(),
    post: vi.fn(),
  },
}));

describe("proofService", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("converts a legacy signed storage URL to an internal opaque path", () => {
    const path = getProofContentPath(
      "https://files.example/storage/v1/object/sign/financial-proofs/tenant-a/commission/2026-08/550e8400-e29b-41d4-a716-446655440000.pdf?token=secret",
    );

    expect(path).toMatch(/^\/proofs\/content\/[A-Za-z0-9_-]+$/);
    expect(path).not.toContain("secret");
    expect(path).not.toContain("financial-proofs");
  });

  it("does not treat an arbitrary external URL as a managed proof", () => {
    expect(getProofContentPath("https://drive.google.com/file/d/external-proof/view")).toBeNull();
  });

  it("builds a browser link with the API base path", () => {
    expect(getProofBrowserURL("/proofs/content/opaque-token")).toMatch(/\/api\/v1\/proofs\/content\/opaque-token$/);
  });

  it("downloads through the authenticated API path instead of the storage URL", async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: new Blob(["%PDF-1.4"], { type: "application/pdf" }),
      headers: {
        "content-type": "application/pdf",
        "content-disposition": "inline; filename=\"comprovante.pdf\"",
      },
    } as never);
    const signedURL =
      "https://files.example/storage/v1/object/sign/financial-proofs/tenant-a/commission/2026-08/550e8400-e29b-41d4-a716-446655440000.pdf?token=secret";

    const result = await proofService.download(signedURL);

    expect(api.get).toHaveBeenCalledWith(expect.stringMatching(/^\/proofs\/content\/[A-Za-z0-9_-]+$/), {
      responseType: "blob",
    });
    expect(result.contentType).toBe("application/pdf");
    expect(result.filename).toBe("comprovante.pdf");
  });
});

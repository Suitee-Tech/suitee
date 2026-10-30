import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { avatarColor, formatFileSize, getInitials, highlightText } from "./presentation";

describe("message presentation helpers", () => {
  it("formats participant initials and attachment sizes", () => {
    expect(getInitials("Maria da Silva")).toBe("MD");
    expect(formatFileSize(512)).toBe("512 B");
    expect(formatFileSize(1536)).toBe("1.5 KB");
    expect(formatFileSize(2 * 1024 * 1024)).toBe("2.0 MB");
  });

  it("selects a deterministic avatar color", () => {
    expect(avatarColor("account-123")).toBe(avatarColor("account-123"));
  });

  it("treats search text as literal text when highlighting", () => {
    const markup = renderToStaticMarkup(highlightText("Contrato (final)", "(final)"));

    expect(markup).toContain("<mark");
    expect(markup).toContain("(final)");
  });
});

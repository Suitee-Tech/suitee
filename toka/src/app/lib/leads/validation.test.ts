import { describe, expect, it } from "vitest";
import { normalizeBRPhone } from "./validation";

describe("lib/leads/validation.normalizeBRPhone", () => {
  // Fixtures mirror toca/.../pkg/validation/phone_test.go — keep them in sync.
  it.each([
    ["mobile spaces", "11 99999 9999", "+5511999999999"],
    ["mobile parens dash", "(11) 99999-9999", "+5511999999999"],
    ["mobile dots", "11.99999.9999", "+5511999999999"],
    ["mobile with country code", "5511999999999", "+5511999999999"],
    ["mobile with +55", "+55 11 99999-9999", "+5511999999999"],
    ["landline 10 digits", "1133334444", "+551133334444"],
    ["landline with parens", "(11) 3333-4444", "+551133334444"],
    ["landline with country code", "551133334444", "+551133334444"],
    ["mobile rio 21", "21 98765 4321", "+5521987654321"],
  ])("normalizes %s", (_name, raw, want) => {
    const got = normalizeBRPhone(raw);
    expect(got.ok).toBe(true);
    if (got.ok) expect(got.value).toBe(want);
  });

  it.each([
    ["empty", "", "empty"],
    ["only spaces", "   ", "empty"],
    ["garbage letters", "abc", "empty"],
    ["too short", "1199", "length"],
    ["too long", "115512345678901", "length"],
    ["invalid DDD 00", "00999999999", "ddd"],
    ["invalid DDD 10", "10999999999", "ddd"],
    ["mobile missing 9", "11888888888", "mobile_missing_9"],
    ["landline starts with 1", "1112345678", "landline_invalid_prefix"],
    ["landline starts with 0", "1102345678", "landline_invalid_prefix"],
    ["landline starts with 6", "1162345678", "landline_invalid_prefix"],
  ])("rejects %s", (_name, raw, reason) => {
    const got = normalizeBRPhone(raw);
    expect(got.ok).toBe(false);
    if (!got.ok) expect(got.reason).toBe(reason);
  });
});

import { afterEach, describe, expect, it, vi } from "vitest";
import { allDealsQueryKey, fetchAllDeals } from "./queries";
import { dealService, type DealDTO } from "../services/deal.service";

vi.mock("../services/deal.service", () => ({ dealService: { getAll: vi.fn() } }));

const items = (start: number, count: number) => Array.from({ length: count }, (_, index) => ({ id: String(start + index) }) as DealDTO);
const page = (data: DealDTO[], total: number) => ({ data: { data, meta: { total } } }) as Awaited<ReturnType<typeof dealService.getAll>>;

describe("complete deal collection", () => {
  afterEach(() => vi.resetAllMocks());
  it("collects every page and keeps tenant caches distinct", async () => {
    vi.mocked(dealService.getAll).mockResolvedValueOnce(page(items(0,100),225)).mockResolvedValueOnce(page(items(100,100),225)).mockResolvedValueOnce(page(items(200,25),225));
    const result = await fetchAllDeals();
    expect(result).toHaveLength(225);
    expect(result[result.length - 1]?.id).toBe("224");
    expect(vi.mocked(dealService.getAll).mock.calls.map(([limit,offset]) => [limit,offset])).toEqual([[100,0],[100,100],[100,200]]);
    expect(allDealsQueryKey("agency-a")).not.toEqual(allDealsQueryKey("agency-b"));
  });
  it("rejects the entire collection when a later page fails", async () => {
    vi.mocked(dealService.getAll).mockResolvedValueOnce(page(items(0,100),101)).mockRejectedValueOnce(new Error("offline"));
    await expect(fetchAllDeals()).rejects.toThrow("offline");
  });
  it("rejects an unstable total instead of returning partial counts", async () => {
    vi.mocked(dealService.getAll).mockResolvedValueOnce(page(items(0,100),101)).mockResolvedValueOnce(page(items(100,1),102));
    await expect(fetchAllDeals()).rejects.toThrow("lista mudou");
  });
  it("stops collection when its query is cancelled", async () => {
    const controller = new AbortController();
    vi.mocked(dealService.getAll).mockImplementationOnce(async () => { controller.abort(); return page(items(0,100),101); });
    await expect(fetchAllDeals(controller.signal)).rejects.toThrow("Coleta cancelada");
    expect(dealService.getAll).toHaveBeenCalledTimes(1);
  });
});

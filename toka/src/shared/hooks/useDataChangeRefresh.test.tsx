import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { publishDataChange } from "../lib/dataSync";
import { useDataChangeRefresh } from "./useDataChangeRefresh";

function Harness({
  refresh,
  resources = ["properties"],
}: {
  refresh: () => void | Promise<void>;
  resources?: string[];
}) {
  useDataChangeRefresh(resources, refresh);
  return null;
}

describe("useDataChangeRefresh", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("coalesces related changes and ignores unrelated domains", async () => {
    const refresh = vi.fn();
    const view = render(<Harness refresh={refresh} />);

    act(() => {
      publishDataChange({ source: "manual", resources: ["properties"] }, { broadcast: false });
      publishDataChange({ source: "manual", resources: ["properties"] }, { broadcast: false });
      publishDataChange({ source: "manual", resources: ["calendar"] }, { broadcast: false });
    });

    expect(refresh).not.toHaveBeenCalled();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(80);
    });
    expect(refresh).toHaveBeenCalledTimes(1);

    view.unmount();
  });

  it("filters cross-tenant changes", async () => {
    window.localStorage.setItem("current_company_id", "company-a");
    const refresh = vi.fn();
    const view = render(<Harness refresh={refresh} />);

    act(() => {
      publishDataChange(
        { source: "manual", resources: ["properties"], tenantId: "company-b" },
        { broadcast: false },
      );
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(100);
    });

    expect(refresh).not.toHaveBeenCalled();
    view.unmount();
  });
});

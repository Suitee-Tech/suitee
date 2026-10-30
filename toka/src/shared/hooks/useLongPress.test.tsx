import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useLongPress } from "./useLongPress";

function Harness({ onLongPress, onClickChild }: { onLongPress: () => void; onClickChild?: () => void }) {
  const longPress = useLongPress(onLongPress, { delay: 400 });
  return (
    <div data-testid="target" {...longPress}>
      <button type="button" data-testid="child" onClick={onClickChild}>
        child
      </button>
    </div>
  );
}

const touchAt = (x: number, y: number) => ({ touches: [{ clientX: x, clientY: y }] });

describe("useLongPress", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("dispara só depois de manter o toque parado pelo delay", () => {
    const onLongPress = vi.fn();
    render(<Harness onLongPress={onLongPress} />);
    const target = screen.getByTestId("target");

    fireEvent.touchStart(target, touchAt(10, 10));
    act(() => {
      vi.advanceTimersByTime(399);
    });
    expect(onLongPress).not.toHaveBeenCalled();

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(onLongPress).toHaveBeenCalledTimes(1);
  });

  it("não dispara num toque rápido", () => {
    const onLongPress = vi.fn();
    render(<Harness onLongPress={onLongPress} />);
    const target = screen.getByTestId("target");

    fireEvent.touchStart(target, touchAt(10, 10));
    act(() => {
      vi.advanceTimersByTime(100);
    });
    fireEvent.touchEnd(target);
    act(() => {
      vi.advanceTimersByTime(1000);
    });

    expect(onLongPress).not.toHaveBeenCalled();
  });

  it("cancela quando o dedo se move além da tolerância (rolagem)", () => {
    const onLongPress = vi.fn();
    render(<Harness onLongPress={onLongPress} />);
    const target = screen.getByTestId("target");

    fireEvent.touchStart(target, touchAt(10, 10));
    fireEvent.touchMove(target, touchAt(10, 60));
    act(() => {
      vi.advanceTimersByTime(1000);
    });

    expect(onLongPress).not.toHaveBeenCalled();
  });

  it("suprime o clique sintético que segue o long-press", () => {
    const onLongPress = vi.fn();
    const onClickChild = vi.fn();
    render(<Harness onLongPress={onLongPress} onClickChild={onClickChild} />);
    const target = screen.getByTestId("target");
    const child = screen.getByTestId("child");

    fireEvent.touchStart(target, touchAt(10, 10));
    act(() => {
      vi.advanceTimersByTime(400);
    });
    expect(onLongPress).toHaveBeenCalledTimes(1);

    fireEvent.click(child);
    expect(onClickChild).not.toHaveBeenCalled();
  });

  it("deixa passar o clique de um toque normal (sem long-press)", () => {
    const onLongPress = vi.fn();
    const onClickChild = vi.fn();
    render(<Harness onLongPress={onLongPress} onClickChild={onClickChild} />);
    const target = screen.getByTestId("target");
    const child = screen.getByTestId("child");

    fireEvent.touchStart(target, touchAt(10, 10));
    fireEvent.touchEnd(target);
    fireEvent.click(child);

    expect(onLongPress).not.toHaveBeenCalled();
    expect(onClickChild).toHaveBeenCalledTimes(1);
  });
});

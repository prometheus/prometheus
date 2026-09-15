// Copyright The Prometheus Authors

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import CustomInfiniteScroll, {
  InfiniteScrollItemsProps,
} from "./CustomInfiniteScroll";

const Items = ({ items }: InfiniteScrollItemsProps<string>) => (
  <ul>
    {items.map((item) => (
      <li key={item}>{item}</li>
    ))}
  </ul>
);
const dataset = (prefix: string, count = 130) =>
  Array.from({ length: count }, (_, i) => `${prefix}-${i}`);

function scrollToEnd(container: HTMLElement) {
  const scrollable = container.querySelector(".infinite-scroll-component")!;
  Object.defineProperties(scrollable, {
    clientHeight: { configurable: true, value: 100 },
    scrollHeight: { configurable: true, value: 200 },
    scrollTop: { configurable: true, value: 100 },
  });
  fireEvent.scroll(scrollable);
}

afterEach(cleanup);

describe("infinite scroll dataset changes", () => {
  it("resets paging and the widget loading latch when a new source replaces a pending page", () => {
    const first = dataset("first");
    const second = dataset("second");
    const { container, rerender } = render(
      <CustomInfiniteScroll allItems={first} child={Items} />,
    );
    expect(screen.getAllByRole("listitem")).toHaveLength(50);
    act(() => {
      scrollToEnd(container);
      rerender(<CustomInfiniteScroll allItems={second} child={Items} />);
    });
    expect(screen.getAllByRole("listitem")).toHaveLength(50);
    expect(screen.getByText("second-0")).toBeInTheDocument();
    scrollToEnd(container);
    expect(screen.getAllByRole("listitem")).toHaveLength(100);
    expect(screen.getByText("second-99")).toBeInTheDocument();
  });

  it.each([0, 12, 50, 51])(
    "resets an expanded list to a %i-item source",
    (count) => {
      const { container, rerender } = render(
        <CustomInfiniteScroll allItems={dataset("old")} child={Items} />,
      );
      scrollToEnd(container);
      expect(screen.getAllByRole("listitem")).toHaveLength(100);
      rerender(
        <CustomInfiniteScroll allItems={dataset("new", count)} child={Items} />,
      );
      expect(screen.queryAllByRole("listitem")).toHaveLength(
        Math.min(count, 50),
      );
      scrollToEnd(container);
      expect(screen.queryAllByRole("listitem")).toHaveLength(count);
    },
  );
});

import { ComponentType, useState } from "react";
import InfiniteScroll from "react-infinite-scroll-component";

const initialNumberOfItemsDisplayed = 50;

export interface InfiniteScrollItemsProps<T> {
  items: T[];
}

interface CustomInfiniteScrollProps<T> {
  allItems: T[];
  child: ComponentType<InfiniteScrollItemsProps<T>>;
}

const CustomInfiniteScroll = <T,>({
  allItems,
  child,
}: CustomInfiniteScrollProps<T>) => {
  const [page, setPage] = useState({
    source: allItems,
    count: initialNumberOfItemsDisplayed,
    generation: 0,
  });
  if (page.source !== allItems) {
    setPage({
      source: allItems,
      count: initialNumberOfItemsDisplayed,
      generation: page.generation + 1,
    });
  }
  const items = allItems.slice(0, page.count);
  const hasMore = page.count < allItems.length;
  const Child = child;
  const fetchMoreData = () =>
    setPage((current) => ({
      ...current,
      count: current.count + initialNumberOfItemsDisplayed,
    }));

  // Reset the widget's load latch even when the new page has the same length.
  return (
    <InfiniteScroll
      key={page.generation}
      next={fetchMoreData}
      hasMore={hasMore}
      loader={<h4>loading...</h4>}
      dataLength={items.length}
      height={items.length > 25 ? "75vh" : ""}
    >
      <Child items={items} />
    </InfiniteScroll>
  );
};

export default CustomInfiniteScroll;

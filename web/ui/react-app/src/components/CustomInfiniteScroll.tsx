import { ComponentType, useEffect, useState } from 'react';
import InfiniteScroll from 'react-infinite-scroll-component';

const initialNumberOfTargetsDisplayed = 50;

export interface InfiniteScrollItemsProps<T> {
  items: T[];
}

interface InfiniteScrollProps<T> {
  value: T[];
  child: ComponentType<InfiniteScrollItemsProps<T>>;
}

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
const CustomInfiniteScroll = <T extends unknown>({ value, child }: InfiniteScrollProps<T>) => {
  const [items, setItems] = useState<T[]>(value.slice(0, 50));
  const [index, setIndex] = useState<number>(initialNumberOfTargetsDisplayed);
  const [hasMore, setHasMore] = useState<boolean>(value.length > initialNumberOfTargetsDisplayed);
  const Child = child;

  useEffect(() => {
    setItems(value.slice(0, initialNumberOfTargetsDisplayed));
    setHasMore(value.length > initialNumberOfTargetsDisplayed);
  }, [value]);

  const fetchMoreData = () => {
    if (items.length === value.length) {
      setHasMore(false);
    } else {
      const newIndex = index + initialNumberOfTargetsDisplayed;
      setIndex(newIndex);
      setItems(value.slice(0, newIndex));
    }
  };

  return (
    <InfiniteScroll
      next={fetchMoreData}
      hasMore={hasMore}
      loader={<h4>loading</h4>}
      dataLength={items.length}
      height={items.length > 25 ? '75vh' : ''}
    >
      <Child items={items} />
    </InfiniteScroll>
  );
};

export default CustomInfiniteScroll;

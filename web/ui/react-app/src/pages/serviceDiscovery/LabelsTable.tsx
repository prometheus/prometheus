import React, { FC, useEffect, useState } from 'react';
import { Badge, Table } from 'reactstrap';
import { TargetLabels } from './Services';
import { ToggleMoreLess } from '../../components/ToggleMoreLess';
import InfiniteScroll from 'react-infinite-scroll-component';

interface LabelProps {
  value: TargetLabels[];
  name: string;
}

const initialNumberOfTargetsDisplayed = 50;

const formatLabels = (labels: Record<string, string> | string) => {
  return Object.entries(labels).map(([key, value]) => {
    return (
      <div key={key}>
        <Badge color="primary" className="mr-1">
          {`${key}="${value}"`}
        </Badge>
      </div>
    );
  });
};

export const LabelsTable: FC<LabelProps> = ({ value, name }) => {
  const [showMore, setShowMore] = useState(false);
  const [items, setItems] = useState<TargetLabels[]>(value.slice(0, 50));
  const [index, setIndex] = useState<number>(initialNumberOfTargetsDisplayed);
  const [hasMore, setHasMore] = useState<boolean>(value.length > initialNumberOfTargetsDisplayed);

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
    <>
      <div>
        <ToggleMoreLess
          event={(): void => {
            setShowMore(!showMore);
          }}
          showMore={showMore}
        >
          <span className="target-head">{name}</span>
        </ToggleMoreLess>
      </div>
      {showMore ? (
        <InfiniteScroll
          next={fetchMoreData}
          hasMore={hasMore}
          loader={<h4>loading...</h4>}
          dataLength={items.length}
          height={items.length > 25 ? '75vh' : ''}
        >
          <Table size="sm" bordered hover striped>
            <thead>
              <tr>
                <th>Discovered Labels</th>
                <th>Target Labels</th>
              </tr>
            </thead>
            <tbody>
              {items.map((_, i) => {
                return (
                  <tr key={i}>
                    <td>{formatLabels(items[i].discoveredLabels)}</td>
                    {items[i].isDropped ? (
                      <td style={{ fontWeight: 'bold' }}>Dropped</td>
                    ) : (
                      <td>{formatLabels(items[i].labels)}</td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </Table>
        </InfiniteScroll>
      ) : null}
    </>
  );
};

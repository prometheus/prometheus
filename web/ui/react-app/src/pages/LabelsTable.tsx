import React, { FC, useState } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Button, Badge, Table } from 'reactstrap';
import { targetLabels } from './Services';

interface LabelProps {
  value: targetLabels[];
  name: string;
}

const formatLabels = (labels: Record<string, string>) => {
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

export const LabelsTable: FC<RouteComponentProps & LabelProps> = ({ value, name }) => {
  const [showMore, setShowMore] = useState(false);

  const toggleShowMore = () => {
    setShowMore(!showMore);
  };

  return (
    <div className="label-component">
      <div className="label-button-style">
        <span className="target-head"> {name} </span>
        <Button size="sm" color="primary" onClick={toggleShowMore}>
          {showMore ? 'show less' : 'show more'}
        </Button>
      </div>
      {showMore ? (
        <Table striped={true} bordered={true}>
          <thead>
            <tr>
              <th>Discovered Labels</th>
              <th>Target Labels</th>
            </tr>
          </thead>
          <tbody>
            {value.map((_, i) => {
              return (
                <tr key={i}>
                  <td>{formatLabels(value[i].discoveredLabels)}</td>
                  <td>{formatLabels(value[i].labels)}</td>
                </tr>
              );
            })}
          </tbody>
        </Table>
      ) : null}
    </div>
  );
};

import React, { FC, useState } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Button, Badge, Table } from 'reactstrap';

interface LabelProps {
  value: Record<string, Record<string, string>>[];
  name: string;
}

const printLabels = (labels: Record<string, string>) => {
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
  const [showMore, doToggle] = useState(false);

  const toggleMore = () => {
    doToggle(!showMore);
  };

  return (
    <div className="label-component">
      <span className="target-head"> {name} </span>
      <Button size="sm" color="primary" onClick={toggleMore}>
        {showMore ? 'show more' : 'show less'}
      </Button>
      {showMore ? (
        <Table striped={true} bordered={true}>
          <thead className="table-outer-layer">
            <tr>
              <th>Discovered Labels</th>
              <th>Target Labels</th>
            </tr>
          </thead>
          <tbody>
            {value.map((_, i) => {
              return (
                <tr key={i}>
                  <td>{printLabels(value[i].discoveredLabels)}</td>
                  <td>{printLabels(value[i].labels)}</td>
                </tr>
              );
            })}
          </tbody>
        </Table>
      ) : null}
    </div>
  );
};

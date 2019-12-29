import React, { FC, useState } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Button, Badge, Table } from 'reactstrap';
import { TargetLabels } from './Services';
import { ToggleButton } from './targets/ToggleButton';

interface LabelProps {
  value: TargetLabels[];
  name: string;
}

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

export const LabelsTable: FC<RouteComponentProps & LabelProps> = ({ value, name }) => {
  const [showMore, setShowMore] = useState(false);

  return (
    <>
      <div>
        <h3>
          <span className="target-head">{name}</span>
          <ToggleButton child={`show ${showMore ? 'less' : 'more'}`} onClick={(): void => { setShowMore(!showMore); }} />
        </h3>
      </div>
      {showMore ? (
        <Table size="sm" bordered hover striped>
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
                  {value[i].isDropped ? (
                    <td style={{ fontWeight: 'bold' }}>Dropped</td>
                  ) : (
                    <td>{formatLabels(value[i].labels)}</td>
                  )}
                </tr>
              );
            })}
          </tbody>
        </Table>
      ) : null}
    </>
  );
};

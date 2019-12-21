import React, { FC, useState } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Button, Badge, Table } from 'reactstrap';

interface LabelProps {
  value: Record<string, Record<string, string>>[];
}

export const LabelsTable: FC<RouteComponentProps & LabelProps> = ({ value }) => {
  const [showMore, doToggle] = useState(false);

  const toggleMore = () => {
    doToggle(!showMore);
  };

  return (
    <>
      <Button size="sm" color="primary" onClick={toggleMore}>
        {showMore ? 'More' : 'Less'}
      </Button>
      {showMore ? (
        <Table striped={true} className="table-head">
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
                  <td>
                    {Object.entries(value[i].discoveredLabels).map(([key, value], i) => {
                      return (
                        <div className="label-style" key={i}>
                          <Badge color="primary" className={`mr-1 ${key}`} key={i}>
                            {`${key}="${value}"`}
                          </Badge>
                        </div>
                      );
                    })}
                  </td>
                  <td>
                    {Object.entries(value[i].labels).map(([key, value], i) => {
                      return (
                        <div className="label-style" key={i}>
                          <Badge color="primary" className={`mr-1 ${key}`} key={i}>
                            {`${key}="${value}"`}
                          </Badge>
                        </div>
                      );
                    })}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </Table>
      ) : null}
    </>
  );
};

import React, { FC, useState } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Button, Badge } from 'reactstrap';

interface LabelProps {
  value: any;
}

const Labels: FC<RouteComponentProps & LabelProps> = ({ value }) => {
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
        <>
          <div>
            {Object.keys(value).map((_, i) => {
              return (
                <div key={i} className="row inner-layer">
                  <div className="col-md-6">
                    {i === 0 ? <div className="head">Discovered Labels</div> : null}
                    <div>
                      {Object.keys(value[i].discoveredLabels).map((labelName, j) => (
                        <div className="label-style" key={j}>
                          <Badge color="primary" className={`mr-1 ${labelName}`} key={labelName}>
                            {`${labelName}="${value[i].discoveredLabels[labelName]}"`}
                          </Badge>
                        </div>
                      ))}
                    </div>
                  </div>
                  <div className="col-md-6">
                    {i === 0 ? <div className="head">Target Labels</div> : null}
                    <div>
                      {Object.keys(value[i].labels).map((labelName, j) => (
                        <div className="label-style" key={j}>
                          <Badge color="primary" className={`mr-1 ${labelName}`} key={labelName}>
                            {`${labelName}="${value[i].labels[labelName]}"`}
                          </Badge>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </>
      ) : null}
    </>
  );
};

export default Labels;

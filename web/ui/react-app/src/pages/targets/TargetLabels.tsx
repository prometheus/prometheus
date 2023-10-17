import { faChevronDown, faChevronUp } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import React, { FC, useState } from 'react';
import { Badge, Button } from 'reactstrap';

interface Labels {
  [key: string]: string;
}

export interface TargetLabelsProps {
  discoveredLabels: Labels;
  labels: Labels;
}

const TargetLabels: FC<TargetLabelsProps> = ({ discoveredLabels, labels }) => {
  const [showDiscovered, setShowDiscovered] = useState(false);

  return (
    <>
      <div className="series-labels-container">
        {Object.keys(labels).map((labelName) => {
          return (
            <Badge color="primary" className="mr-1" key={labelName}>
              {`${labelName}="${labels[labelName]}"`}
            </Badge>
          );
        })}
        <Button
          size="sm"
          color="link"
          title={`${showDiscovered ? 'Hide' : 'Show'} discovered (pre-relabeling) labels`}
          onClick={() => setShowDiscovered(!showDiscovered)}
          style={{ fontSize: '0.8rem' }}
        >
          <FontAwesomeIcon icon={showDiscovered ? faChevronUp : faChevronDown} />
        </Button>
      </div>
      {showDiscovered && (
        <>
          <div className="mt-3 font-weight-bold">Discovered labels:</div>
          {Object.keys(discoveredLabels).map((labelName) => (
            <div key={labelName}>
              <Badge color="info" className="mr-1">
                {`${labelName}="${discoveredLabels[labelName]}"`}
              </Badge>
            </div>
          ))}
        </>
      )}
    </>
  );
};

export default TargetLabels;

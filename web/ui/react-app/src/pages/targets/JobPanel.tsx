import React, { FC, useState } from 'react';
import { TargetGroup } from './target';
import JobHeader from './JobHeader';
import { Collapse } from 'reactstrap';
import JobDetails from './JobDetails';
import styles from './JobPanel.module.css';

interface JobPanelProps {
  scrapeJob: string;
  targetGroup: TargetGroup;
}

const JobPanel: FC<JobPanelProps> = ({ scrapeJob, targetGroup }) => {
  const [expanded, setExpanded] = useState(true);
  const jobHeaderProps = {
    scrapeJob,
    expanded,
    setExpanded,
    up: targetGroup.metadata.up,
    targetTotal: targetGroup.targets.length,
  };

  return (
    <div className={styles.container}>
      <JobHeader {...jobHeaderProps} />
      <Collapse isOpen={expanded}>
        <JobDetails targetGroup={targetGroup} />
      </Collapse>
    </div>
  );
};

export default JobPanel;

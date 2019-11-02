import React, { FC } from 'react';
import { Button } from 'reactstrap';
import styles from './JobHeader.module.css';

interface JobHeaderProps {
  expanded: boolean;
  scrapeJob: string;
  setExpanded: React.Dispatch<React.SetStateAction<boolean>>;
  targetTotal: number;
  up: number;
}

const JobHeader: FC<JobHeaderProps> = ({ expanded, scrapeJob, setExpanded, targetTotal, up }) => {
  const modifier = up < targetTotal ? 'danger' : 'normal';
  const id = `job-${scrapeJob}`;
  const anchorProps = {
    href: `#${id}`,
    id,
  };
  const btnProps = {
    children: `show ${expanded ? 'less' : 'more'}`,
    color: 'primary',
    onClick: (): void => setExpanded(!expanded),
    size: 'xs',
  };
  return (
    <h2>
      <a className={styles[modifier]} {...anchorProps}>
        {`${scrapeJob} (${up}/${targetTotal} up)`}
      </a>
      <Button className={styles['expand-btn']} {...btnProps} />
    </h2>
  );
};

export default JobHeader;

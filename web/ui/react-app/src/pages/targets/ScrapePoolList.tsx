import { KVSearch } from '@nexucis/kvsearch';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { useFetch } from '../../hooks/useFetch';
import { API_PATH } from '../../constants/constants';
import { groupTargets, ScrapePool, ScrapePools, Target } from './target';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { ChangeEvent, FC, useEffect, useState } from 'react';
import { Col, Collapse, Row } from 'reactstrap';
import { ScrapePoolContent } from './ScrapePoolContent';
import Filter, { Expanded, FilterData } from './Filter';
import { useLocalStorage } from '../../hooks/useLocalStorage';
import styles from './ScrapePoolPanel.module.css';
import { ToggleMoreLess } from '../../components/ToggleMoreLess';
import SearchBar from '../../components/SearchBar';

interface ScrapePoolListProps {
  activeTargets: Target[];
}

const kvSearch = new KVSearch<Target>({
  shouldSort: true,
  indexedKeys: ['labels', 'scrapePool', ['labels', /.*/]],
});

interface PanelProps {
  scrapePool: string;
  targetGroup: ScrapePool;
  expanded: boolean;
  toggleExpanded: () => void;
}

export const ScrapePoolPanel: FC<PanelProps> = (props: PanelProps) => {
  const modifier = props.targetGroup.upCount < props.targetGroup.targets.length ? 'danger' : 'normal';
  const id = `pool-${props.scrapePool}`;
  const anchorProps = {
    href: `#${id}`,
    id,
  };
  return (
    <div>
      <ToggleMoreLess event={props.toggleExpanded} showMore={props.expanded}>
        <a className={styles[modifier]} {...anchorProps}>
          {`${props.scrapePool} (${props.targetGroup.upCount}/${props.targetGroup.targets.length} up)`}
        </a>
      </ToggleMoreLess>
      <Collapse isOpen={props.expanded}>
        <ScrapePoolContent targets={props.targetGroup.targets} />
      </Collapse>
    </div>
  );
};

// ScrapePoolListContent is taking care of every possible filter
const ScrapePoolListContent: FC<ScrapePoolListProps> = ({ activeTargets }) => {
  const initialPoolList = groupTargets(activeTargets);
  const [poolList, setPoolList] = useState<ScrapePools>(initialPoolList);
  const [targetList, setTargetList] = useState(activeTargets);

  const initialFilter: FilterData = {
    showHealthy: true,
    showUnhealthy: true,
  };
  const [filter, setFilter] = useLocalStorage('targets-page-filter', initialFilter);

  const initialExpanded: Expanded = Object.keys(initialPoolList).reduce(
    (acc: { [scrapePool: string]: boolean }, scrapePool: string) => ({
      ...acc,
      [scrapePool]: true,
    }),
    {}
  );
  const [expanded, setExpanded] = useLocalStorage('targets-page-expansion-state', initialExpanded);
  const { showHealthy, showUnhealthy } = filter;

  const handleSearchChange = (e: ChangeEvent<HTMLTextAreaElement | HTMLInputElement>) => {
    if (e.target.value !== '') {
      const result = kvSearch.filter(e.target.value.trim(), activeTargets);
      setTargetList(result.map((value) => value.original));
    } else {
      setTargetList(activeTargets);
    }
  };

  useEffect(() => {
    const list = targetList.filter((t) => showHealthy || t.health.toLowerCase() !== 'up');
    setPoolList(groupTargets(list));
  }, [showHealthy, targetList]);

  return (
    <>
      <Row xs="4" className="align-items-center">
        <Col>
          <Filter filter={filter} setFilter={setFilter} expanded={expanded} setExpanded={setExpanded} />
        </Col>
        <Col xs="6">
          <SearchBar handleChange={handleSearchChange} placeholder="Filter by endpoint or labels" />
        </Col>
      </Row>
      {Object.keys(poolList)
        .filter((scrapePool) => {
          const targetGroup = poolList[scrapePool];
          const isHealthy = targetGroup.upCount === targetGroup.targets.length;
          return (isHealthy && showHealthy) || (!isHealthy && showUnhealthy);
        })
        .map<JSX.Element>((scrapePool) => (
          <ScrapePoolPanel
            key={scrapePool}
            scrapePool={scrapePool}
            targetGroup={poolList[scrapePool]}
            expanded={expanded[scrapePool]}
            toggleExpanded={(): void => setExpanded({ ...expanded, [scrapePool]: !expanded[scrapePool] })}
          />
        ))}
    </>
  );
};

const ScrapePoolListWithStatusIndicator = withStatusIndicator(ScrapePoolListContent);

export const ScrapePoolList: FC = () => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<ScrapePoolListProps>(`${pathPrefix}/${API_PATH}/targets?state=active`);
  const { status: responseStatus } = response;
  const badResponse = responseStatus !== 'success' && responseStatus !== 'start fetching';
  return (
    <ScrapePoolListWithStatusIndicator
      {...response.data}
      error={badResponse ? new Error(responseStatus) : error}
      isLoading={isLoading}
      componentTitle="Targets information"
    />
  );
};

export default ScrapePoolList;

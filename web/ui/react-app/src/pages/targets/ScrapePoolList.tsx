import { KVSearch } from '@nexucis/kvsearch';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { useFetch } from '../../hooks/useFetch';
import { API_PATH } from '../../constants/constants';
import { filterTargetsByHealth, groupTargets, ScrapePool, ScrapePools, Target } from './target';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { FC, useCallback, useEffect, useMemo, useState } from 'react';
import { Badge, Col, Collapse, Dropdown, DropdownItem, DropdownMenu, DropdownToggle, Input, Row } from 'reactstrap';
import { ScrapePoolContent } from './ScrapePoolContent';
import Filter, { Expanded, FilterData } from './Filter';
import { useLocalStorage } from '../../hooks/useLocalStorage';
import styles from './ScrapePoolPanel.module.css';
import { ToggleMoreLess } from '../../components/ToggleMoreLess';
import SearchBar from '../../components/SearchBar';
import { setQuerySearchFilter, getQuerySearchFilter } from '../../utils/index';
import Checkbox from '../../components/Checkbox';

export interface ScrapePoolNamesListProps {
  scrapePools: string[];
}

interface ScrapePoolDropDownProps {
  selectedPool: string | null;
  scrapePools: string[];
  onScrapePoolChange: (name: string) => void;
}

const ScrapePoolDropDown: FC<ScrapePoolDropDownProps> = ({ selectedPool, scrapePools, onScrapePoolChange }) => {
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const toggle = () => setDropdownOpen((prevState) => !prevState);

  const [filter, setFilter] = useState<string>('');

  const filteredPools = scrapePools.filter((pool) => pool.toLowerCase().includes(filter.toLowerCase()));

  return (
    <Dropdown isOpen={dropdownOpen} toggle={toggle}>
      <DropdownToggle caret className="mw-100 text-truncate">
        {selectedPool === null || !scrapePools.includes(selectedPool) ? 'All scrape pools' : selectedPool}
      </DropdownToggle>
      <DropdownMenu style={{ maxHeight: 400, overflowY: 'auto' }}>
        {selectedPool ? (
          <>
            <DropdownItem key="__all__" value={null} onClick={() => onScrapePoolChange('')}>
              Clear selection
            </DropdownItem>
            <DropdownItem divider />
          </>
        ) : null}
        <DropdownItem key="__header" header toggle={false}>
          <Input autoFocus placeholder="Filter" value={filter} onChange={(event) => setFilter(event.target.value.trim())} />
        </DropdownItem>
        {scrapePools.length === 0 ? (
          <DropdownItem disabled>No scrape pools configured</DropdownItem>
        ) : (
          filteredPools.map((name) => (
            <DropdownItem key={name} value={name} onClick={() => onScrapePoolChange(name)} active={name === selectedPool}>
              {name}
            </DropdownItem>
          ))
        )}
      </DropdownMenu>
    </Dropdown>
  );
};

interface ScrapePoolListProps {
  scrapePools: string[];
  selectedPool: string | null;
  onPoolSelect: (name: string) => void;
}

interface ScrapePoolListContentProps extends ScrapePoolListProps {
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

type targetHealth = 'healthy' | 'unhealthy' | 'unknown';

const healthColorTuples: Array<[targetHealth, string]> = [
  ['healthy', 'success'],
  ['unhealthy', 'danger'],
  ['unknown', 'warning'],
];

// ScrapePoolListContent is taking care of every possible filter
const ScrapePoolListContent: FC<ScrapePoolListContentProps> = ({
  activeTargets,
  scrapePools,
  selectedPool,
  onPoolSelect,
}) => {
  const initialPoolList = groupTargets(activeTargets);
  const [poolList, setPoolList] = useState<ScrapePools>(initialPoolList);
  const [targetList, setTargetList] = useState(activeTargets);

  const initialFilter: FilterData = {
    showHealthy: true,
    showUnhealthy: true,
  };
  const [filter, setFilter] = useLocalStorage('targets-page-filter', initialFilter);

  const [healthFilters, setHealthFilters] = useLocalStorage('target-health-filter', {
    healthy: true,
    unhealthy: true,
    unknown: true,
  });
  const toggleHealthFilter = (val: targetHealth) => () => {
    setHealthFilters({
      ...healthFilters,
      [val]: !healthFilters[val],
    });
  };

  const initialExpanded: Expanded = Object.keys(initialPoolList).reduce(
    (acc: { [scrapePool: string]: boolean }, scrapePool: string) => ({
      ...acc,
      [scrapePool]: true,
    }),
    {}
  );
  const [expanded, setExpanded] = useLocalStorage('targets-page-expansion-state', initialExpanded);
  const { showHealthy, showUnhealthy } = filter;

  const handleSearchChange = useCallback(
    (value: string) => {
      setQuerySearchFilter(value);
      if (value !== '') {
        const result = kvSearch.filter(value.trim(), activeTargets);
        setTargetList(result.map((value) => value.original));
      } else {
        setTargetList(activeTargets);
      }
    },
    [activeTargets]
  );

  const defaultValue = useMemo(getQuerySearchFilter, []);

  useEffect(() => {
    const list = targetList.filter((t) => showHealthy || t.health.toLowerCase() !== 'up');
    setPoolList(groupTargets(list));
  }, [showHealthy, targetList]);

  return (
    <>
      <Row className="align-items-center">
        <Col className="flex-grow-0 py-1">
          <ScrapePoolDropDown selectedPool={selectedPool} scrapePools={scrapePools} onScrapePoolChange={onPoolSelect} />
        </Col>
        <Col className="flex-grow-0 py-1">
          <Filter filter={filter} setFilter={setFilter} expanded={expanded} setExpanded={setExpanded} />
        </Col>
        <Col className="flex-grow-1 py-1">
          <SearchBar
            defaultValue={defaultValue}
            handleChange={handleSearchChange}
            placeholder="Filter by endpoint or labels"
          />
        </Col>
        <Col className="flex-grow-0 py-1">
          <div className="d-flex flex-row-reverse">
            {healthColorTuples.map(([val, color]) => (
              <Checkbox
                wrapperStyles={{ marginBottom: 0 }}
                key={val}
                checked={healthFilters[val]}
                id={`${val}-toggler`}
                onChange={toggleHealthFilter(val)}
              >
                <Badge color={color} className="text-capitalize">
                  {val}
                </Badge>
              </Checkbox>
            ))}
          </div>
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
            targetGroup={{
              upCount: poolList[scrapePool].upCount,
              targets: poolList[scrapePool].targets.filter((target) => filterTargetsByHealth(target.health, healthFilters)),
            }}
            expanded={expanded[scrapePool]}
            toggleExpanded={(): void => setExpanded({ ...expanded, [scrapePool]: !expanded[scrapePool] })}
          />
        ))}
    </>
  );
};

const ScrapePoolListWithStatusIndicator = withStatusIndicator(ScrapePoolListContent);

export const ScrapePoolList: FC<ScrapePoolListProps> = ({ selectedPool, scrapePools, ...props }) => {
  // If we have more than 20 scrape pools AND there's no pool selected then select first pool
  // by default. This is to avoid loading a huge list of targets when we have many pools configured.
  // If we have up to 20 scrape pools then pass whatever is the value of selectedPool, it can
  // be a pool name or a null (if all pools should be shown).
  const poolToShow = selectedPool === null && scrapePools.length > 20 ? scrapePools[0] : selectedPool;

  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<ScrapePoolListContentProps>(
    `${pathPrefix}/${API_PATH}/targets?state=active${poolToShow === null ? '' : `&scrapePool=${poolToShow}`}`
  );
  const { status: responseStatus } = response;
  const badResponse = responseStatus !== 'success' && responseStatus !== 'start fetching';

  return (
    <ScrapePoolListWithStatusIndicator
      {...props}
      {...response.data}
      selectedPool={poolToShow}
      scrapePools={scrapePools}
      error={badResponse ? new Error(responseStatus) : error}
      isLoading={isLoading}
      componentTitle="Targets information"
    />
  );
};

export default ScrapePoolList;

import React, { ChangeEvent, FC, useState } from 'react';
import { Input, InputGroup, Table } from 'reactstrap';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { useFetch } from '../../hooks/useFetch';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { API_PATH } from '../../constants/constants';
import { faSort, faSortDown, faSortUp } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { IconDefinition } from '@fortawesome/fontawesome-common-types';
import sanitizeHTML from 'sanitize-html';
import { Fuzzy, FuzzyResult } from '@nexucis/fuzzy';

const fuz = new Fuzzy({ pre: '<strong>', post: '</strong>', shouldSort: true });
const metricSeparator = '||';

interface MetricData {
  type?: string;
  help?: string;
  unit?: string;
}

interface ExploreMap {
  [key: string]: Array<MetricData>;
}

interface ExploreProps {
  status?: string;
  data?: ExploreMap;
}

const compareAlphaFn =
  (keys: boolean, reverse: boolean) =>
  ([k1, v1]: [string, Array<MetricData>], [k2, v2]: [string, Array<MetricData>]): number => {
    const a = keys ? k1 : k1;
    const b = keys ? k2 : k2;
    const reverser = reverse ? -1 : 1;
    return reverser * a.localeCompare(b);
  };

const getSortIcon = (b: boolean | undefined): IconDefinition => {
  if (b === undefined) {
    return faSort;
  }
  if (b) {
    return faSortDown;
  }
  return faSortUp;
};

interface SortState {
  name: string;
  alpha: boolean;
  focused: boolean;
}

export const ExploreContent: FC<ExploreProps> = ({ data = {} }) => {
  const initialSearch = '';
  const [searchState, setSearchState] = useState(initialSearch);
  const initialSort: SortState = {
    name: 'Metric',
    alpha: true,
    focused: true,
  };
  const [sortState, setSortState] = useState(initialSort);
  const searchable = Object.entries(data)
    .sort(compareAlphaFn(sortState.name === 'Metric', !sortState.alpha))
    .map(([metric, value]) => `${metric}${metricSeparator}${value[0]['help']}`);
  let filtered = searchable;
  if (searchState.length > 0) {
    filtered = fuz.filter(searchState, searchable).map((value: FuzzyResult) => value.rendered);
  }
  return (
    <>
      <h2>Metrics Explorer</h2>
      <InputGroup>
        <Input
          autoFocus
          placeholder="Filter by metric name..."
          className="my-3"
          value={searchState}
          onChange={({ target }: ChangeEvent<HTMLInputElement>): void => {
            setSearchState(target.value);
          }}
        />
      </InputGroup>
      <Table bordered size="sm" striped hover>
        <thead>
          <tr>
            <td
              key="Metric"
              className={`px-4 Metric`}
              style={{ width: '30%' }}
              onClick={(): void =>
                setSortState({
                  name: 'Metric',
                  focused: true,
                  alpha: sortState.name === 'Metric' ? !sortState.alpha : true,
                })
              }
            >
              <span className="mr-2">Metric</span>
              <FontAwesomeIcon icon={getSortIcon(sortState.name !== 'Metric' ? undefined : sortState.alpha)} />
            </td>
            <td key="Help" className={'px-4 "Help"'} style={{ width: '70%' }}>
              <span className="mr-2">Help</span>
            </td>
          </tr>
        </thead>
        <tbody>
          {filtered.map((result: string) => {
            const [metricMatchStr, valueMatchStr] = result.split(metricSeparator);
            const sanitizeOpts = { allowedTags: ['strong'] };
            return (
              <tr key={metricMatchStr}>
                <td className="flag-item">
                  <span dangerouslySetInnerHTML={{ __html: sanitizeHTML(metricMatchStr, sanitizeOpts) }} />
                </td>
                <td className="flag-value">
                  <span dangerouslySetInnerHTML={{ __html: sanitizeHTML(valueMatchStr, sanitizeOpts) }} />
                </td>
              </tr>
            );
          })}
        </tbody>
      </Table>
    </>
  );
};
const ExploreWithStatusIndicator = withStatusIndicator(ExploreContent);

ExploreContent.displayName = 'Explore';

const Explore: FC = () => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<ExploreMap>(`${pathPrefix}/${API_PATH}/metadata`);
  return <ExploreWithStatusIndicator data={response.data} error={error} isLoading={isLoading} />;
};

export default Explore;

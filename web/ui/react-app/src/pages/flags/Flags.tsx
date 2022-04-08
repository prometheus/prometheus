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
const flagSeparator = '||';

interface FlagMap {
  [key: string]: string;
}

interface FlagsProps {
  data?: FlagMap;
}

const compareAlphaFn =
  (keys: boolean, reverse: boolean) =>
  ([k1, v1]: [string, string], [k2, v2]: [string, string]): number => {
    const a = keys ? k1 : v1;
    const b = keys ? k2 : v2;
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

export const FlagsContent: FC<FlagsProps> = ({ data = {} }) => {
  const initialSearch = '';
  const [searchState, setSearchState] = useState(initialSearch);
  const initialSort: SortState = {
    name: 'Flag',
    alpha: true,
    focused: true,
  };
  const [sortState, setSortState] = useState(initialSort);
  const searchable = Object.entries(data)
    .sort(compareAlphaFn(sortState.name === 'Flag', !sortState.alpha))
    .map(([flag, value]) => `--${flag}${flagSeparator}${value}`);
  let filtered = searchable;
  if (searchState.length > 0) {
    filtered = fuz.filter(searchState, searchable).map((value: FuzzyResult) => value.rendered);
  }
  return (
    <>
      <h2>Command-Line Flags</h2>
      <InputGroup>
        <Input
          autoFocus
          placeholder="Filter by flag name or value..."
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
            {['Flag', 'Value'].map((col: string) => (
              <td
                key={col}
                className={`px-4 ${col}`}
                style={{ width: '50%' }}
                onClick={(): void =>
                  setSortState({
                    name: col,
                    focused: true,
                    alpha: sortState.name === col ? !sortState.alpha : true,
                  })
                }
              >
                <span className="mr-2">{col}</span>
                <FontAwesomeIcon icon={getSortIcon(sortState.name !== col ? undefined : sortState.alpha)} />
              </td>
            ))}
          </tr>
        </thead>
        <tbody>
          {filtered.map((result: string) => {
            const [flagMatchStr, valueMatchStr] = result.split(flagSeparator);
            const sanitizeOpts = { allowedTags: ['strong'] };
            return (
              <tr key={flagMatchStr}>
                <td className="flag-item">
                  <span dangerouslySetInnerHTML={{ __html: sanitizeHTML(flagMatchStr, sanitizeOpts) }} />
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
const FlagsWithStatusIndicator = withStatusIndicator(FlagsContent);

FlagsContent.displayName = 'Flags';

const Flags: FC = () => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<FlagMap>(`${pathPrefix}/${API_PATH}/status/flags`);
  return <FlagsWithStatusIndicator data={response.data} error={error} isLoading={isLoading} />;
};

export default Flags;

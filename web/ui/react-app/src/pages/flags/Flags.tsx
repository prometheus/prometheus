import React, { ChangeEvent, FC, useState } from 'react';
import { RouteComponentProps } from '@reach/router';
import { Input, InputGroup, Table } from 'reactstrap';
import { withStatusIndicator } from '../../components/withStatusIndicator';
import { useFetch } from '../../hooks/useFetch';
import { usePathPrefix } from '../../contexts/PathPrefixContext';
import { API_PATH } from '../../constants/constants';
import { faSort, faSortDown, faSortUp } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { IconDefinition } from '@fortawesome/fontawesome-common-types';

interface FlagMap {
  [key: string]: string;
}

interface FlagsProps {
  data?: FlagMap;
}

const compareAlphaFn = (keys: boolean, reverse: boolean) => (
  [k1, v1]: [string, string],
  [k2, v2]: [string, string]
): number => {
  const a = keys ? k1 : v1;
  const b = keys ? k2 : v2;
  const reverser = reverse ? -1 : 1;
  return reverser * a.localeCompare(b);
};

const boldSubstring = (s: string, subs: string) => {
  if (subs === '' || s.indexOf(subs) < 0) {
    return <span>{s}</span>;
  }
  return <span dangerouslySetInnerHTML={{ __html: s.split(subs).join(subs.bold()) }} />;
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

interface ColumnState {
  name: string;
  alpha: boolean;
  focused: boolean;
}

interface FlagState {
  search: string;
  sort: ColumnState;
}

export const FlagsContent: FC<FlagsProps> = ({ data = {} }) => {
  const initialState: FlagState = {
    search: '',
    sort: {
      name: 'Flag',
      alpha: true,
      focused: true,
    },
  };
  const [state, setState] = useState(initialState);
  return (
    <>
      <h2>Command-Line Flags</h2>
      <InputGroup>
        <Input
          autoFocus
          placeholder="Filter by flag name or value..."
          className="my-3"
          value={state.search}
          onChange={({ target }: ChangeEvent<HTMLInputElement>): void => {
            setState({ ...state, search: target.value });
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
                  setState({
                    ...state,
                    sort: {
                      name: col,
                      focused: true,
                      alpha: state.sort.name === col ? !state.sort.alpha : true,
                    },
                  })
                }
              >
                <span className="mr-2">{col}</span>
                <FontAwesomeIcon icon={getSortIcon(state.sort.name !== col ? undefined : state.sort.alpha)} />
              </td>
            ))}
          </tr>
        </thead>
        <tbody>
          {Object.entries(data)
            .sort(compareAlphaFn(state.sort.name === 'Flag', !state.sort.alpha))
            .filter(([flag, value]) => flag.includes(state.search) || value.includes(state.search))
            .map(([flag, value]) => (
              <tr key={flag}>
                <td className="px-4">
                  <code className="text-dark flag-item">--{boldSubstring(flag, state.search)}</code>
                </td>
                <td className="px-4">
                  <code className="text-dark value-item">{boldSubstring(value, state.search)}</code>
                </td>
              </tr>
            ))}
        </tbody>
      </Table>
    </>
  );
};
const FlagsWithStatusIndicator = withStatusIndicator(FlagsContent);

FlagsContent.displayName = 'Flags';

const Flags: FC<RouteComponentProps> = () => {
  const pathPrefix = usePathPrefix();
  const { response, error, isLoading } = useFetch<FlagMap>(`${pathPrefix}/${API_PATH}/status/flags`);
  return <FlagsWithStatusIndicator data={response.data} error={error} isLoading={isLoading} />;
};

export default Flags;

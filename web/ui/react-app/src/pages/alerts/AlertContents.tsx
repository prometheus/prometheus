import React, { ChangeEvent, FC, Fragment, useEffect, useState } from 'react';
import { Badge, Col, Row } from 'reactstrap';
import CollapsibleAlertPanel from './CollapsibleAlertPanel';
import Checkbox from '../../components/Checkbox';
import { isPresent } from '../../utils';
import { Rule } from '../../types/types';
import { useLocalStorage } from '../../hooks/useLocalStorage';
import CustomInfiniteScroll, { InfiniteScrollItemsProps } from '../../components/CustomInfiniteScroll';
import { KVSearch } from '@nexucis/kvsearch';
import SearchBar from '../../components/SearchBar';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type RuleState = keyof RuleStatus<any>;

export interface RuleStatus<T> {
  firing: T;
  pending: T;
  inactive: T;
}

export interface AlertsProps {
  groups?: RuleGroup[];
  statsCount: RuleStatus<number>;
}

export interface Alert {
  labels: Record<string, string>;
  state: RuleState;
  value: string;
  annotations: Record<string, string>;
  activeAt: string;
}

interface RuleGroup {
  name: string;
  file: string;
  rules: Rule[];
  interval: number;
}

const kvSearchRule = new KVSearch<Rule>({
  shouldSort: true,
  indexedKeys: ['name', 'labels', ['labels', /.*/]],
});

const stateColorTuples: Array<[RuleState, 'success' | 'warning' | 'danger']> = [
  ['inactive', 'success'],
  ['pending', 'warning'],
  ['firing', 'danger'],
];

function GroupContent(showAnnotations: boolean) {
  const Content: FC<InfiniteScrollItemsProps<Rule>> = ({ items }) => {
    return (
      <>
        {items.map((rule, j) => (
          <CollapsibleAlertPanel key={rule.name + j} showAnnotations={showAnnotations} rule={rule} />
        ))}
      </>
    );
  };
  return Content;
}

const AlertsContent: FC<AlertsProps> = ({ groups = [], statsCount }) => {
  const [groupList, setGroupList] = useState(groups);
  const [filteredList, setFilteredList] = useState(groups);
  const [filter, setFilter] = useLocalStorage('alerts-status-filter', {
    firing: true,
    pending: true,
    inactive: true,
  });
  const [showAnnotations, setShowAnnotations] = useLocalStorage('alerts-annotations-status', { checked: false });

  const toggleFilter = (ruleState: RuleState) => () => {
    setFilter({
      ...filter,
      [ruleState]: !filter[ruleState],
    });
  };

  const handleSearchChange = (e: ChangeEvent<HTMLTextAreaElement | HTMLInputElement>) => {
    if (e.target.value !== '') {
      const pattern = e.target.value.trim();
      const result: RuleGroup[] = [];
      for (const group of groups) {
        const ruleFilterList = kvSearchRule.filter(pattern, group.rules);
        if (ruleFilterList.length > 0) {
          result.push({
            file: group.file,
            name: group.name,
            interval: group.interval,
            rules: ruleFilterList.map((value) => value.original),
          });
        }
      }
      setGroupList(result);
    } else {
      setGroupList(groups);
    }
  };

  useEffect(() => {
    const result: RuleGroup[] = [];
    for (const group of groupList) {
      const newGroup = {
        file: group.file,
        name: group.name,
        interval: group.interval,
        rules: group.rules.filter((value) => filter[value.state]),
      };
      if (newGroup.rules.length > 0) {
        result.push(newGroup);
      }
    }
    setFilteredList(result);
  }, [groupList, filter]);

  return (
    <>
      <Row className="align-items-center">
        <Col className="d-flex" lg="4" md="5">
          {stateColorTuples.map(([state, color]) => {
            return (
              <Checkbox key={state} checked={filter[state]} id={`${state}-toggler`} onChange={toggleFilter(state)}>
                <Badge color={color} className="text-capitalize">
                  {state} ({statsCount[state]})
                </Badge>
              </Checkbox>
            );
          })}
        </Col>
        <Col lg="5" md="4">
          <SearchBar handleChange={handleSearchChange} placeholder="Filter by name or labels" />
        </Col>
        <Col className="d-flex flex-row-reverse" md="3">
          <Checkbox
            checked={showAnnotations.checked}
            id="show-annotations-toggler"
            onChange={({ target }) => setShowAnnotations({ checked: target.checked })}
          >
            <span style={{ fontSize: '0.9rem', lineHeight: 1.9, display: 'inline-block', whiteSpace: 'nowrap' }}>
              Show annotations
            </span>
          </Checkbox>
        </Col>
      </Row>
      {filteredList.map((group, i) => (
        <Fragment key={i}>
          <GroupInfo rules={group.rules}>
            {group.file} &gt; {group.name}
          </GroupInfo>
          <CustomInfiniteScroll allItems={group.rules} child={GroupContent(showAnnotations.checked)} />
        </Fragment>
      ))}
    </>
  );
};

interface GroupInfoProps {
  rules: Rule[];
}

export const GroupInfo: FC<GroupInfoProps> = ({ rules, children }) => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const statesCounter = rules.reduce<any>(
    (acc, r) => {
      return {
        ...acc,
        [r.state]: acc[r.state] + r.alerts.length,
      };
    },
    {
      firing: 0,
      pending: 0,
    }
  );

  return (
    <div className="group-info border rounded-sm" style={{ lineHeight: 1.1 }}>
      {children}
      <div className="badges-wrapper">
        {isPresent(statesCounter.inactive) && <Badge color="success">inactive</Badge>}
        {statesCounter.pending > 0 && <Badge color="warning">pending ({statesCounter.pending})</Badge>}
        {statesCounter.firing > 0 && <Badge color="danger">firing ({statesCounter.firing})</Badge>}
      </div>
    </div>
  );
};

AlertsContent.displayName = 'Alerts';

export default AlertsContent;

import {
  Accordion,
  Alert,
  Anchor,
  Group,
  RingProgress,
  Stack,
  Table,
  Text,
} from "@mantine/core";
import { KVSearch } from "@nexucis/kvsearch";
import { IconInfoCircle } from "@tabler/icons-react";
import { useSuspenseAPIQuery } from "../../api/api";
import {
  DroppedTarget,
  Labels,
  Target,
  TargetsResult,
} from "../../api/responseTypes/targets";
import { FC, useMemo } from "react";
import { useAppDispatch, useAppSelector } from "../../state/hooks";
import {
  setCollapsedPools,
  setShowLimitAlert,
} from "../../state/serviceDiscoveryPageSlice";
import CustomInfiniteScroll from "../../components/CustomInfiniteScroll";

import { useDebouncedValue, useLocalStorage } from "@mantine/hooks";
import { targetPoolDisplayLimit } from "./ServiceDiscoveryPage";
import { LabelBadges } from "../../components/LabelBadges";

type TargetLabels = {
  discoveredLabels: Labels;
  labels: Labels;
  isDropped: boolean;
};

type ScrapePool = {
  targets: TargetLabels[];
  active: number;
  total: number;
  // Can be different from "total" if the "keep_dropped_targets" setting is used
  // to limit the number of dropped targets for which the server keeps details.
  serverTotal: number;
};

type ScrapePools = {
  [scrapePool: string]: ScrapePool;
};

const activeTargetKVSearch = new KVSearch<Target>({
  shouldSort: true,
  indexedKeys: [
    "labels",
    "discoveredLabels",
    ["discoveredLabels", /.*/],
    ["labels", /.*/],
  ],
});

const droppedTargetKVSearch = new KVSearch<DroppedTarget>({
  shouldSort: true,
  indexedKeys: ["discoveredLabels", ["discoveredLabels", /.*/]],
});

const buildPoolsData = (
  poolNames: string[],
  targetsData: TargetsResult,
  search: string,
  stateFilter: (string | null)[]
): ScrapePools => {
  const { activeTargets, droppedTargets, droppedTargetCounts } = targetsData;
  const pools: ScrapePools = {};

  for (const pn of poolNames) {
    pools[pn] = {
      targets: [],
      active: 0,
      total: 0,
      serverTotal: droppedTargetCounts[pn] || 0,
    };
  }

  for (const target of activeTargets) {
    const pool = pools[target.scrapePool];
    if (!pool) {
      // TODO: Should we do better here?
      throw new Error(
        "Received target information for an unknown scrape pool, likely the list of scrape pools has changed. Please reload the page."
      );
    }

    pool.active++;
    pool.total++;
    pool.serverTotal++;
  }

  const filteredActiveTargets =
    stateFilter.length !== 0 && !stateFilter.includes("active")
      ? []
      : search === ""
        ? activeTargets
        : activeTargetKVSearch
            .filter(search, activeTargets)
            .map((value) => value.original);

  for (const target of filteredActiveTargets) {
    pools[target.scrapePool].targets.push({
      discoveredLabels: target.discoveredLabels,
      labels: target.labels,
      isDropped: false,
    });
  }

  for (const target of droppedTargets) {
    const pool = pools[target.scrapePool];
    if (!pool) {
      // TODO: Should we do better here?
      throw new Error(
        "Received target information for an unknown scrape pool, likely the list of scrape pools has changed. Please reload the page."
      );
    }

    pool.total++;
  }

  const filteredDroppedTargets =
    stateFilter.length !== 0 && !stateFilter.includes("dropped")
      ? []
      : search === ""
        ? droppedTargets
        : droppedTargetKVSearch
            .filter(search, droppedTargets)
            .map((value) => value.original);

  for (const target of filteredDroppedTargets) {
    pools[target.scrapePool].targets.push({
      discoveredLabels: target.discoveredLabels,
      isDropped: true,
      labels: {},
    });
  }

  return pools;
};

type ScrapePoolListProp = {
  poolNames: string[];
  selectedPool: string | null;
  stateFilter: string[];
  searchFilter: string;
};

const ScrapePoolList: FC<ScrapePoolListProp> = ({
  poolNames,
  selectedPool,
  stateFilter,
  searchFilter,
}) => {
  const dispatch = useAppDispatch();
  const [showEmptyPools, setShowEmptyPools] = useLocalStorage<boolean>({
    key: "serviceDiscoveryPage.showEmptyPools",
    defaultValue: false,
  });

  // Based on the selected pool (if any), load the list of targets.
  const {
    data: { data: targetsData },
  } = useSuspenseAPIQuery<TargetsResult>({
    path: `/targets`,
    params: {
      scrapePool: selectedPool === null ? "" : selectedPool,
    },
  });

  const { collapsedPools, showLimitAlert } = useAppSelector(
    (state) => state.serviceDiscoveryPage
  );

  const [debouncedSearch] = useDebouncedValue<string>(searchFilter, 250);

  const allPools = useMemo(
    () =>
      buildPoolsData(
        selectedPool ? [selectedPool] : poolNames,
        targetsData,
        debouncedSearch,
        stateFilter
      ),
    [selectedPool, poolNames, targetsData, debouncedSearch, stateFilter]
  );
  const allPoolNames = Object.keys(allPools);
  const shownPoolNames = showEmptyPools
    ? allPoolNames
    : allPoolNames.filter((pn) => allPools[pn].targets.length !== 0);

  return (
    <Stack>
      {allPoolNames.length === 0 ? (
        <Alert title="No scrape pools found" icon={<IconInfoCircle />}>
          No scrape pools found.
        </Alert>
      ) : (
        !showEmptyPools &&
        allPoolNames.length !== shownPoolNames.length && (
          <Alert
            title="Hiding pools with no matching targets"
            icon={<IconInfoCircle />}
          >
            Hiding {allPoolNames.length - shownPoolNames.length} empty pools due
            to filters or no targets.
            <Anchor ml="md" fz="1em" onClick={() => setShowEmptyPools(true)}>
              Show empty pools
            </Anchor>
          </Alert>
        )
      )}
      {showLimitAlert && (
        <Alert
          title="Found many pools, showing only one"
          icon={<IconInfoCircle />}
          withCloseButton
          onClose={() => dispatch(setShowLimitAlert(false))}
        >
          There are more than {targetPoolDisplayLimit} scrape pools. Showing
          only the first one. Use the dropdown to select a different pool.
        </Alert>
      )}
      <Accordion
        multiple
        variant="separated"
        value={allPoolNames.filter((p) => !collapsedPools.includes(p))}
        onChange={(value) =>
          dispatch(
            setCollapsedPools(allPoolNames.filter((p) => !value.includes(p)))
          )
        }
      >
        {shownPoolNames.map((poolName) => {
          const pool = allPools[poolName];
          return (
            <Accordion.Item key={poolName} value={poolName}>
              <Accordion.Control>
                <Group wrap="nowrap" justify="space-between" mr="lg">
                  <Text>{poolName}</Text>
                  <Group gap="xs">
                    <Text c="gray.6">
                      {pool.active} / {pool.serverTotal}
                    </Text>
                    <RingProgress
                      size={25}
                      thickness={5}
                      sections={
                        pool.serverTotal === 0
                          ? []
                          : [
                              {
                                value: (pool.active / pool.serverTotal) * 100,
                                color: "green.4",
                              },
                              {
                                value:
                                  ((pool.serverTotal - pool.active) /
                                    pool.serverTotal) *
                                  100,
                                color: "blue.6",
                              },
                            ]
                      }
                    />
                  </Group>
                </Group>
              </Accordion.Control>
              <Accordion.Panel>
                {pool.total !== pool.serverTotal && (
                  <Alert
                    title="Only showing partial dropped targets"
                    icon={<IconInfoCircle />}
                    color="yellow"
                    mb="sm"
                  >
                    {pool.serverTotal - pool.total} further dropped targets are
                    not shown here because the server only kept details on a
                    maximum of {pool.total - pool.active} dropped targets (
                    <code>keep_dropped_targets</code> configuration setting).
                  </Alert>
                )}
                {pool.total === 0 ? (
                  <Alert title="No targets" icon={<IconInfoCircle />}>
                    No targets in this scrape pool.
                    <Anchor
                      ml="md"
                      fz="1em"
                      onClick={() => setShowEmptyPools(false)}
                    >
                      Hide empty pools
                    </Anchor>
                  </Alert>
                ) : pool.targets.length === 0 ? (
                  <Alert title="No matching targets" icon={<IconInfoCircle />}>
                    No targets in this pool match your filter criteria (omitted{" "}
                    {pool.total} filtered targets).
                    <Anchor
                      ml="md"
                      fz="1em"
                      onClick={() => setShowEmptyPools(false)}
                    >
                      Hide empty pools
                    </Anchor>
                  </Alert>
                ) : (
                  <CustomInfiniteScroll
                    allItems={pool.targets}
                    child={({ items }) => (
                      <Table>
                        <Table.Thead>
                          <Table.Tr>
                            <Table.Th w="50%">Discovered labels</Table.Th>
                            <Table.Th w="50%">Target labels</Table.Th>
                          </Table.Tr>
                        </Table.Thead>
                        <Table.Tbody>
                          {items.map((target, i) => (
                            // TODO: Find a stable and definitely unique key.
                            <Table.Tr key={i}>
                              <Table.Td py="lg" valign="top">
                                <LabelBadges
                                  labels={target.discoveredLabels}
                                  wrapper={Stack}
                                />
                              </Table.Td>
                              <Table.Td
                                py="lg"
                                valign={target.isDropped ? "middle" : "top"}
                              >
                                {target.isDropped ? (
                                  <Text c="blue.6" fw="bold">
                                    dropped due to relabeling rules
                                  </Text>
                                ) : (
                                  <LabelBadges
                                    labels={target.labels}
                                    wrapper={Stack}
                                  />
                                )}
                              </Table.Td>
                            </Table.Tr>
                          ))}
                        </Table.Tbody>
                      </Table>
                    )}
                  />
                )}
              </Accordion.Panel>
            </Accordion.Item>
          );
        })}
      </Accordion>
    </Stack>
  );
};

export default ScrapePoolList;

import {
  Accordion,
  Alert,
  Anchor,
  Badge,
  Group,
  RingProgress,
  Stack,
  Table,
  Text,
} from "@mantine/core";
import { KVSearch } from "@nexucis/kvsearch";
import {
  IconAlertTriangle,
  IconInfoCircle,
} from "@tabler/icons-react";
import { useSuspenseAPIQuery } from "../../api/api";
import { Target, TargetsResult } from "../../api/responseTypes/targets";
import React, { FC, memo, useMemo } from "react";
import { useLocalStorage } from "@mantine/hooks";
import { useAppDispatch, useAppSelector } from "../../state/hooks";
import {
  setCollapsedPools,
  setShowLimitAlert,
} from "../../state/targetsPageSlice";
import EndpointLink from "../../components/EndpointLink";
import CustomInfiniteScroll from "../../components/CustomInfiniteScroll";

import badgeClasses from "../../Badge.module.css";
import panelClasses from "../../Panel.module.css";
import TargetLabels from "./TargetLabels";
import ScrapeTimingDetails from "./ScrapeTimingDetails";
import { targetPoolDisplayLimit } from "./TargetsPage";

type ScrapePool = {
  targets: Target[];
  count: number;
  upCount: number;
  downCount: number;
  unknownCount: number;
};

type ScrapePools = {
  [scrapePool: string]: ScrapePool;
};

const poolPanelHealthClass = (pool: ScrapePool) =>
  pool.count > 0 && pool.downCount === pool.count
    ? panelClasses.panelHealthErr
    : pool.downCount >= 1
      ? panelClasses.panelHealthWarn
      : pool.unknownCount === pool.count
        ? panelClasses.panelHealthUnknown
        : panelClasses.panelHealthOk;

const healthBadgeClass = (state: string) => {
  switch (state.toLowerCase()) {
    case "up":
      return badgeClasses.healthOk;
    case "down":
      return badgeClasses.healthErr;
    case "unknown":
      return badgeClasses.healthUnknown;
    default:
      // Should never happen.
      return badgeClasses.healthWarn;
  }
};

const kvSearch = new KVSearch<Target>({
  shouldSort: true,
  indexedKeys: ["labels", "scrapePool", ["labels", /.*/]],
});

const buildPoolsData = (
  poolNames: string[],
  targets: Target[],
  search: string,
  healthFilter: (string | null)[]
): ScrapePools => {
  const pools: ScrapePools = {};

  for (const pn of poolNames) {
    pools[pn] = {
      targets: [],
      count: 0,
      upCount: 0,
      downCount: 0,
      unknownCount: 0,
    };
  }

  for (const target of targets) {
    if (!pools[target.scrapePool]) {
      // TODO: Should we do better here?
      throw new Error(
        "Received target information for an unknown scrape pool, likely the list of scrape pools has changed. Please reload the page."
      );
    }

    pools[target.scrapePool].count++;

    switch (target.health.toLowerCase()) {
      case "up":
        pools[target.scrapePool].upCount++;
        break;
      case "down":
        pools[target.scrapePool].downCount++;
        break;
      case "unknown":
        pools[target.scrapePool].unknownCount++;
        break;
    }
  }

  const filteredTargets: Target[] = (
    search === ""
      ? targets
      : kvSearch.filter(search, targets).map((value) => value.original)
  ).filter(
    (target) =>
      healthFilter.length === 0 || healthFilter.includes(target.health)
  );

  for (const target of filteredTargets) {
    pools[target.scrapePool].targets.push(target);
  }

  return pools;
};

type ScrapePoolListProp = {
  poolNames: string[];
  selectedPool: string | null;
  healthFilter: string[];
  searchFilter: string;
};

const ScrapePoolList: FC<ScrapePoolListProp> = memo(
  ({ poolNames, selectedPool, healthFilter, searchFilter }) => {
    // Based on the selected pool (if any), load the list of targets.
    const {
      data: {
        data: { activeTargets },
      },
    } = useSuspenseAPIQuery<TargetsResult>({
      path: `/targets`,
      params: {
        state: "active",
        scrapePool: selectedPool === null ? "" : selectedPool,
      },
    });

    const dispatch = useAppDispatch();
    const [showEmptyPools, setShowEmptyPools] = useLocalStorage<boolean>({
      key: "targetsPage.showEmptyPools",
      defaultValue: false,
    });

    const { collapsedPools, showLimitAlert } = useAppSelector(
      (state) => state.targetsPage
    );

    const allPools = useMemo(
      () =>
        buildPoolsData(
          selectedPool ? [selectedPool] : poolNames,
          activeTargets,
          searchFilter,
          healthFilter
        ),
      [selectedPool, poolNames, activeTargets, searchFilter, healthFilter]
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
              Hiding {allPoolNames.length - shownPoolNames.length} empty pools
              due to filters or no targets.
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
              <Accordion.Item
                key={poolName}
                value={poolName}
                className={poolPanelHealthClass(pool)}
              >
                <Accordion.Control>
                  <Group wrap="nowrap" justify="space-between" mr="lg">
                    <Text>{poolName}</Text>
                    <Group gap="xs">
                      <Text c="gray.6">
                        {pool.upCount} / {pool.count} up
                      </Text>
                      <RingProgress
                        size={25}
                        thickness={5}
                        sections={
                          pool.count === 0
                            ? []
                            : [
                                {
                                  value: (pool.upCount / pool.count) * 100,
                                  color: "green.4",
                                },
                                {
                                  value: (pool.unknownCount / pool.count) * 100,
                                  color: "gray.4",
                                },
                                {
                                  value: (pool.downCount / pool.count) * 100,
                                  color: "red.5",
                                },
                              ]
                        }
                      />
                    </Group>
                  </Group>
                </Accordion.Control>
                <Accordion.Panel>
                  {pool.count === 0 ? (
                    <Alert title="No targets" icon={<IconInfoCircle />}>
                      No active targets in this scrape pool.
                      <Anchor
                        ml="md"
                        fz="1em"
                        onClick={() => setShowEmptyPools(false)}
                      >
                        Hide empty pools
                      </Anchor>
                    </Alert>
                  ) : pool.targets.length === 0 ? (
                    <Alert
                      title="No matching targets"
                      icon={<IconInfoCircle />}
                    >
                      No targets in this pool match your filter criteria
                      (omitted {pool.count} filtered targets).
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
                              <Table.Th w="25%">Endpoint</Table.Th>
                              <Table.Th>Labels</Table.Th>
                              <Table.Th w={230}>Last scrape</Table.Th>
                              <Table.Th w={100}>State</Table.Th>
                            </Table.Tr>
                          </Table.Thead>
                          <Table.Tbody>
                            {items.map((target, i) => (
                              // TODO: Find a stable and definitely unique key.
                              <React.Fragment key={i}>
                                <Table.Tr
                                  style={{
                                    borderBottom: target.lastError
                                      ? "none"
                                      : undefined,
                                  }}
                                >
                                  <Table.Td valign="top">
                                    <EndpointLink
                                      endpoint={target.scrapeUrl}
                                      globalUrl={target.globalUrl}
                                    />
                                  </Table.Td>

                                  <Table.Td valign="top">
                                    <TargetLabels
                                      labels={target.labels}
                                      discoveredLabels={target.discoveredLabels}
                                    />
                                  </Table.Td>
                                  <Table.Td valign="top">
                                    <ScrapeTimingDetails target={target} />
                                  </Table.Td>
                                  <Table.Td valign="top">
                                    <Badge
                                      className={healthBadgeClass(
                                        target.health
                                      )}
                                    >
                                      {target.health}
                                    </Badge>
                                  </Table.Td>
                                </Table.Tr>
                                {target.lastError && (
                                  <Table.Tr>
                                    <Table.Td colSpan={5} valign="top">
                                      <Alert
                                        color="red"
                                        mb="sm"
                                        icon={<IconAlertTriangle />}
                                      >
                                        <strong>Error scraping target:</strong>{" "}
                                        {target.lastError}
                                      </Alert>
                                    </Table.Td>
                                  </Table.Tr>
                                )}
                              </React.Fragment>
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
  }
);

export default ScrapePoolList;

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
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import { useSuspenseAPIQuery } from "../../api/api";
import { Target, TargetsResult } from "../../api/responseTypes/targets";
import React, { FC, useState } from "react";
import badgeClasses from "../../Badge.module.css";
import {
  humanizeDurationRelative,
  humanizeDuration,
  now,
} from "../../lib/formatTime";
import { LabelBadges } from "../../components/LabelBadges";
import { useAppDispatch, useAppSelector } from "../../state/hooks";
import { setCollapsedPools } from "../../state/targetsPageSlice";
import EndpointLink from "../../components/EndpointLink";
import CustomInfiniteScroll from "../../components/CustomInfiniteScroll";

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

const healthBadgeClass = (state: string) => {
  switch (state.toLowerCase()) {
    case "up":
      return badgeClasses.healthOk;
    case "down":
      return badgeClasses.healthErr;
    case "unknown":
      return badgeClasses.healthUnknown;
    default:
      return badgeClasses.warn;
  }
};

const kvSearch = new KVSearch<Target>({
  shouldSort: true,
  indexedKeys: ["labels", "scrapePool", ["labels", /.*/]],
});

const groupTargets = (
  poolNames: string[],
  allTargets: Target[],
  shownTargets: Target[]
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

  for (const target of allTargets) {
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

  for (const target of shownTargets) {
    pools[target.scrapePool].targets.push(target);
  }

  return pools;
};

type ScrapePoolListProp = {
  poolNames: string[];
  selectedPool: string | null;
  limited: boolean;
};

const ScrapePoolList: FC<ScrapePoolListProp> = ({
  poolNames,
  selectedPool,
  limited,
}) => {
  const dispatch = useAppDispatch();
  const [showEmptyPools, setShowEmptyPools] = useState(true);

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

  const { healthFilter, searchFilter, collapsedPools } = useAppSelector(
    (state) => state.targetsPage
  );

  const search = searchFilter.trim();
  const healthFilteredTargets = activeTargets.filter(
    (target) =>
      healthFilter.length === 0 ||
      healthFilter.includes(target.health.toLowerCase())
  );
  const filteredTargets =
    search === ""
      ? healthFilteredTargets
      : kvSearch
          .filter(searchFilter, healthFilteredTargets)
          .map((value) => value.original);

  const allPools = groupTargets(
    selectedPool ? [selectedPool] : poolNames,
    activeTargets,
    filteredTargets
  );
  const allPoolNames = Object.keys(allPools);
  const shownPoolNames = showEmptyPools
    ? allPoolNames
    : allPoolNames.filter((pn) => allPools[pn].targets.length !== 0);

  return (
    <Stack>
      {allPoolNames.length === 0 ? (
        <Alert
          title="No scrape pools found"
          icon={<IconInfoCircle size={14} />}
        >
          No scrape pools found.
        </Alert>
      ) : (
        !showEmptyPools &&
        allPoolNames.length !== shownPoolNames.length && (
          <Alert
            title="Hiding pools with no matching targets"
            icon={<IconInfoCircle size={14} />}
          >
            Hiding {allPoolNames.length - shownPoolNames.length} empty pools due
            to filters or no targets.
            <Anchor ml="md" fz="1em" onClick={() => setShowEmptyPools(true)}>
              Show empty pools
            </Anchor>
          </Alert>
        )
      )}
      {limited && (
        <Alert
          title="Found many pools, showing only one"
          icon={<IconInfoCircle size={14} />}
        >
          There are more than 20 scrape pools. Showing only the first one. Use
          the dropdown to select a different pool.
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
              style={{
                borderLeft:
                  pool.upCount === 0
                    ? "5px solid var(--mantine-color-red-4)"
                    : pool.upCount !== pool.count
                      ? "5px solid var(--mantine-color-orange-5)"
                      : "5px solid var(--mantine-color-green-4)",
              }}
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
                      sections={[
                        {
                          value: (pool.upCount / pool.count) * 100,
                          color: "green.4",
                        },
                        {
                          value: (pool.downCount / pool.count) * 100,
                          color: "red.5",
                        },
                        {
                          value: (pool.unknownCount / pool.count) * 100,
                          color: "gray.4",
                        },
                      ]}
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
                  <Alert title="No matching targets" icon={<IconInfoCircle />}>
                    No targets in this pool match your filter criteria (omitted{" "}
                    {pool.count} filtered targets).
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
                            <Table.Th w="30%">Endpoint</Table.Th>
                            <Table.Th w={100}>State</Table.Th>
                            <Table.Th>Labels</Table.Th>
                            <Table.Th w="10%">Last scrape</Table.Th>
                            <Table.Th w="10%">Scrape duration</Table.Th>
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
                                <Table.Td>
                                  {/* TODO: Process target URL like in old UI */}
                                  <EndpointLink
                                    endpoint={target.scrapeUrl}
                                    globalUrl={target.globalUrl}
                                  />
                                </Table.Td>
                                <Table.Td>
                                  <Badge
                                    className={healthBadgeClass(target.health)}
                                  >
                                    {target.health}
                                  </Badge>
                                </Table.Td>
                                <Table.Td>
                                  <LabelBadges labels={target.labels} />
                                </Table.Td>
                                <Table.Td>
                                  {humanizeDurationRelative(
                                    target.lastScrape,
                                    now()
                                  )}
                                </Table.Td>
                                <Table.Td>
                                  {humanizeDuration(
                                    target.lastScrapeDuration * 1000
                                  )}
                                </Table.Td>
                              </Table.Tr>
                              {target.lastError && (
                                <Table.Tr>
                                  <Table.Td colSpan={5}>
                                    <Alert
                                      color="red"
                                      mb="sm"
                                      icon={<IconAlertTriangle size={14} />}
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
};

export default ScrapePoolList;

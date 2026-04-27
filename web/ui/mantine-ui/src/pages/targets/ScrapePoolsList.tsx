import {
  Accordion,
  Alert,
  Anchor,
  Badge,
  Button,
  Group,
  Modal,
  RingProgress,
  Stack,
  Table,
  Text,
  Textarea,
} from "@mantine/core";
import { KVSearch } from "@nexucis/kvsearch";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import { useSuspenseAPIQuery } from "../../api/api";
import { Target, TargetsResult } from "../../api/responseTypes/targets";
import React, { FC, memo, useMemo, useState } from "react";
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
import { Accordion } from "../../components/Accordion";
import { useSettings } from "../../state/settingsSlice";

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

const MetricsModal: FC<{
  opened: boolean;
  onClose: () => void;
  title: string;
  loading: boolean;
  error: string | null;
  body: string;
  onCopy: () => void;
  onDownload: () => void;
}> = ({ opened, onClose, title, loading, error, body, onCopy, onDownload }) => {
  return (
    <Modal opened={opened} onClose={onClose} title={title} size="xl">
      <Group mb="sm" justify="flex-end">
        <Button
          size="xs"
          variant="light"
          disabled={loading || body === ""}
          onClick={onCopy}
        >
          Copy
        </Button>
        <Button
          size="xs"
          variant="light"
          disabled={loading || body === ""}
          onClick={onDownload}
        >
          Download
        </Button>
      </Group>
      {error && (
        <Alert mb="sm" color="red" icon={<IconAlertTriangle />}>
          {error}
        </Alert>
      )}
      <Textarea
        value={loading ? "Loading…" : body}
        readOnly
        autosize
        minRows={12}
        maxRows={28}
        styles={{ input: { fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace" } }}
      />
    </Modal>
  );
};

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
    const { enableTargetScrapeProxy, pathPrefix } = useSettings();
    const [showEmptyPools, setShowEmptyPools] = useLocalStorage<boolean>({
      key: "targetsPage.showEmptyPools",
      defaultValue: false,
    });

    const [metricsModalOpen, setMetricsModalOpen] = useState(false);
    const [metricsModalTitle, setMetricsModalTitle] = useState("");
    const [metricsLoading, setMetricsLoading] = useState(false);
    const [metricsError, setMetricsError] = useState<string | null>(null);
    const [metricsBody, setMetricsBody] = useState("");

    const openMetrics = async (t: Target) => {
      if (!enableTargetScrapeProxy) {
        return;
      }
      setMetricsModalTitle(`${t.scrapePool} — ${t.scrapeUrl}`);
      setMetricsBody("");
      setMetricsError(null);
      setMetricsLoading(true);
      setMetricsModalOpen(true);

      try {
        const params = new URLSearchParams({
          scrapePool: t.scrapePool,
          scrapeUrl: t.scrapeUrl,
        });
        const resp = await fetch(
          `${pathPrefix}/api/v1/targets/scrape?${params.toString()}`,
          { method: "GET" }
        );
        const text = await resp.text();
        if (!resp.ok) {
          throw new Error(text || resp.statusText);
        }
        setMetricsBody(text);
      } catch (err) {
        setMetricsError(err instanceof Error ? err.message : String(err));
      } finally {
        setMetricsLoading(false);
      }
    };

    const copyMetrics = async () => {
      if (!metricsBody) return;
      await navigator.clipboard.writeText(metricsBody);
    };

    const downloadMetrics = () => {
      const blob = new Blob([metricsBody], { type: "text/plain;charset=utf-8" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "metrics.txt";
      a.click();
      URL.revokeObjectURL(url);
    };

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
        <MetricsModal
          opened={metricsModalOpen}
          onClose={() => setMetricsModalOpen(false)}
          title={metricsModalTitle}
          loading={metricsLoading}
          error={metricsError}
          body={metricsBody}
          onCopy={copyMetrics}
          onDownload={downloadMetrics}
        />
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
          keepMounted={false}
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
                              {enableTargetScrapeProxy && (
                                <Table.Th w={140}>Metrics</Table.Th>
                              )}
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
                                  {enableTargetScrapeProxy && (
                                    <Table.Td valign="top">
                                      <Button
                                        size="xs"
                                        variant="light"
                                        onClick={() => openMetrics(target)}
                                      >
                                        View metrics
                                      </Button>
                                    </Table.Td>
                                  )}
                                </Table.Tr>
                                {target.lastError && (
                                  <Table.Tr>
                                    <Table.Td colSpan={enableTargetScrapeProxy ? 5 : 4} valign="top">
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

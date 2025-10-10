import {
  Card,
  Group,
  Table,
  Text,
  Accordion,
  Badge,
  Tooltip,
  Box,
  Stack,
  Alert,
  TextInput,
  Anchor,
  Pagination,
  rem,
} from "@mantine/core";
import { useSuspenseAPIQuery } from "../api/api";
import { AlertingRule, AlertingRulesResult } from "../api/responseTypes/rules";
import badgeClasses from "../Badge.module.css";
import panelClasses from "../Panel.module.css";
import RuleDefinition from "../components/RuleDefinition";
import { humanizeDurationRelative, now } from "../lib/formatTime";
import { Fragment, useEffect, useMemo } from "react";
import { StateMultiSelect } from "../components/StateMultiSelect";
import { IconInfoCircle, IconSearch } from "@tabler/icons-react";
import { LabelBadges } from "../components/LabelBadges";
import { useLocalStorage } from "@mantine/hooks";
import { useSettings } from "../state/settingsSlice";
import {
  ArrayParam,
  NumberParam,
  StringParam,
  useQueryParam,
  withDefault,
} from "use-query-params";
import { useDebouncedValue } from "@mantine/hooks";
import { KVSearch } from "@nexucis/kvsearch";
import { inputIconStyle } from "../styles";
import CustomInfiniteScroll from "../components/CustomInfiniteScroll";
import classes from "./AlertsPage.module.css";

type AlertsPageData = {
  // How many rules are in each state across all groups.
  globalCounts: {
    inactive: number;
    pending: number;
    firing: number;
    unknown: number;
  };
  groups: {
    name: string;
    file: string;
    // How many rules are in each state for this group.
    counts: {
      total: number;
      inactive: number;
      pending: number;
      firing: number;
      unknown: number;
    };
    rules: {
      rule: AlertingRule;
      // How many alerts are in each state for this rule.
      counts: {
        firing: number;
        pending: number;
      };
    }[];
  }[];
};

const kvSearch = new KVSearch<AlertingRule>({
  shouldSort: true,
  indexedKeys: ["name", "labels", ["labels", /.*/]],
});

const buildAlertsPageData = (
  data: AlertingRulesResult,
  search: string,
  stateFilter: (string | null)[]
) => {
  const pageData: AlertsPageData = {
    globalCounts: {
      inactive: 0,
      pending: 0,
      firing: 0,
      unknown: 0,
    },
    groups: [],
  };

  for (const group of data.groups) {
    const groupCounts = {
      total: 0,
      inactive: 0,
      pending: 0,
      firing: 0,
      unknown: 0,
    };

    for (const r of group.rules) {
      groupCounts.total++;
      switch (r.state) {
        case "inactive":
          pageData.globalCounts.inactive++;
          groupCounts.inactive++;
          break;
        case "firing":
          pageData.globalCounts.firing++;
          groupCounts.firing++;
          break;
        case "pending":
          pageData.globalCounts.pending++;
          groupCounts.pending++;
          break;
        case "unknown":
          pageData.globalCounts.unknown++;
          groupCounts.unknown++;
          break;
        default:
          throw new Error(`Unknown rule state: ${r.state}`);
      }
    }

    const filteredRules: AlertingRule[] = (
      search === ""
        ? group.rules
        : kvSearch.filter(search, group.rules).map((value) => value.original)
    ).filter((r) => stateFilter.length === 0 || stateFilter.includes(r.state));

    pageData.groups.push({
      name: group.name,
      file: group.file,
      counts: groupCounts,
      rules: filteredRules.map((r) => ({
        rule: r,
        counts: {
          firing: r.alerts.filter((a) => a.state === "firing").length,
          pending: r.alerts.filter((a) => a.state === "pending").length,
        },
      })),
    });
  }

  return pageData;
};

// Should be defined as a constant here instead of inline as a value
// to avoid unnecessary re-renders. Otherwise the empty array has
// a different reference on each render and causes subsequent memoized
// computations to re-run as long as no state filter is selected.
const emptyStateFilter: string[] = [];

export default function AlertsPage() {
  // Fetch the alerting rules data.
  const { data } = useSuspenseAPIQuery<AlertingRulesResult>({
    path: `/rules`,
    params: {
      type: "alert",
    },
  });

  const { showAnnotations } = useSettings();

  // Define URL query params.
  const [stateFilter, setStateFilter] = useQueryParam(
    "state",
    withDefault(ArrayParam, emptyStateFilter)
  );
  const [searchFilter, setSearchFilter] = useQueryParam(
    "search",
    withDefault(StringParam, "")
  );
  const [debouncedSearch] = useDebouncedValue<string>(searchFilter.trim(), 250);
  const [showEmptyGroups, setShowEmptyGroups] = useLocalStorage<boolean>({
    key: "alertsPage.showEmptyGroups",
    defaultValue: false,
  });

  const { alertGroupsPerPage } = useSettings();
  const [activePage, setActivePage] = useQueryParam(
    "page",
    withDefault(NumberParam, 1)
  );

  // Update the page data whenever the fetched data or filters change.
  const alertsPageData: AlertsPageData = useMemo(
    () => buildAlertsPageData(data.data, debouncedSearch, stateFilter),
    [data, stateFilter, debouncedSearch]
  );

  const shownGroups = useMemo(
    () =>
      showEmptyGroups
        ? alertsPageData.groups
        : alertsPageData.groups.filter((g) => g.rules.length > 0),
    [alertsPageData.groups, showEmptyGroups]
  );

  // If we were e.g. on page 10 and the number of total pages decreases to 5 (due to filtering
  // or changing the max number of items per page), go to the largest possible page.
  const totalPageCount = Math.ceil(shownGroups.length / alertGroupsPerPage);
  const effectiveActivePage = Math.max(1, Math.min(activePage, totalPageCount));

  useEffect(() => {
    if (effectiveActivePage !== activePage) {
      setActivePage(effectiveActivePage);
    }
  }, [effectiveActivePage, activePage, setActivePage]);

  const currentPageGroups = useMemo(
    () =>
      shownGroups.slice(
        (effectiveActivePage - 1) * alertGroupsPerPage,
        effectiveActivePage * alertGroupsPerPage
      ),
    [shownGroups, effectiveActivePage, alertGroupsPerPage]
  );

  // We memoize the actual rendering of the page items to avoid re-rendering
  // them on every state change. This is especially important when the user
  // types into the search box, as the search filter changes on every keystroke,
  // even before debouncing takes place (extracting the filters and results list
  // into separate components would be an alternative to this, but it's kinda
  // convenient to have in the same file IMO).
  const renderedPageItems = useMemo(
    () =>
      currentPageGroups.map((g) => (
        <Card shadow="xs" withBorder p="md" key={`${g.file}-${g.name}`}>
          <Group mb="sm" justify="space-between">
            <Group align="baseline">
              <Text fz="xl" fw={600} c="var(--mantine-primary-color-filled)">
                {g.name}
              </Text>
              <Text fz="sm" c="gray.6">
                {g.file}
              </Text>
            </Group>
            <Group>
              {g.counts.firing > 0 && (
                <Badge className={badgeClasses.healthErr}>
                  firing ({g.counts.firing})
                </Badge>
              )}
              {g.counts.pending > 0 && (
                <Badge className={badgeClasses.healthWarn}>
                  pending ({g.counts.pending})
                </Badge>
              )}
              {g.counts.unknown > 0 && (
                <Badge className={badgeClasses.healthUnknown}>
                  unknown ({g.counts.unknown})
                </Badge>
              )}
              {g.counts.inactive > 0 && (
                <Badge className={badgeClasses.healthOk}>
                  inactive ({g.counts.inactive})
                </Badge>
              )}
            </Group>
          </Group>
          {g.counts.total === 0 ? (
            <Alert title="No rules" icon={<IconInfoCircle />}>
              No rules in this group.
              <Anchor
                ml="md"
                fz="1em"
                onClick={() => setShowEmptyGroups(false)}
              >
                Hide empty groups
              </Anchor>
            </Alert>
          ) : g.rules.length === 0 ? (
            <Alert title="No matching rules" icon={<IconInfoCircle />}>
              No rules in this group match your filter criteria (omitted{" "}
              {g.counts.total} filtered rules).
              <Anchor
                ml="md"
                fz="1em"
                onClick={() => setShowEmptyGroups(false)}
              >
                Hide empty groups
              </Anchor>
            </Alert>
          ) : (
            <CustomInfiniteScroll
              allItems={g.rules}
              child={({ items }) => (
                <Accordion multiple variant="separated" classNames={classes}>
                  {items.map((r, j) => {
                    return (
                      <Accordion.Item
                        mt={rem(5)}
                        key={j}
                        value={j.toString()}
                        className={
                          r.counts.firing > 0
                            ? panelClasses.panelHealthErr
                            : r.counts.pending > 0
                              ? panelClasses.panelHealthWarn
                              : r.rule.state === "unknown"
                                ? panelClasses.panelHealthUnknown
                                : panelClasses.panelHealthOk
                        }
                      >
                        <Accordion.Control
                          styles={{ label: { paddingBlock: rem(10) } }}
                        >
                          <Group wrap="nowrap" justify="space-between" mr="lg">
                            <Text>{r.rule.name}</Text>
                            <Group gap="xs">
                              {r.counts.firing > 0 && (
                                <Badge className={badgeClasses.healthErr}>
                                  firing ({r.counts.firing})
                                </Badge>
                              )}
                              {r.counts.pending > 0 && (
                                <Badge className={badgeClasses.healthWarn}>
                                  pending ({r.counts.pending})
                                </Badge>
                              )}
                            </Group>
                          </Group>
                        </Accordion.Control>
                        <Accordion.Panel>
                          <RuleDefinition rule={r.rule} />
                          {r.rule.alerts.length > 0 && (
                            <Table mt="lg">
                              <Table.Thead>
                                <Table.Tr style={{ whiteSpace: "nowrap" }}>
                                  <Table.Th>Alert labels</Table.Th>
                                  <Table.Th>State</Table.Th>
                                  <Table.Th>Active Since</Table.Th>
                                  <Table.Th>Value</Table.Th>
                                </Table.Tr>
                              </Table.Thead>
                              <Table.Tbody>
                                {r.rule.type === "alerting" &&
                                  r.rule.alerts.map((a, k) => (
                                    <Fragment key={k}>
                                      <Table.Tr>
                                        <Table.Td>
                                          <LabelBadges labels={a.labels} />
                                        </Table.Td>
                                        <Table.Td>
                                          <Badge
                                            className={
                                              a.state === "firing"
                                                ? badgeClasses.healthErr
                                                : badgeClasses.healthWarn
                                            }
                                          >
                                            {a.state}
                                          </Badge>
                                        </Table.Td>
                                        <Table.Td
                                          style={{ whiteSpace: "nowrap" }}
                                        >
                                          <Tooltip label={a.activeAt}>
                                            <Box>
                                              {humanizeDurationRelative(
                                                a.activeAt,
                                                now(),
                                                ""
                                              )}
                                            </Box>
                                          </Tooltip>
                                        </Table.Td>
                                        <Table.Td
                                          style={{ whiteSpace: "nowrap" }}
                                        >
                                          {isNaN(Number(a.value))
                                            ? a.value
                                            : Number(a.value)}
                                        </Table.Td>
                                      </Table.Tr>
                                      {showAnnotations && (
                                        <Table.Tr>
                                          <Table.Td colSpan={4}>
                                            <Table mt="md" mb="xl">
                                              <Table.Tbody>
                                                {Object.entries(
                                                  a.annotations
                                                ).map(([k, v]) => (
                                                  <Table.Tr key={k}>
                                                    <Table.Th c="light-dark(var(--mantine-color-gray-7), var(--mantine-color-gray-4))">
                                                      {k}
                                                    </Table.Th>
                                                    <Table.Td>{v}</Table.Td>
                                                  </Table.Tr>
                                                ))}
                                              </Table.Tbody>
                                            </Table>
                                          </Table.Td>
                                        </Table.Tr>
                                      )}
                                    </Fragment>
                                  ))}
                              </Table.Tbody>
                            </Table>
                          )}
                        </Accordion.Panel>
                      </Accordion.Item>
                    );
                  })}
                </Accordion>
              )}
            />
          )}
        </Card>
      )),
    [currentPageGroups, showAnnotations, setShowEmptyGroups]
  );

  return (
    <Stack mt="xs">
      <Group>
        <StateMultiSelect
          options={["inactive", "pending", "firing", "unknown"]}
          optionClass={(o) =>
            o === "inactive"
              ? badgeClasses.healthOk
              : o === "pending"
                ? badgeClasses.healthWarn
                : o === "firing"
                  ? badgeClasses.healthErr
                  : badgeClasses.healthUnknown
          }
          optionCount={(o) =>
            alertsPageData.globalCounts[
              o as keyof typeof alertsPageData.globalCounts
            ]
          }
          placeholder="Filter by rule group state"
          values={(stateFilter?.filter((v) => v !== null) as string[]) || []}
          onChange={(values) => setStateFilter(values)}
        />
        <TextInput
          flex={1}
          leftSection={<IconSearch style={inputIconStyle} />}
          placeholder="Filter by rule name or labels"
          value={searchFilter || ""}
          onChange={(event) =>
            setSearchFilter(event.currentTarget.value || null)
          }
        ></TextInput>
      </Group>
      {alertsPageData.groups.length === 0 ? (
        <Alert title="No rules found" icon={<IconInfoCircle />}>
          No rules found.
        </Alert>
      ) : (
        !showEmptyGroups &&
        alertsPageData.groups.length !== shownGroups.length && (
          <Alert
            title="Hiding groups with no matching rules"
            icon={<IconInfoCircle />}
          >
            Hiding {alertsPageData.groups.length - shownGroups.length} empty
            groups due to filters or no rules.
            <Anchor ml="md" fz="1em" onClick={() => setShowEmptyGroups(true)}>
              Show empty groups
            </Anchor>
          </Alert>
        )
      )}
      <Pagination
        total={totalPageCount}
        value={effectiveActivePage}
        onChange={setActivePage}
        hideWithOnePage
      />
      {renderedPageItems}
    </Stack>
  );
}

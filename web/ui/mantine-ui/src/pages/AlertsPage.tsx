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
} from "@mantine/core";
import { useSuspenseAPIQuery } from "../api/api";
import { AlertingRule, AlertingRulesResult } from "../api/responseTypes/rules";
import badgeClasses from "../Badge.module.css";
import panelClasses from "../Panel.module.css";
import RuleDefinition from "../components/RuleDefinition";
import { humanizeDurationRelative, now } from "../lib/formatTime";
import { Fragment, useMemo } from "react";
import { StateMultiSelect } from "../components/StateMultiSelect";
import { IconInfoCircle, IconSearch } from "@tabler/icons-react";
import { LabelBadges } from "../components/LabelBadges";
import { useSettings } from "../state/settingsSlice";
import {
  ArrayParam,
  BooleanParam,
  StringParam,
  useQueryParam,
  withDefault,
} from "use-query-params";
import { useDebouncedValue } from "@mantine/hooks";
import { KVSearch } from "@nexucis/kvsearch";
import { inputIconStyle } from "../styles";

type AlertsPageData = {
  // How many rules are in each state across all groups.
  globalCounts: {
    inactive: number;
    pending: number;
    firing: number;
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
    },
    groups: [],
  };

  for (const group of data.groups) {
    const groupCounts = {
      total: 0,
      inactive: 0,
      pending: 0,
      firing: 0,
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
    withDefault(ArrayParam, [])
  );
  const [searchFilter, setSearchFilter] = useQueryParam(
    "search",
    withDefault(StringParam, "")
  );
  const [debouncedSearch] = useDebouncedValue<string>(searchFilter.trim(), 250);
  const [showEmptyGroups, setShowEmptyGroups] = useQueryParam(
    "showEmptyGroups",
    withDefault(BooleanParam, true)
  );

  // Update the page data whenever the fetched data or filters change.
  const alertsPageData: AlertsPageData = useMemo(
    () => buildAlertsPageData(data.data, debouncedSearch, stateFilter),
    [data, stateFilter, debouncedSearch]
  );

  const shownGroups = showEmptyGroups
    ? alertsPageData.groups
    : alertsPageData.groups.filter((g) => g.rules.length > 0);

  return (
    <Stack mt="xs">
      <Group>
        <StateMultiSelect
          options={["inactive", "pending", "firing"]}
          optionClass={(o) =>
            o === "inactive"
              ? badgeClasses.healthOk
              : o === "pending"
                ? badgeClasses.healthWarn
                : badgeClasses.healthErr
          }
          optionCount={(o) =>
            alertsPageData.globalCounts[
              o as keyof typeof alertsPageData.globalCounts
            ]
          }
          placeholder="Filter by rule state"
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
            icon={<IconInfoCircle/>}
          >
            Hiding {alertsPageData.groups.length - shownGroups.length} empty
            groups due to filters or no rules.
            <Anchor ml="md" fz="1em" onClick={() => setShowEmptyGroups(true)}>
              Show empty groups
            </Anchor>
          </Alert>
        )
      )}
      <Stack>
        {shownGroups.map((g, i) => {
          return (
            <Card
              shadow="xs"
              withBorder
              p="md"
              key={i} // TODO: Find a stable and definitely unique key.
            >
              <Group mb="md" mt="xs" ml="xs" justify="space-between">
                <Group align="baseline">
                  <Text
                    fz="xl"
                    fw={600}
                    c="var(--mantine-primary-color-filled)"
                  >
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
                <Accordion multiple variant="separated">
                  {g.rules.map((r, j) => {
                    return (
                      <Accordion.Item
                        styles={{
                          item: {
                            // TODO: This transparency hack is an OK workaround to make the collapsed items
                            // have a different background color than their surrounding group card in dark mode,
                            // but it would be better to use CSS to override the light/dark colors for
                            // collapsed/expanded accordion items.
                            backgroundColor: "#c0c0c015",
                          },
                        }}
                        key={j}
                        value={j.toString()}
                        className={
                          r.counts.firing > 0
                            ? panelClasses.panelHealthErr
                            : r.counts.pending > 0
                              ? panelClasses.panelHealthWarn
                              : panelClasses.panelHealthOk
                        }
                      >
                        <Accordion.Control>
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
                                <Table.Tr style={{whiteSpace: "nowrap"}}>
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
            </Card>
          );
        })}
      </Stack>
    </Stack>
  );
}

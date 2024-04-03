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
  Input,
  Alert,
} from "@mantine/core";
import { useSuspenseAPIQuery } from "../api/api";
import { AlertingRulesMap } from "../api/responseTypes/rules";
import badgeClasses from "../Badge.module.css";
import RuleDefinition from "../RuleDefinition";
import { humanizeDurationRelative, now } from "../lib/formatTime";
import { Fragment } from "react";
import { StateMultiSelect } from "../StateMultiSelect";
import { useAppDispatch, useAppSelector } from "../state/hooks";
import { IconInfoCircle, IconSearch } from "@tabler/icons-react";
import { LabelBadges } from "../LabelBadges";
import { updateAlertFilters } from "../state/alertsPageSlice";

export default function AlertsPage() {
  const { data } = useSuspenseAPIQuery<AlertingRulesMap>({
    path: `/rules`,
    params: {
      type: "alert",
    },
  });

  const dispatch = useAppDispatch();
  const showAnnotations = useAppSelector(
    (state) => state.settings.showAnnotations
  );
  const filters = useAppSelector((state) => state.alertsPage.filters);

  const ruleStatsCount = {
    inactive: 0,
    pending: 0,
    firing: 0,
  };

  data.data.groups.forEach((el) =>
    el.rules.forEach((r) => ruleStatsCount[r.state]++)
  );

  return (
    <>
      <Group mb="md" mt="xs">
        <StateMultiSelect
          options={["inactive", "pending", "firing"]}
          optionClass={(o) =>
            o === "inactive"
              ? badgeClasses.healthOk
              : o === "pending"
              ? badgeClasses.healthWarn
              : badgeClasses.healthErr
          }
          placeholder="Filter by alert state"
          values={filters.state}
          onChange={(values) => dispatch(updateAlertFilters({ state: values }))}
        />
        <Input
          flex={1}
          leftSection={<IconSearch size={14} />}
          placeholder="Filter by alert name or labels"
        ></Input>
      </Group>
      <Stack>
        {data.data.groups.map((g, i) => {
          const filteredRules = g.rules.filter(
            (r) => filters.state.length === 0 || filters.state.includes(r.state)
          );
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
              </Group>
              {filteredRules.length === 0 && (
                <Alert
                  title="No matching rules"
                  icon={<IconInfoCircle size={14} />}
                >
                  No rules found that match your filter criteria.
                </Alert>
              )}
              <Accordion multiple variant="separated">
                {filteredRules.map((r, j) => {
                  const numFiring = r.alerts.filter(
                    (a) => a.state === "firing"
                  ).length;
                  const numPending = r.alerts.filter(
                    (a) => a.state === "pending"
                  ).length;

                  return (
                    <Accordion.Item
                      key={j}
                      value={j.toString()}
                      style={{
                        borderLeft:
                          numFiring > 0
                            ? "5px solid var(--mantine-color-red-4)"
                            : numPending > 0
                            ? "5px solid var(--mantine-color-orange-5)"
                            : "5px solid var(--mantine-color-green-4)",
                      }}
                    >
                      <Accordion.Control>
                        <Group wrap="nowrap" justify="space-between" mr="lg">
                          <Text>{r.name}</Text>
                          <Group gap="xs">
                            {numFiring > 0 && (
                              <Badge className={badgeClasses.healthErr}>
                                firing ({numFiring})
                              </Badge>
                            )}
                            {numPending > 0 && (
                              <Badge className={badgeClasses.healthWarn}>
                                pending ({numPending})
                              </Badge>
                            )}
                          </Group>
                        </Group>
                      </Accordion.Control>
                      <Accordion.Panel>
                        <RuleDefinition rule={r} />
                        {r.alerts.length > 0 && (
                          <Table mt="lg">
                            <Table.Thead>
                              <Table.Tr>
                                <Table.Th>Alert labels</Table.Th>
                                <Table.Th>State</Table.Th>
                                <Table.Th>Active Since</Table.Th>
                                <Table.Th>Value</Table.Th>
                              </Table.Tr>
                            </Table.Thead>
                            <Table.Tbody>
                              {r.type === "alerting" &&
                                r.alerts.map((a, k) => (
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
                                      <Table.Td>
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
                                      <Table.Td>{a.value}</Table.Td>
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
                                                  <Table.Th>{k}</Table.Th>
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
            </Card>
          );
        })}
      </Stack>
    </>
  );
}

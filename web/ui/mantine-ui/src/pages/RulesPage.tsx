import {
  Alert,
  Anchor,
  Badge,
  Card,
  Group,
  Pagination,
  rem,
  Stack,
  Text,
  TextInput,
  Tooltip,
} from "@mantine/core";
import {
  humanizeDurationRelative,
  humanizeDuration,
  now,
} from "../lib/formatTime";
import {
  IconAlertTriangle,
  IconBell,
  IconHourglass,
  IconInfoCircle,
  IconRefresh,
  IconRepeat,
  IconSearch,
  IconTimeline,
} from "@tabler/icons-react";
import { useSuspenseAPIQuery } from "../api/api";
import { Rule, RuleGroup, RulesResult } from "../api/responseTypes/rules";
import badgeClasses from "../Badge.module.css";
import RuleDefinition from "../components/RuleDefinition";
import { badgeIconStyle, inputIconStyle } from "../styles";
import {
  ArrayParam,
  NumberParam,
  StringParam,
  useQueryParam,
  withDefault,
} from "use-query-params";
import { useSettings } from "../state/settingsSlice";
import { useEffect, useMemo } from "react";
import CustomInfiniteScroll from "../components/CustomInfiniteScroll";
import classes from "./RulesPage.module.css";
import { useDebouncedValue, useLocalStorage } from "@mantine/hooks";
import { KVSearch } from "@nexucis/kvsearch";
import { StateMultiSelect } from "../components/StateMultiSelect";
import { Accordion } from "../components/Accordion";

const kvSearch = new KVSearch<Rule>({
  shouldSort: true,
  indexedKeys: ["name", "labels", ["labels", /.*/]],
});

type RulesPageData = {
  groups: (RuleGroup & { prefilterRulesCount: number })[];
};

const buildRulesPageData = (
  data: RulesResult,
  search: string,
  healthFilter: (string | null)[]
): RulesPageData => {
  const groups = data.groups.map((group) => ({
    ...group,
    prefilterRulesCount: group.rules.length,
    rules: (search === ""
      ? group.rules
      : kvSearch.filter(search, group.rules).map((value) => value.original)
    ).filter(
      (r) => healthFilter.length === 0 || healthFilter.includes(r.health)
    ),
  }));

  return { groups };
};

const healthBadgeClass = (state: string) => {
  switch (state) {
    case "ok":
      return badgeClasses.healthOk;
    case "err":
      return badgeClasses.healthErr;
    case "unknown":
      return badgeClasses.healthUnknown;
    default:
      throw new Error("Unknown rule health state");
  }
};

// Should be defined as a constant here instead of inline as a value
// to avoid unnecessary re-renders. Otherwise the empty array has
// a different reference on each render and causes subsequent memoized
// computations to re-run as long as no health filter is selected.
const emptyHealthFilter: string[] = [];

export default function RulesPage() {
  const { data } = useSuspenseAPIQuery<RulesResult>({ path: `/rules` });

  // Define URL query params.
  const [healthFilter, setHealthFilter] = useQueryParam(
    "health",
    withDefault(ArrayParam, emptyHealthFilter)
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

  const { ruleGroupsPerPage } = useSettings();
  const [activePage, setActivePage] = useQueryParam(
    "page",
    withDefault(NumberParam, 1)
  );

  // Update the page data whenever the fetched data or filters change.
  const rulesPageData = useMemo(
    () => buildRulesPageData(data.data, debouncedSearch, healthFilter),
    [data, healthFilter, debouncedSearch]
  );

  const shownGroups = useMemo(
    () =>
      showEmptyGroups
        ? rulesPageData.groups
        : rulesPageData.groups.filter((g) => g.rules.length > 0),
    [rulesPageData.groups, showEmptyGroups]
  );

  // If we were e.g. on page 10 and the number of total pages decreases to 5 (due to filtering
  // or changing the max number of items per page), go to the largest possible page.
  const totalPageCount = Math.ceil(shownGroups.length / ruleGroupsPerPage);
  const effectiveActivePage = Math.max(1, Math.min(activePage, totalPageCount));

  useEffect(() => {
    if (effectiveActivePage !== activePage) {
      setActivePage(effectiveActivePage);
    }
  }, [effectiveActivePage, activePage, setActivePage]);

  const currentPageGroups = useMemo(
    () =>
      shownGroups.slice(
        (effectiveActivePage - 1) * ruleGroupsPerPage,
        effectiveActivePage * ruleGroupsPerPage
      ),
    [shownGroups, effectiveActivePage, ruleGroupsPerPage]
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
              <Tooltip label="Last group evaluation" withArrow>
                <Badge
                  variant="light"
                  className={badgeClasses.statsBadge}
                  styles={{ label: { textTransform: "none" } }}
                  leftSection={<IconRefresh style={badgeIconStyle} />}
                >
                  last run {humanizeDurationRelative(g.lastEvaluation, now())}
                </Badge>
              </Tooltip>
              <Tooltip label="Duration of last group evaluation" withArrow>
                <Badge
                  variant="light"
                  className={badgeClasses.statsBadge}
                  styles={{ label: { textTransform: "none" } }}
                  leftSection={<IconHourglass style={badgeIconStyle} />}
                >
                  took {humanizeDuration(parseFloat(g.evaluationTime) * 1000)}
                </Badge>
              </Tooltip>
              <Tooltip label="Group evaluation interval" withArrow>
                <Badge
                  variant="transparent"
                  className={badgeClasses.statsBadge}
                  styles={{ label: { textTransform: "none" } }}
                  leftSection={<IconRepeat style={badgeIconStyle} />}
                >
                  every {humanizeDuration(parseFloat(g.interval) * 1000)}{" "}
                </Badge>
              </Tooltip>
            </Group>
          </Group>
          {g.prefilterRulesCount === 0 ? (
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
              {g.prefilterRulesCount} filtered rules).
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
                  {items.map((r, j) => (
                    <Accordion.Item
                      mt={rem(5)}
                      key={j}
                      value={j.toString()}
                      style={{
                        borderLeft:
                          r.health === "err"
                            ? "5px solid var(--mantine-color-red-4)"
                            : r.health === "unknown"
                              ? "5px solid var(--mantine-color-gray-5)"
                              : "5px solid var(--mantine-color-green-4)",
                      }}
                    >
                      <Accordion.Control
                        styles={{ label: { paddingBlock: rem(10) } }}
                      >
                        <Group justify="space-between" mr="lg">
                          <Group gap="xs" wrap="nowrap">
                            {r.type === "alerting" ? (
                              <Tooltip label="Alerting rule" withArrow>
                                <IconBell
                                  style={{ width: rem(15), height: rem(15) }}
                                />
                              </Tooltip>
                            ) : (
                              <Tooltip label="Recording rule" withArrow>
                                <IconTimeline
                                  style={{ width: rem(15), height: rem(15) }}
                                />
                              </Tooltip>
                            )}
                            <Text>{r.name}</Text>
                          </Group>
                          <Group gap="xs">
                            <Group gap="xs" wrap="wrap">
                              <Tooltip label="Last rule evaluation" withArrow>
                                <Badge
                                  variant="light"
                                  className={badgeClasses.statsBadge}
                                  styles={{ label: { textTransform: "none" } }}
                                  leftSection={
                                    <IconRefresh style={badgeIconStyle} />
                                  }
                                >
                                  {humanizeDurationRelative(
                                    r.lastEvaluation,
                                    now()
                                  )}
                                </Badge>
                              </Tooltip>

                              <Tooltip
                                label="Duration of last rule evaluation"
                                withArrow
                              >
                                <Badge
                                  variant="light"
                                  className={badgeClasses.statsBadge}
                                  styles={{ label: { textTransform: "none" } }}
                                  leftSection={
                                    <IconHourglass style={badgeIconStyle} />
                                  }
                                >
                                  {humanizeDuration(
                                    parseFloat(r.evaluationTime) * 1000
                                  )}
                                </Badge>
                              </Tooltip>
                            </Group>
                            <Badge className={healthBadgeClass(r.health)}>
                              {r.health}
                            </Badge>
                          </Group>
                        </Group>
                      </Accordion.Control>
                      <Accordion.Panel>
                        <RuleDefinition rule={r} />
                        {r.lastError && (
                          <Alert
                            color="red"
                            mt="sm"
                            title="Rule failed to evaluate"
                            icon={<IconAlertTriangle />}
                          >
                            <strong>Error:</strong> {r.lastError}
                          </Alert>
                        )}
                      </Accordion.Panel>
                    </Accordion.Item>
                  ))}
                </Accordion>
              )}
            />
          )}
        </Card>
      )),
    [currentPageGroups, setShowEmptyGroups]
  );

  return (
    <Stack mt="xs">
      <Group>
        <StateMultiSelect
          options={["ok", "unknown", "err"]}
          optionClass={(o) =>
            o === "ok"
              ? badgeClasses.healthOk
              : o === "unknown"
                ? badgeClasses.healthWarn
                : badgeClasses.healthErr
          }
          placeholder="Filter by rule health"
          values={(healthFilter?.filter((v) => v !== null) as string[]) || []}
          onChange={(values) => setHealthFilter(values)}
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
      {rulesPageData.groups.length === 0 ? (
        <Alert title="No rules found" icon={<IconInfoCircle />}>
          No rules found.
        </Alert>
      ) : (
        !showEmptyGroups &&
        rulesPageData.groups.length !== shownGroups.length && (
          <Alert
            title="Hiding groups with no matching rules"
            icon={<IconInfoCircle />}
          >
            Hiding {rulesPageData.groups.length - shownGroups.length} empty
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

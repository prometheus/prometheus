import {
  Accordion,
  ActionIcon,
  Alert,
  Anchor,
  Badge,
  Card,
  CopyButton,
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
  IconCheck,
  IconCopy,
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
import { useI18n } from "../i18nContext";

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
  const { t } = useI18n();
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
              <Text fz="xl" fw={600}>
                {g.name}
              </Text>
              <Text fz="sm" c="gray.6">
                {g.file}
              </Text>
            </Group>
            <Group>
              <Tooltip label={t("rules.lastGroupEvaluation")} withArrow>
                <Badge
                  variant="light"
                  className={badgeClasses.statsBadge}
                  styles={{ label: { textTransform: "none" } }}
                  leftSection={<IconRefresh style={badgeIconStyle} />}
                >
                  {t("rules.lastRun", { duration: humanizeDurationRelative(g.lastEvaluation, now()) })}
                </Badge>
              </Tooltip>
              <Tooltip label={t("rules.durationLastGroupEvaluation")} withArrow>
                <Badge
                  variant="light"
                  className={badgeClasses.statsBadge}
                  styles={{ label: { textTransform: "none" } }}
                  leftSection={<IconHourglass style={badgeIconStyle} />}
                >
                  {t("rules.took", { duration: humanizeDuration(parseFloat(g.evaluationTime) * 1000) })}
                </Badge>
              </Tooltip>
              <Tooltip label={t("rules.groupInterval")} withArrow>
                <Badge
                  variant="transparent"
                  className={badgeClasses.statsBadge}
                  styles={{ label: { textTransform: "none" } }}
                  leftSection={<IconRepeat style={badgeIconStyle} />}
                >
                  {t("rules.every", { duration: humanizeDuration(parseFloat(g.interval) * 1000) })}
                </Badge>
              </Tooltip>
            </Group>
          </Group>
          {g.prefilterRulesCount === 0 ? (
            <Alert title={t("alerts.noRules")} icon={<IconInfoCircle />}>
              {t("alerts.noRulesInGroup")}
              <Anchor
                ml="md"
                fz="1em"
                onClick={() => setShowEmptyGroups(false)}
              >
                {t("alerts.hideEmptyGroups")}
              </Anchor>
            </Alert>
          ) : g.rules.length === 0 ? (
            <Alert title={t("alerts.noMatchingRules")} icon={<IconInfoCircle />}>
              {t("alerts.omittedRules", { count: g.prefilterRulesCount })}
              <Anchor
                ml="md"
                fz="1em"
                onClick={() => setShowEmptyGroups(false)}
              >
                {t("alerts.hideEmptyGroups")}
              </Anchor>
            </Alert>
          ) : (
            <CustomInfiniteScroll
              allItems={g.rules}
              child={({ items }) => (
                <Accordion multiple variant="separated" keepMounted={false} classNames={classes}>
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
                        className={classes.ruleControl}
                        styles={{ label: { paddingBlock: rem(10) } }}
                      >
                        <Group justify="space-between" mr="lg">
                          <Group gap="xs" wrap="nowrap">
                            {r.type === "alerting" ? (
                              <Tooltip label={t("rules.alertingRule")} withArrow>
                                <IconBell
                                  style={{ width: rem(15), height: rem(15) }}
                                />
                              </Tooltip>
                            ) : (
                              <Tooltip label={t("rules.recordingRule")} withArrow>
                                <IconTimeline
                                  style={{ width: rem(15), height: rem(15) }}
                                />
                              </Tooltip>
                            )}
                            <Text>{r.name}</Text>
                            <CopyButton value={r.name}>
                              {({ copied, copy }) => (
                                <ActionIcon
                                  className={classes.copyRuleNameButton}
                                  variant="subtle"
                                  color="gray"
                                  onClick={(e) => {
                                    e.stopPropagation(); copy();
                                  }}
                                  size="xs"
                                >
                                  {copied ? <IconCheck size={12} /> : <IconCopy size={12} />}
                                </ActionIcon>
                              )}
                            </CopyButton>
                          </Group>
                          <Group gap="xs">
                            <Group gap="xs" wrap="wrap">
                              <Tooltip label={t("rules.lastRuleEvaluation")} withArrow>
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
                                label={t("rules.durationLastRuleEvaluation")}
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
                            title={t("rules.ruleFailed")}
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
    [currentPageGroups, setShowEmptyGroups, t]
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
          placeholder={t("rules.filterByHealth")}
          values={(healthFilter?.filter((v) => v !== null) as string[]) || []}
          onChange={(values) => setHealthFilter(values)}
        />
        <TextInput
          flex={1}
          leftSection={<IconSearch style={inputIconStyle} />}
          placeholder={t("alerts.filterByRuleName")}
          value={searchFilter || ""}
          onChange={(event) =>
            setSearchFilter(event.currentTarget.value || null)
          }
        ></TextInput>
      </Group>
      {rulesPageData.groups.length === 0 ? (
        <Alert title={t("alerts.noRulesFound")} icon={<IconInfoCircle />}>
          {t("alerts.noRulesFound")}.
        </Alert>
      ) : (
        !showEmptyGroups &&
        rulesPageData.groups.length !== shownGroups.length && (
          <Alert
            title={t("alerts.hidingEmptyGroups")}
            icon={<IconInfoCircle />}
          >
            {t("alerts.emptyGroupsSummary", {
              count: rulesPageData.groups.length - shownGroups.length,
            })}
            <Anchor ml="md" fz="1em" onClick={() => setShowEmptyGroups(true)}>
              {t("alerts.showEmptyGroups")}
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

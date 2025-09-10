import {
  ActionIcon,
  Box,
  Group,
  Select,
  Skeleton,
  TextInput,
} from "@mantine/core";
import {
  IconLayoutNavbarCollapse,
  IconLayoutNavbarExpand,
  IconSearch,
} from "@tabler/icons-react";
import { StateMultiSelect } from "../../components/StateMultiSelect";
import { Suspense, useState } from "react";
import badgeClasses from "../../Badge.module.css";
import { useAppDispatch, useAppSelector } from "../../state/hooks";
import {
  setCollapsedPools,
  setShowLimitAlert,
} from "../../state/targetsPageSlice";
import {
  ArrayParam,
  StringParam,
  useQueryParam,
  withDefault,
} from "use-query-params";
import ErrorBoundary from "../../components/ErrorBoundary";
import ScrapePoolList from "./ScrapePoolsList";
import { useSuspenseAPIQuery } from "../../api/api";
import { ScrapePoolsResult } from "../../api/responseTypes/scrapePools";
import { expandIconStyle, inputIconStyle } from "../../styles";
import { useDebouncedValue } from "@mantine/hooks";

export const targetPoolDisplayLimit = 20;

// Should be defined as a constant here instead of inline as a value
// to avoid unnecessary re-renders. Otherwise the empty array has
// a different reference on each render and causes subsequent memoized
// computations to re-run as long as no state filter is selected.
const emptyHealthFilter: string[] = [];

export default function TargetsPage() {
  // Load the list of all available scrape pools.
  const {
    data: {
      data: { scrapePools },
    },
  } = useSuspenseAPIQuery<ScrapePoolsResult>({
    path: `/scrape_pools`,
  });

  const dispatch = useAppDispatch();

  const poolDefaultPlaceholder = "Select scrape pool";

  const [scrapePool, setScrapePool] = useQueryParam("pool", StringParam);
  const [poolPlaceholder, setPoolPlaceholder] = useState<string>(poolDefaultPlaceholder);
  const [poolSelecting, setPoolSelecting] = useState<boolean>(false);
  const [healthFilter, setHealthFilter] = useQueryParam(
    "health",
    withDefault(ArrayParam, emptyHealthFilter)
  );
  const [searchFilter, setSearchFilter] = useQueryParam(
    "search",
    withDefault(StringParam, "")
  );
  const [debouncedSearch] = useDebouncedValue<string>(searchFilter.trim(), 250);

  const { collapsedPools, showLimitAlert } = useAppSelector(
    (state) => state.targetsPage
  );

  // When we have more than X targets, we want to limit the display by selecting the first
  // scrape pool and reflecting that in the URL as well. We also want to show an alert
  // about the fact that we are limiting the display, but the tricky bit is that this
  // alert should only be shown once, upon the first "redirect" that causes the limiting,
  // not again when the page is reloaded with the same URL parameters. That's why we remember
  // `showLimitAlert` in Redux (just useState() doesn't work properly, because the component
  // for some Suspense-related reasons seems to be mounted/unmounted multiple times, so the
  // state cell would get initialized multiple times as well).
  const limited =
    scrapePools.length > targetPoolDisplayLimit && scrapePool === undefined;
  if (limited) {
    setScrapePool(scrapePools[0]);
    dispatch(setShowLimitAlert(true));
  }

  return (
    <>
      <Group mb="md" mt="xs">
        <Select
          placeholder={poolPlaceholder}
          data={[{ label: "All pools", value: "" }, ...scrapePools]}
          value={poolSelecting ? null : (limited && scrapePools[0]) || scrapePool || null}
          onChange={(value) => {
            setScrapePool(value);
            if (showLimitAlert) {
              dispatch(setShowLimitAlert(false));
            }
          }}
          onDropdownOpen={() => {
            setPoolPlaceholder(scrapePool || poolDefaultPlaceholder)
            setPoolSelecting(true)
          }}
          onDropdownClose={() => {
            setPoolPlaceholder(poolDefaultPlaceholder)
            setPoolSelecting(false)
          }}
          searchable
        />
        <StateMultiSelect
          options={["unknown", "up", "down"]}
          optionClass={(o) =>
            o === "unknown"
              ? badgeClasses.healthUnknown
              : o === "up"
                ? badgeClasses.healthOk
                : badgeClasses.healthErr
          }
          placeholder="Filter by target health"
          values={(healthFilter?.filter((v) => v !== null) as string[]) || []}
          onChange={(values) => setHealthFilter(values)}
        />
        <TextInput
          flex={1}
          leftSection={<IconSearch style={inputIconStyle} />}
          placeholder="Filter by endpoint or labels"
          value={searchFilter || ""}
          onChange={(event) =>
            setSearchFilter(event.currentTarget.value || null)
          }
        ></TextInput>
        <ActionIcon
          size="input-sm"
          title={
            collapsedPools.length > 0
              ? "Expand all pools"
              : "Collapse all pools"
          }
          variant="light"
          onClick={() =>
            dispatch(
              setCollapsedPools(collapsedPools.length > 0 ? [] : scrapePools)
            )
          }
        >
          {collapsedPools.length > 0 ? (
            <IconLayoutNavbarExpand style={expandIconStyle} />
          ) : (
            <IconLayoutNavbarCollapse style={expandIconStyle} />
          )}
        </ActionIcon>
      </Group>

      <ErrorBoundary key={location.pathname} title="Error showing target pools">
        <Suspense
          fallback={
            <Box mt="lg">
              {Array.from(Array(10), (_, i) => (
                <Skeleton key={i} height={40} mb={15} width={1000} mx="auto" />
              ))}
            </Box>
          }
        >
          <ScrapePoolList
            poolNames={scrapePools}
            selectedPool={(limited && scrapePools[0]) || scrapePool || null}
            healthFilter={healthFilter as string[]}
            searchFilter={debouncedSearch}
          />
        </Suspense>
      </ErrorBoundary>
    </>
  );
}

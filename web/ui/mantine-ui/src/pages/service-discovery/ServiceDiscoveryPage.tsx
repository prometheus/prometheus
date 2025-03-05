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
import { Suspense } from "react";
import { useAppDispatch, useAppSelector } from "../../state/hooks";

import {
  ArrayParam,
  StringParam,
  useQueryParam,
  withDefault,
} from "use-query-params";
import ErrorBoundary from "../../components/ErrorBoundary";
import ScrapePoolList from "./ServiceDiscoveryPoolsList";
import { useSuspenseAPIQuery } from "../../api/api";
import { ScrapePoolsResult } from "../../api/responseTypes/scrapePools";
import {
  setCollapsedPools,
  setShowLimitAlert,
} from "../../state/serviceDiscoveryPageSlice";
import { StateMultiSelect } from "../../components/StateMultiSelect";
import badgeClasses from "../../Badge.module.css";
import { expandIconStyle, inputIconStyle } from "../../styles";

export const targetPoolDisplayLimit = 20;

export default function ServiceDiscoveryPage() {
  // Load the list of all available scrape pools.
  const {
    data: {
      data: { scrapePools },
    },
  } = useSuspenseAPIQuery<ScrapePoolsResult>({
    path: `/scrape_pools`,
  });

  const dispatch = useAppDispatch();

  const [scrapePool, setScrapePool] = useQueryParam("pool", StringParam);
  const [stateFilter, setStateFilter] = useQueryParam(
    "state",
    withDefault(ArrayParam, [])
  );
  const [searchFilter, setSearchFilter] = useQueryParam(
    "search",
    withDefault(StringParam, "")
  );

  const { collapsedPools, showLimitAlert } = useAppSelector(
    (state) => state.serviceDiscoveryPage
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
          placeholder="Select scrape pool"
          data={[{ label: "All pools", value: "" }, ...scrapePools]}
          value={(limited && scrapePools[0]) || scrapePool || null}
          onChange={(value) => {
            setScrapePool(value);
            if (showLimitAlert) {
              dispatch(setShowLimitAlert(false));
            }
          }}
          searchable
        />
        <StateMultiSelect
          options={["active", "dropped"]}
          optionClass={(o) =>
            o === "active" ? badgeClasses.healthOk : badgeClasses.healthInfo
          }
          placeholder="Filter by state"
          values={(stateFilter?.filter((v) => v !== null) as string[]) || []}
          onChange={(values) => setStateFilter(values)}
        />
        <TextInput
          flex={1}
          leftSection={<IconSearch style={inputIconStyle} />}
          placeholder="Filter by labels"
          value={searchFilter || ""}
          onChange={(event) => setSearchFilter(event.currentTarget.value)}
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
            stateFilter={stateFilter as string[]}
            searchFilter={searchFilter}
          />
        </Suspense>
      </ErrorBoundary>
    </>
  );
}

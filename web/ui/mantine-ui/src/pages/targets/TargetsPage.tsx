import { ActionIcon, Box, Group, Input, Select, Skeleton } from "@mantine/core";
import {
  IconLayoutNavbarCollapse,
  IconLayoutNavbarExpand,
  IconSearch,
} from "@tabler/icons-react";
import { StateMultiSelect } from "../../components/StateMultiSelect";
import { useSuspenseAPIQuery } from "../../api/api";
import { ScrapePoolsResult } from "../../api/responseTypes/scrapePools";
import { Suspense, useEffect } from "react";
import badgeClasses from "../../Badge.module.css";
import { useAppDispatch, useAppSelector } from "../../state/hooks";
import {
  setCollapsedPools,
  setHealthFilter,
  setSelectedPool,
} from "../../state/targetsPageSlice";
import ScrapePoolList from "./ScrapePoolsList";
import ErrorBoundary from "../../components/ErrorBoundary";

const scrapePoolQueryParam = "scrapePool";

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

  // If there is a selected pool in the URL, extract it on initial load.
  useEffect(() => {
    const uriPool = new URLSearchParams(window.location.search).get(
      scrapePoolQueryParam
    );
    if (uriPool !== null) {
      dispatch(setSelectedPool(uriPool));
    }
  }, [dispatch]);

  const { selectedPool, healthFilter, collapsedPools } = useAppSelector(
    (state) => state.targetsPage
  );

  let poolToShow = selectedPool;
  let limitedDueToManyPools = false;

  if (poolToShow === null && scrapePools.length > 20) {
    poolToShow = scrapePools[0];
    limitedDueToManyPools = true;
  }

  return (
    <>
      <Group mb="md" mt="xs">
        <Select
          placeholder="Select scrape pool"
          data={[{ label: "All pools", value: "" }, ...scrapePools]}
          value={selectedPool}
          onChange={(value) => dispatch(setSelectedPool(value || null))}
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
          placeholder="Filter by target state"
          values={healthFilter}
          onChange={(values) => dispatch(setHealthFilter(values))}
        />
        <Input
          flex={1}
          leftSection={<IconSearch size={14} />}
          placeholder="Filter by endpoint or labels"
        ></Input>
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
            <IconLayoutNavbarExpand size={16} />
          ) : (
            <IconLayoutNavbarCollapse size={16} />
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
            selectedPool={poolToShow}
            limited={limitedDueToManyPools}
          />
        </Suspense>
      </ErrorBoundary>
    </>
  );
}

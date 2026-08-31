import { Box, Collapse, Skeleton } from "@mantine/core";
import { CodeHighlight } from "@mantine/code-highlight";
import { Suspense } from "react";
import { useSuspenseAPIQuery } from "../api/api";
import ConfigResult from "../api/responseTypes/config";
import ErrorBoundary from "./ErrorBoundary";

const ScrapePoolConfigContent = ({ pool }: { pool: string }) => {
  const {
    data: {
      data: { yaml },
    },
  } = useSuspenseAPIQuery<ConfigResult>({
    path: "/scrape_pools/config",
    params: { scrapePool: pool },
  });

  return <CodeHighlight code={yaml} language="yaml" maw="100%" />;
};

type ScrapePoolConfigProps = {
  pool: string;
  expanded: boolean;
  id?: string;
};

const ScrapePoolConfig = ({ pool, expanded, id }: ScrapePoolConfigProps) => {
  const collapseId = id ?? `scrape-config-${pool}`;

  return (
    <Collapse expanded={expanded} id={collapseId}>
      {expanded && (
        <Box mb="md">
          <ErrorBoundary
            key={pool}
            title="Error loading scrape configuration"
          >
            <Suspense
              fallback={
                <Box py="xs">
                  <Skeleton height={24} mb="xs" />
                  <Skeleton height={120} />
                </Box>
              }
            >
              <ScrapePoolConfigContent pool={pool} />
            </Suspense>
          </ErrorBoundary>
        </Box>
      )}
    </Collapse>
  );
};

export default ScrapePoolConfig;

import { Accordion, Box, Skeleton } from "@mantine/core";
import { CodeHighlight } from "@mantine/code-highlight";
import { Suspense, useState } from "react";
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

const ScrapePoolConfig = ({ pool }: { pool: string }) => {
  const [opened, setOpened] = useState(false);

  return (
    <Accordion
      value={opened ? "config" : null}
      onChange={(value) => setOpened(value === "config")}
      variant="contained"
      mb="md"
    >
      <Accordion.Item value="config">
        <Accordion.Control>Scrape configuration</Accordion.Control>
        <Accordion.Panel>
          {opened && (
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
          )}
        </Accordion.Panel>
      </Accordion.Item>
    </Accordion>
  );
};

export default ScrapePoolConfig;

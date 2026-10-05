import { FC, useId, useState } from "react";
import { Alert, Skeleton, Box, Group, Stack } from "@mantine/core";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import { InstantQueryResult } from "../../api/responseTypes/query";
import {
  APIQueryMetadata,
  SuccessAPIResponse,
  useAPIQuery,
} from "../../api/api";
import dayjs from "dayjs";
import timezone from "dayjs/plugin/timezone";
import { useAppDispatch, useAppSelector } from "../../state/hooks";
import { setVisualizer } from "../../state/queryPageSlice";
import TimeInput from "./TimeInput";
import DataTable from "./DataTable";
import QueryStatsDisplay from "./QueryStatsDisplay";
import { useSettings } from "../../state/settingsSlice";
dayjs.extend(timezone);

export interface TableTabProps {
  panelIdx: number;
  retriggerIdx: number;
  expr: string;
}

const TableTab: FC<TableTabProps> = ({ panelIdx, retriggerIdx, expr }) => {
  const { visualizer } = useAppSelector(
    (state) => state.queryPage.panels[panelIdx],
  );
  const dispatch = useAppDispatch();
  const { showQueryWarnings, showQueryInfoNotices } = useSettings();

  const { endTime, range } = visualizer;

  const queryKey = [useId(), "/query", expr, endTime, retriggerIdx];
  const {
    data: result,
    error,
    isFetching,
  } = useAPIQuery<
    InstantQueryResult,
    {
      response: SuccessAPIResponse<InstantQueryResult>;
      metadata: APIQueryMetadata;
    }
  >({
    key: queryKey,
    path: "/query",
    params: (requestTimeMs) => {
      const time = (endTime ?? requestTimeMs) / 1000;
      return { query: expr, time: `${time}`, stats: "true" };
    },
    enabled: expr !== "",
    select: (response, metadata) => ({ response, metadata }),
  });
  const data = result?.response;

  return (
    <Stack gap="lg" mt="sm">
      <Group justify="space-between">
        <TimeInput
          time={endTime}
          range={range}
          description="Evaluation time"
          onChangeTime={(time) =>
            dispatch(
              setVisualizer({
                idx: panelIdx,
                visualizer: { ...visualizer, endTime: time },
              }),
            )
          }
        />
        {!isFetching && data !== undefined && (
          <QueryStatsDisplay
            numResults={data.data.result.length}
            responseTime={result!.metadata.responseTimeMs}
            stats={data.data.stats!}
          />
        )}
      </Group>
      {isFetching ? (
        <Box>
          {Array.from(Array(5), (_, i) => (
            <Skeleton key={i} height={30} mb={15} />
          ))}
        </Box>
      ) : error !== null ? (
        <Alert
          color="red"
          title="Error executing query"
          icon={<IconAlertTriangle />}
        >
          {error.message}
        </Alert>
      ) : data === undefined ? (
        <Alert variant="transparent">No data queried yet</Alert>
      ) : (
        <>
          {data.data.result.length === 0 && (
            <Alert title="Empty query result" icon={<IconInfoCircle />}>
              This query returned no data.
            </Alert>
          )}

          {showQueryWarnings &&
            data.warnings?.map((w, idx) => (
              <Alert
                key={idx}
                color="red"
                title="Query warning"
                icon={<IconAlertTriangle />}
              >
                {w}
              </Alert>
            ))}

          {showQueryInfoNotices &&
            data.infos?.map((w, idx) => (
              <Alert
                key={idx}
                color="yellow"
                title="Query notice"
                icon={<IconInfoCircle />}
              >
                {w}
              </Alert>
            ))}
          <LimitedDataTable
            key={JSON.stringify([queryKey, result!.metadata.receivedAtMs])}
            data={data.data}
          />
        </>
      )}
    </Stack>
  );
};

const LimitedDataTable: FC<{ data: InstantQueryResult }> = ({ data }) => {
  const [limitResults, setLimitResults] = useState(true);
  return (
    <DataTable
      data={data}
      limitResults={limitResults}
      setLimitResults={setLimitResults}
    />
  );
};

export default TableTab;

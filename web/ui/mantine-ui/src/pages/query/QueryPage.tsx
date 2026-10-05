import { Alert, Box, Button, Stack } from "@mantine/core";
import {
  IconAlertCircle,
  IconAlertTriangle,
  IconPlus,
} from "@tabler/icons-react";
import { useAppDispatch, useAppSelector } from "../../state/hooks";
import {
  addPanel,
  newDefaultPanel,
  setPanels,
} from "../../state/queryPageSlice";
import Panel from "./QueryPanel";
import { LabelValuesResult } from "../../api/responseTypes/labelValues";
import { useAPIQuery } from "../../api/api";
import { useEffect, useState } from "react";
import { InstantQueryResult } from "../../api/responseTypes/query";
import { humanizeDuration } from "../../lib/formatTime";
import { decodePanelOptionsFromURLParams } from "./urlStateEncoding";
import { buttonIconStyle } from "../../styles";

export default function QueryPage() {
  const panels = useAppSelector((state) => state.queryPage.panels);
  const dispatch = useAppDispatch();
  const [dismissedSample, setDismissedSample] = useState<number | null>(null);

  // Update the panels whenever the URL params change.
  useEffect(() => {
    const handleURLChange = () => {
      const panels = decodePanelOptionsFromURLParams(window.location.search);
      if (panels.length > 0) {
        dispatch(setPanels(panels));
      }
    };

    handleURLChange();

    window.addEventListener("popstate", handleURLChange);

    return () => {
      window.removeEventListener("popstate", handleURLChange);
    };
  }, [dispatch]);

  // Clear the query page when navigating away from it.
  useEffect(() => {
    return () => {
      dispatch(setPanels([newDefaultPanel()]));
    };
  }, [dispatch]);

  const { data: metricNamesResult, error: metricNamesError } =
    useAPIQuery<LabelValuesResult>({
      path: "/label/__name__/values",
    });

  const { data: timeResult, error: timeError } = useAPIQuery<
    InstantQueryResult,
    { delta: number; receivedAtMs: number }
  >({
    path: "/query",
    params: { query: "time()" },
    select: (response, { receivedAtMs }) => {
      if (response.data.resultType !== "scalar") {
        throw new Error("Unexpected result type from time query");
      }
      return {
        delta: Math.abs(receivedAtMs / 1000 - response.data.result[0]),
        receivedAtMs,
      };
    },
  });
  const timeDelta =
    timeResult && timeResult.receivedAtMs !== dismissedSample
      ? timeResult.delta
      : 0;

  return (
    <Box mt="xs">
      {metricNamesError && (
        <Alert
          mb="sm"
          icon={<IconAlertTriangle />}
          color="red"
          title="Error fetching metrics list"
        >
          Unable to fetch list of metric names: {metricNamesError.message}
        </Alert>
      )}
      {timeError && (
        <Alert
          mb="sm"
          icon={<IconAlertTriangle />}
          color="red"
          title="Error fetching server time"
        >
          {timeError.message}
        </Alert>
      )}
      {timeDelta > 30 && (
        <Alert
          mb="sm"
          title="Server time is out of sync"
          color="red"
          icon={<IconAlertCircle />}
          withCloseButton
          closeButtonLabel="Close"
          onClose={() => setDismissedSample(timeResult!.receivedAtMs)}
        >
          Detected a time difference of{" "}
          <strong>{humanizeDuration(timeDelta * 1000)}</strong> between your
          browser and the server. You may see unexpected time-shifted query
          results due to the time drift.
        </Alert>
      )}

      <Stack gap="xl">
        {panels.map((p, idx) => (
          <Panel
            key={p.id}
            idx={idx}
            metricNames={metricNamesResult?.data || []}
          />
        ))}
      </Stack>

      <Button
        variant="light"
        mt="xl"
        leftSection={<IconPlus style={buttonIconStyle} />}
        onClick={() => dispatch(addPanel())}
      >
        Add query
      </Button>
    </Box>
  );
}

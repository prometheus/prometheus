import { FC, PropsWithChildren, useEffect, useState } from "react";
import { IconAlertTriangle } from "@tabler/icons-react";
import { useAppDispatch } from "../state/hooks";
import { updateSettings, useSettings } from "../state/settingsSlice";
import { useSuspenseAPIQuery } from "../api/api";
import { WALReplayStatus } from "../api/responseTypes/walreplay";
import { Progress, Alert, Stack } from "@mantine/core";
import { useSuspenseQuery } from "@tanstack/react-query";

const STATUS_STARTING = "is starting up...";
const STATUS_STOPPING = "is shutting down...";
const STATUS_LOADING = "is not ready...";

const ReadinessLoader: FC = () => {
  const { pathPrefix, agentMode } = useSettings();
  const dispatch = useAppDispatch();

  // Query key is incremented every second to retrigger the status fetching.
  const [queryKey, setQueryKey] = useState(0);
  const [statusMessage, setStatusMessage] = useState("");

  // Query readiness status.
  const { data: ready } = useSuspenseQuery<boolean>({
    queryKey: ["ready", queryKey],
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    queryFn: async ({ signal }: { signal: AbortSignal }) => {
      try {
        const res = await fetch(`${pathPrefix}/-/ready`, {
          cache: "no-store",
          credentials: "same-origin",
          signal,
        });
        switch (res.status) {
          case 200:
            setStatusMessage(""); // Clear any status message when ready.
            return true;
          case 503:
            // Check the custom header `X-Prometheus-Stopping` for stopping information.
            if (res.headers.get("X-Prometheus-Stopping") === "true") {
              setStatusMessage(STATUS_STOPPING);
            } else {
              setStatusMessage(STATUS_STARTING);
            }

            return false;
          default:
            throw new Error(res.statusText);
        }
      } catch (_) {
        throw new Error("Unexpected error while fetching ready status");
      }
    },
  });

  // Only call WAL replay status API if the service is starting up.
  const shouldQueryWALReplay = statusMessage === STATUS_STARTING;

  const { data: walData, isSuccess: walSuccess } =
    useSuspenseAPIQuery<WALReplayStatus>({
      path: "/status/walreplay",
      key: ["walreplay", queryKey],
      enabled: shouldQueryWALReplay, // Only enabled when service is starting up.
    });

  useEffect(() => {
    if (ready) {
      dispatch(updateSettings({ ready: ready }));
    }
  }, [ready, dispatch]);

  useEffect(() => {
    const interval = setInterval(() => setQueryKey((v) => v + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  return (
    <Alert
      color="yellow"
      title={
        "Prometheus " +
        ((agentMode && "Agent ") || "") +
        (statusMessage || STATUS_LOADING)
      }
      icon={<IconAlertTriangle />}
      maw={500}
      mx="auto"
      mt="lg"
    >
      {shouldQueryWALReplay && walSuccess && walData && (
        <Stack>
          <strong>
            Replaying WAL ({walData.data.current}/{walData.data.max})
          </strong>
          <Progress
            size="xl"
            animated
            color="yellow"
            value={
              ((walData.data.current - walData.data.min + 1) /
                (walData.data.max - walData.data.min + 1)) *
              100
            }
          />
        </Stack>
      )}
    </Alert>
  );
};

export const ReadinessWrapper: FC<PropsWithChildren> = ({ children }) => {
  const { ready } = useSettings();

  if (ready) {
    return <>{children}</>;
  }

  return <ReadinessLoader />;
};

export default ReadinessWrapper;

import { useState } from "react";
import {
  TextInput,
  Button,
  Group,
  Alert,
  Text,
  Stack,
} from "@mantine/core";
import { IconAlertTriangle, IconCheck, IconTrash } from "@tabler/icons-react";
import { useSettings } from "../state/settingsSlice";
import { API_PATH } from "../api/api";
import InfoPageStack from "../components/InfoPageStack";
import InfoPageCard from "../components/InfoPageCard";

export default function DeleteSeriesPage() {
  const { pathPrefix } = useSettings();

  const [matchers, setMatchers] = useState("");
  const [startTime, setStartTime] = useState("");
  const [endTime, setEndTime] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [cleaning, setCleaning] = useState(false);

  const handleDelete = async () => {
    setError(null);
    setSuccess(null);

    const matchList = matchers
      .split("\n")
      .map((m) => m.trim())
      .filter((m) => m !== "");

    if (matchList.length === 0) {
      setError("Provide at least one match[] selector.");
      return;
    }

    const params = new URLSearchParams();
    for (const m of matchList) {
      params.append("match[]", m);
    }
    if (startTime) {
      params.append("start", startTime);
    }
    if (endTime) {
      params.append("end", endTime);
    }

    setDeleting(true);
    try {
      const res = await fetch(
        `${pathPrefix}/${API_PATH}/admin/tsdb/delete_series?${params.toString()}`,
        {
          method: "POST",
          credentials: "same-origin",
        }
      );

      if (!res.ok) {
        if (res.headers.get("content-type")?.startsWith("application/json")) {
          const body = await res.json();
          throw new Error(body.error || res.statusText);
        }
        throw new Error(res.statusText);
      }

      setSuccess(
        `Successfully deleted series matching: ${matchList.join(", ")}`
      );
      setMatchers("");
      setStartTime("");
      setEndTime("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
    } finally {
      setDeleting(false);
    }
  };

  const handleCleanTombstones = async () => {
    setError(null);
    setSuccess(null);
    setCleaning(true);

    try {
      const res = await fetch(
        `${pathPrefix}/${API_PATH}/admin/tsdb/clean_tombstones`,
        {
          method: "POST",
          credentials: "same-origin",
        }
      );

      if (!res.ok) {
        if (res.headers.get("content-type")?.startsWith("application/json")) {
          const body = await res.json();
          throw new Error(body.error || res.statusText);
        }
        throw new Error(res.statusText);
      }

      setSuccess("Tombstones cleaned successfully.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
    } finally {
      setCleaning(false);
    }
  };

  return (
    <InfoPageStack>
      <InfoPageCard title="Delete Series" icon={IconTrash}>
        <Stack gap="md">
          <Alert
            icon={<IconAlertTriangle size={16} />}
            color="yellow"
            title="Warning"
          >
            This operation marks matching series for deletion. Deleted data
            cannot be recovered. Use Clean Tombstones afterwards to reclaim disk
            space.
          </Alert>

          {error && (
            <Alert color="red" title="Error" withCloseButton onClose={() => setError(null)}>
              {error}
            </Alert>
          )}

          {success && (
            <Alert
              icon={<IconCheck size={16} />}
              color="green"
              title="Success"
              withCloseButton
              onClose={() => setSuccess(null)}
            >
              {success}
            </Alert>
          )}

          <TextInput
            label="Series selector(s)"
            description="PromQL series selectors, one per line. Example: up{job=&quot;prometheus&quot;}"
            placeholder={'up{job="prometheus"}'}
            value={matchers}
            onChange={(e) => setMatchers(e.currentTarget.value)}
            required
          />

          <Group grow>
            <TextInput
              label="Start time"
              description="RFC3339 or Unix timestamp (optional)"
              placeholder="2024-01-01T00:00:00Z"
              value={startTime}
              onChange={(e) => setStartTime(e.currentTarget.value)}
            />
            <TextInput
              label="End time"
              description="RFC3339 or Unix timestamp (optional)"
              placeholder="2024-12-31T23:59:59Z"
              value={endTime}
              onChange={(e) => setEndTime(e.currentTarget.value)}
            />
          </Group>

          <Group>
            <Button
              color="red"
              leftSection={<IconTrash size={16} />}
              onClick={handleDelete}
              loading={deleting}
              disabled={!matchers.trim()}
            >
              Delete series
            </Button>
          </Group>
        </Stack>
      </InfoPageCard>

      <InfoPageCard title="Clean Tombstones">
        <Stack gap="md">
          <Text size="sm" c="dimmed">
            After deleting series, tombstones mark the data for deletion but
            do not free disk space immediately. Use this to remove tombstones
            and reclaim storage.
          </Text>
          <Group>
            <Button
              variant="outline"
              onClick={handleCleanTombstones}
              loading={cleaning}
            >
              Clean tombstones
            </Button>
          </Group>
        </Stack>
      </InfoPageCard>
    </InfoPageStack>
  );
}

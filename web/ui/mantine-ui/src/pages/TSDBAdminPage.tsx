import { useState } from "react";
import {
  Alert,
  Button,
  Group,
  Modal,
  Stack,
  Text,
  TextInput,
} from "@mantine/core";
import { DateTimePicker } from "@mantine/dates";
import { useDisclosure } from "@mantine/hooks";
import { notifications } from "@mantine/notifications";
import {
  IconAlertTriangle,
  IconTrash,
  IconEraser,
  IconPlus,
  IconX,
} from "@tabler/icons-react";
import { useSuspenseAPIQuery } from "../api/api";
import { useSettings } from "../state/settingsSlice";
import InfoPageStack from "../components/InfoPageStack";
import InfoPageCard from "../components/InfoPageCard";
import { inputIconStyle, buttonIconStyle } from "../styles";
import dayjs from "dayjs";

interface FeaturesResult {
  [category: string]: {
    [feature: string]: boolean;
  };
}

export default function TSDBAdminPage() {
  const { pathPrefix } = useSettings();

  const {
    data: { data: features },
  } = useSuspenseAPIQuery<FeaturesResult>({ path: `/status/features` });

  const adminAPIEnabled = features?.api?.admin ?? false;

  const [matchers, setMatchers] = useState<string[]>([""]);
  const [startTime, setStartTime] = useState<string | null>(null);
  const [endTime, setEndTime] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isCleaning, setIsCleaning] = useState(false);

  const [
    deleteModalOpened,
    { open: openDeleteModal, close: closeDeleteModal },
  ] = useDisclosure(false);
  const [
    cleanModalOpened,
    { open: openCleanModal, close: closeCleanModal },
  ] = useDisclosure(false);

  const addMatcher = () => {
    setMatchers([...matchers, ""]);
  };

  const removeMatcher = (index: number) => {
    setMatchers(matchers.filter((_, i) => i !== index));
  };

  const updateMatcher = (index: number, value: string) => {
    const newMatchers = [...matchers];
    newMatchers[index] = value;
    setMatchers(newMatchers);
  };

  const validMatchers = matchers.filter((m) => m.trim() !== "");

  const handleDeleteSeries = async () => {
    setIsDeleting(true);
    closeDeleteModal();

    try {
      const params = new URLSearchParams();
      validMatchers.forEach((m) => params.append("match[]", m));
      if (startTime) {
        params.append("start", (dayjs(startTime).valueOf() / 1000).toString());
      }
      if (endTime) {
        params.append("end", (dayjs(endTime).valueOf() / 1000).toString());
      }

      const response = await fetch(
        `${pathPrefix}/api/v1/admin/tsdb/delete_series`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/x-www-form-urlencoded",
          },
          body: params.toString(),
          credentials: "same-origin",
        }
      );

      const result = await response.json();

      if (result.status === "success") {
        notifications.show({
          title: "Series deleted",
          message: `Successfully deleted series matching: ${validMatchers.join(", ")}`,
          color: "green",
        });
        setMatchers([""]);
        setStartTime(null);
        setEndTime(null);
      } else {
        notifications.show({
          title: "Failed to delete series",
          message: result.error || "Unknown error occurred",
          color: "red",
        });
      }
    } catch (error) {
      notifications.show({
        title: "Failed to delete series",
        message: error instanceof Error ? error.message : "Network error",
        color: "red",
      });
    } finally {
      setIsDeleting(false);
    }
  };

  const handleCleanTombstones = async () => {
    setIsCleaning(true);
    closeCleanModal();

    try {
      const response = await fetch(
        `${pathPrefix}/api/v1/admin/tsdb/clean_tombstones`,
        {
          method: "POST",
          credentials: "same-origin",
        }
      );

      const result = await response.json();

      if (result.status === "success") {
        notifications.show({
          title: "Tombstones cleaned",
          message: "Successfully cleaned tombstones from TSDB",
          color: "green",
        });
      } else {
        notifications.show({
          title: "Failed to clean tombstones",
          message: result.error || "Unknown error occurred",
          color: "red",
        });
      }
    } catch (error) {
      notifications.show({
        title: "Failed to clean tombstones",
        message: error instanceof Error ? error.message : "Network error",
        color: "red",
      });
    } finally {
      setIsCleaning(false);
    }
  };

  return (
    <InfoPageStack>
      {!adminAPIEnabled && (
        <Alert
          icon={<IconAlertTriangle />}
          title="Admin APIs disabled"
          color="yellow"
        >
          The admin APIs are disabled. To enable them, start Prometheus with the{" "}
          <code>--web.enable-admin-api</code> flag.
        </Alert>
      )}

      <InfoPageCard title="Delete Series" icon={IconTrash}>
        <Stack gap="md">
          <Text size="sm" c="dimmed">
            Delete time series data matching the specified label matchers. The
            data will be marked for deletion and removed during the next
            compaction, or you can clean tombstones immediately after deletion.
          </Text>

          <Stack gap="xs">
            <Text fw={500} size="sm">
              Series selectors
            </Text>
            {matchers.map((matcher, index) => (
              <Group key={index} gap="xs">
                <TextInput
                  style={{ flex: 1 }}
                  placeholder='{__name__="metric_name", job="example"}'
                  value={matcher}
                  onChange={(e) => updateMatcher(index, e.currentTarget.value)}
                  disabled={!adminAPIEnabled}
                  leftSection={<IconTrash style={inputIconStyle} />}
                />
                {matchers.length > 1 && (
                  <Button
                    variant="subtle"
                    color="red"
                    size="sm"
                    onClick={() => removeMatcher(index)}
                    disabled={!adminAPIEnabled}
                  >
                    <IconX style={buttonIconStyle} />
                  </Button>
                )}
              </Group>
            ))}
            <Button
              variant="light"
              size="xs"
              leftSection={<IconPlus style={inputIconStyle} />}
              onClick={addMatcher}
              disabled={!adminAPIEnabled}
              style={{ alignSelf: "flex-start" }}
            >
              Add selector
            </Button>
          </Stack>

          <Group grow>
            <DateTimePicker
              label="Start time (optional)"
              placeholder="Beginning of time"
              valueFormat="YYYY-MM-DD HH:mm:ss"
              withSeconds
              value={startTime}
              onChange={(value) => setStartTime(value)}
              clearable
              disabled={!adminAPIEnabled}
            />
            <DateTimePicker
              label="End time (optional)"
              placeholder="End of time"
              valueFormat="YYYY-MM-DD HH:mm:ss"
              withSeconds
              value={endTime}
              onChange={(value) => setEndTime(value)}
              clearable
              disabled={!adminAPIEnabled}
            />
          </Group>

          <Button
            color="red"
            leftSection={<IconTrash style={buttonIconStyle} />}
            onClick={openDeleteModal}
            disabled={!adminAPIEnabled || validMatchers.length === 0}
            loading={isDeleting}
          >
            Delete series
          </Button>
        </Stack>
      </InfoPageCard>

      <InfoPageCard title="Clean Tombstones" icon={IconEraser}>
        <Stack gap="md">
          <Text size="sm" c="dimmed">
            Remove deleted data from disk permanently. After deleting series,
            the data is only marked for deletion (tombstoned). Running this
            operation will free up disk space by removing the tombstoned data.
          </Text>

          <Button
            color="orange"
            leftSection={<IconEraser style={buttonIconStyle} />}
            onClick={openCleanModal}
            disabled={!adminAPIEnabled}
            loading={isCleaning}
          >
            Clean tombstones
          </Button>
        </Stack>
      </InfoPageCard>

      <Modal
        opened={deleteModalOpened}
        onClose={closeDeleteModal}
        title="Confirm series deletion"
      >
        <Stack gap="md">
          <Alert icon={<IconAlertTriangle />} color="red">
            This action cannot be undone. The selected series data will be
            permanently marked for deletion.
          </Alert>

          <Text size="sm">
            You are about to delete series matching:
          </Text>
          <Stack gap="xs">
            {validMatchers.map((m, i) => (
              <code key={i}>{m}</code>
            ))}
          </Stack>
          {(startTime || endTime) && (
            <Text size="sm">
              Time range:{" "}
              {startTime ? dayjs(startTime).format("YYYY-MM-DD HH:mm:ss") : "beginning"} to{" "}
              {endTime ? dayjs(endTime).format("YYYY-MM-DD HH:mm:ss") : "now"}
            </Text>
          )}

          <Group justify="flex-end" gap="sm">
            <Button variant="default" onClick={closeDeleteModal}>
              Cancel
            </Button>
            <Button color="red" onClick={handleDeleteSeries}>
              Delete series
            </Button>
          </Group>
        </Stack>
      </Modal>

      <Modal
        opened={cleanModalOpened}
        onClose={closeCleanModal}
        title="Confirm clean tombstones"
      >
        <Stack gap="md">
          <Text size="sm">
            This will permanently remove all tombstoned (deleted) data from the
            TSDB. This operation may take some time depending on the amount of
            data.
          </Text>

          <Group justify="flex-end" gap="sm">
            <Button variant="default" onClick={closeCleanModal}>
              Cancel
            </Button>
            <Button color="orange" onClick={handleCleanTombstones}>
              Clean tombstones
            </Button>
          </Group>
        </Stack>
      </Modal>
    </InfoPageStack>
  );
}

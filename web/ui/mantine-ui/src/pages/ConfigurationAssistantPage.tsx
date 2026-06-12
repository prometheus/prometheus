import { useMemo, useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Card,
  Group,
  List,
  Stack,
  Table,
  Text,
  Textarea,
  Title,
} from "@mantine/core";
import { IconAlertTriangle, IconCheck } from "@tabler/icons-react";
import { useSettings } from "../state/settingsSlice";

type ScrapeJobPreview = {
  jobName: string;
  scrapeInterval: string;
  metricsPath: string;
  targets: string[];
};

type LocalAnalysisResult = {
  errors: string[];
  warnings: string[];
  suggestions: string[];
  jobs: ScrapeJobPreview[];
};

type ApplyResponse = {
  status: string;
  message: string;
  backupFile?: string;
  suggestions?: string[];
};

const sampleConfig = `global:
  scrape_interval: 15s

scrape_configs:
  - job_name: "prometheus"
    static_configs:
      - targets: ["localhost:9090"]`;

function extractValue(line: string): string {
  return line.split(":").slice(1).join(":").trim().replace(/^["']|["']$/g, "");
}

function extractTargets(line: string): string[] {
  const match = line.match(/\[(.*)\]/);
  if (!match) {
    return [];
  }

  return match[1]
    .split(",")
    .map((target) => target.trim().replace(/^["']|["']$/g, ""))
    .filter(Boolean);
}

function analyzePrometheusConfig(configText: string): LocalAnalysisResult {
  const errors: string[] = [];
  const warnings: string[] = [];
  const suggestions: string[] = [];
  const jobs: ScrapeJobPreview[] = [];

  const trimmedConfig = configText.trim();

  if (!trimmedConfig) {
    errors.push("Configuration is empty.");
    suggestions.push("Paste a prometheus.yml configuration before applying changes.");
    return { errors, warnings, suggestions, jobs };
  }

  if (!trimmedConfig.includes("scrape_configs:")) {
    errors.push("Missing required scrape_configs section.");
    suggestions.push("Add a scrape_configs section to define scrape jobs.");
  }

  const lines = trimmedConfig.split("\n");
  let currentJob: ScrapeJobPreview | null = null;
  let globalScrapeInterval = "default";

  for (const line of lines) {
    const trimmedLine = line.trim();

    if (trimmedLine.startsWith("scrape_interval:") && !currentJob) {
      globalScrapeInterval = extractValue(trimmedLine);
    }

    if (trimmedLine.startsWith("- job_name:")) {
      if (currentJob) {
        jobs.push(currentJob);
      }

      currentJob = {
        jobName: extractValue(trimmedLine),
        scrapeInterval: globalScrapeInterval,
        metricsPath: "/metrics",
        targets: [],
      };
    }

    if (currentJob && trimmedLine.startsWith("scrape_interval:")) {
      currentJob.scrapeInterval = extractValue(trimmedLine);
    }

    if (currentJob && trimmedLine.startsWith("metrics_path:")) {
      currentJob.metricsPath = extractValue(trimmedLine);
    }

    if (currentJob && trimmedLine.startsWith("- targets:")) {
      currentJob.targets = extractTargets(trimmedLine);
    }
  }

  if (currentJob) {
    jobs.push(currentJob);
  }

  if (trimmedConfig.includes("scrape_configs:") && jobs.length === 0) {
    warnings.push("scrape_configs exists, but no job_name was detected.");
    suggestions.push("Add at least one scrape job with job_name.");
  }

  for (const job of jobs) {
    if (job.targets.length === 0) {
      warnings.push(`Job "${job.jobName || "unknown"}" does not define targets.`);
      suggestions.push("Add static_configs with targets for each scrape job.");
    }
  }

  if (errors.length === 0 && warnings.length === 0) {
    suggestions.push("Local preview did not detect common configuration issues.");
  }

  return { errors, warnings, suggestions, jobs };
}

export default function ConfigurationAssistantPage() {
  const { pathPrefix } = useSettings();

  const [configText, setConfigText] = useState(sampleConfig);
  const [hasValidated, setHasValidated] = useState(false);
  const [isApplying, setIsApplying] = useState(false);
  const [applyResponse, setApplyResponse] = useState<ApplyResponse | null>(null);

  const localResult = useMemo(
    () => analyzePrometheusConfig(configText),
    [configText]
  );

  const hasLocalIssues =
    localResult.errors.length > 0 || localResult.warnings.length > 0;

  async function applyAndReload() {
    setIsApplying(true);
    setApplyResponse(null);

    try {
      const response = await fetch(`${pathPrefix}/-/config-assistant/apply`, {
        method: "POST",
        credentials: "same-origin",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ yaml: configText }),
      });

      const contentType = response.headers.get("content-type");
      const data = contentType?.includes("application/json")
        ? ((await response.json()) as ApplyResponse)
        : ({
            status: response.ok ? "success" : "error",
            message: await response.text(),
          } satisfies ApplyResponse);

      setApplyResponse(data);
    } catch (error) {
      setApplyResponse({
        status: "error",
        message: `Failed to apply configuration: ${String(error)}`,
      });
    } finally {
      setIsApplying(false);
    }
  }

  return (
    <Stack maw={1000} mx="auto" gap="md">
      <div>
        <Title order={2}>Configuration Assistant</Title>
        <Text c="dimmed">
          Validate, preview, and apply Prometheus configuration changes.
        </Text>
      </div>

      <Alert color="yellow" icon={<IconAlertTriangle />} title="Local prototype">
        This assistant writes to the Prometheus configuration file used by the
        running server and then triggers a reload. It requires the lifecycle API
        to be enabled.
      </Alert>

      <Textarea
        label="Prometheus configuration"
        description="Edit prometheus.yml content here."
        autosize
        minRows={14}
        value={configText}
        onChange={(event) => {
          setConfigText(event.currentTarget.value);
          setHasValidated(false);
          setApplyResponse(null);
        }}
      />

      <Group>
        <Button variant="default" onClick={() => setHasValidated(true)}>
          Validate locally
        </Button>
        <Button
          onClick={applyAndReload}
          loading={isApplying}
          disabled={hasLocalIssues}
        >
          Apply and reload
        </Button>
      </Group>

      {hasValidated && (
        <Card withBorder>
          <Title order={3}>Local validation</Title>

          {hasLocalIssues ? (
            <Stack mt="sm">
              {localResult.errors.length > 0 && (
                <div>
                  <Badge color="red">Errors</Badge>
                  <List mt="xs">
                    {localResult.errors.map((error) => (
                      <List.Item key={error}>{error}</List.Item>
                    ))}
                  </List>
                </div>
              )}

              {localResult.warnings.length > 0 && (
                <div>
                  <Badge color="yellow">Warnings</Badge>
                  <List mt="xs">
                    {localResult.warnings.map((warning) => (
                      <List.Item key={warning}>{warning}</List.Item>
                    ))}
                  </List>
                </div>
              )}
            </Stack>
          ) : (
            <Text mt="sm">No common local issues detected.</Text>
          )}
        </Card>
      )}

      <Card withBorder>
        <Title order={3}>Configuration preview</Title>

        {localResult.jobs.length === 0 ? (
          <Text mt="sm" c="dimmed">
            No scrape jobs could be previewed.
          </Text>
        ) : (
          <Table mt="sm" striped withTableBorder>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Job name</Table.Th>
                <Table.Th>Targets</Table.Th>
                <Table.Th>Scrape interval</Table.Th>
                <Table.Th>Metrics path</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {localResult.jobs.map((job) => (
                <Table.Tr key={job.jobName}>
                  <Table.Td>{job.jobName}</Table.Td>
                  <Table.Td>
                    {job.targets.length > 0
                      ? job.targets.join(", ")
                      : "No targets detected"}
                  </Table.Td>
                  <Table.Td>{job.scrapeInterval}</Table.Td>
                  <Table.Td>{job.metricsPath}</Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        )}
      </Card>

      <Card withBorder>
        <Title order={3}>Suggestions</Title>
        <List mt="sm">
          {localResult.suggestions.map((suggestion) => (
            <List.Item key={suggestion}>{suggestion}</List.Item>
          ))}
        </List>
      </Card>

      {applyResponse && (
        <Alert
          color={applyResponse.status === "success" ? "green" : "red"}
          icon={
            applyResponse.status === "success" ? (
              <IconCheck />
            ) : (
              <IconAlertTriangle />
            )
          }
          title={
            applyResponse.status === "success"
              ? "Configuration applied"
              : "Apply failed"
          }
        >
          <Stack gap="xs">
            <Text>{applyResponse.message}</Text>
            {applyResponse.backupFile && (
              <Text size="sm">Backup file: {applyResponse.backupFile}</Text>
            )}
            {applyResponse.suggestions && applyResponse.suggestions.length > 0 && (
              <List>
                {applyResponse.suggestions.map((suggestion) => (
                  <List.Item key={suggestion}>{suggestion}</List.Item>
                ))}
              </List>
            )}
          </Stack>
        </Alert>
      )}
    </Stack>
  );
}

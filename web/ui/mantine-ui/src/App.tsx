import "@mantine/core/styles.css";
import "@mantine/code-highlight/styles.css";
import "@mantine/notifications/styles.css";
import "@mantine/dates/styles.css";
import "./mantine-overrides.css";
import classes from "./App.module.css";
import PrometheusLogo from "./images/prometheus-logo.svg";

import {
  ActionIcon,
  AppShell,
  Box,
  Burger,
  Button,
  Group,
  MantineProvider,
  Menu,
  Skeleton,
  Text,
  createTheme,
  rem,
} from "@mantine/core";
import { useDisclosure } from "@mantine/hooks";
import {
  IconBell,
  IconBellFilled,
  IconBook,
  IconChevronDown,
  IconChevronRight,
  IconCloudDataConnection,
  IconDatabase,
  IconDeviceDesktopAnalytics,
  IconFlag,
  IconHeartRateMonitor,
  IconInfoCircle,
  IconSearch,
  IconServer,
  IconServerCog,
} from "@tabler/icons-react";
import {
  BrowserRouter,
  Link,
  NavLink,
  Navigate,
  Route,
  Routes,
} from "react-router-dom";
import { IconTable } from "@tabler/icons-react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import QueryPage from "./pages/query/QueryPage";
import AlertsPage from "./pages/AlertsPage";
import RulesPage from "./pages/RulesPage";
import TargetsPage from "./pages/targets/TargetsPage";
import StatusPage from "./pages/StatusPage";
import TSDBStatusPage from "./pages/TSDBStatusPage";
import FlagsPage from "./pages/FlagsPage";
import ConfigPage from "./pages/ConfigPage";
import AgentPage from "./pages/AgentPage";
import { Suspense } from "react";
import ErrorBoundary from "./components/ErrorBoundary";
import { ThemeSelector } from "./components/ThemeSelector";
import { Notifications } from "@mantine/notifications";
import { useSettings } from "./state/settingsSlice";
import SettingsMenu from "./components/SettingsMenu";
import ReadinessWrapper from "./components/ReadinessWrapper";
import NotificationsProvider from "./components/NotificationsProvider";
import NotificationsIcon from "./components/NotificationsIcon";
import { QueryParamProvider } from "use-query-params";
import { ReactRouter6Adapter } from "use-query-params/adapters/react-router-6";
import ServiceDiscoveryPage from "./pages/service-discovery/ServiceDiscoveryPage";
import AlertmanagerDiscoveryPage from "./pages/AlertmanagerDiscoveryPage";
import { actionIconStyle, navIconStyle } from "./styles";

import {
  CodeHighlightAdapterProvider,
  createHighlightJsAdapter,
} from "@mantine/code-highlight";
import hljs from "highlight.js/lib/core";
import "./highlightjs.css";
import yamlLang from "highlight.js/lib/languages/yaml";

hljs.registerLanguage("yaml", yamlLang);

const highlightJsAdapter = createHighlightJsAdapter(hljs);

const queryClient = new QueryClient();

const mainNavPages = [
  {
    title: "Query",
    path: "/query",
    icon: <IconSearch style={navIconStyle} />,
    element: <QueryPage />,
    inAgentMode: false,
  },
  {
    title: "Alerts",
    path: "/alerts",
    icon: <IconBellFilled style={navIconStyle} />,
    element: <AlertsPage />,
    inAgentMode: false,
  },
];

const monitoringStatusPages = [
  {
    title: "Target health",
    path: "/targets",
    icon: <IconHeartRateMonitor style={navIconStyle} />,
    element: <TargetsPage />,
    inAgentMode: true,
  },
  {
    title: "Rule health",
    path: "/rules",
    icon: <IconTable style={navIconStyle} />,
    element: <RulesPage />,
    inAgentMode: false,
  },
  {
    title: "Service discovery",
    path: "/service-discovery",
    icon: <IconCloudDataConnection style={navIconStyle} />,
    element: <ServiceDiscoveryPage />,
    inAgentMode: true,
  },
];

const serverStatusPages = [
  {
    title: "Runtime & build information",
    path: "/status",
    icon: <IconInfoCircle style={navIconStyle} />,
    element: <StatusPage />,
    inAgentMode: true,
  },
  {
    title: "TSDB status",
    path: "/tsdb-status",
    icon: <IconDatabase style={navIconStyle} />,
    element: <TSDBStatusPage />,
    inAgentMode: false,
  },
  {
    title: "Command-line flags",
    path: "/flags",
    icon: <IconFlag style={navIconStyle} />,
    element: <FlagsPage />,
    inAgentMode: true,
  },
  {
    title: "Configuration",
    path: "/config",
    icon: <IconServerCog style={navIconStyle} />,
    element: <ConfigPage />,
    inAgentMode: true,
  },
  {
    title: "Alertmanager discovery",
    path: "/alertmanager-discovery",
    icon: <IconBell style={navIconStyle} />,
    element: <AlertmanagerDiscoveryPage />,
    inAgentMode: false,
  },
];

const allStatusPages = [...monitoringStatusPages, ...serverStatusPages];

const theme = createTheme({
  colors: {
    "codebox-bg": [
      "#f5f5f5",
      "#e7e7e7",
      "#cdcdcd",
      "#b2b2b2",
      "#9a9a9a",
      "#8b8b8b",
      "#848484",
      "#717171",
      "#656565",
      "#575757",
    ],
  },
});

const navLinkXPadding = "md";

function App() {
  const [opened, { toggle }] = useDisclosure();

  const { agentMode, consolesLink, pathPrefix } = useSettings();

  const navLinks = (
    <>
      {consolesLink && (
        <Button
          component="a"
          href={consolesLink}
          className={classes.link}
          leftSection={<IconDeviceDesktopAnalytics style={navIconStyle} />}
          px={navLinkXPadding}
        >
          Consoles
        </Button>
      )}

      {mainNavPages
        .filter((p) => !agentMode || p.inAgentMode)
        .map((p) => (
          <Button
            key={p.path}
            component={NavLink}
            to={p.path}
            className={classes.link}
            leftSection={p.icon}
            px={navLinkXPadding}
          >
            {p.title}
          </Button>
        ))}

      <Menu shadow="md" width={240}>
        <Routes>
          {allStatusPages
            .filter((p) => !agentMode || p.inAgentMode)
            .map((p) => (
              <Route
                key={p.path}
                path={p.path}
                element={
                  <Menu.Target>
                    <Button
                      component={NavLink}
                      to={p.path}
                      className={classes.link}
                      leftSection={p.icon}
                      rightSection={<IconChevronDown style={navIconStyle} />}
                      px={navLinkXPadding}
                      onClick={(e) => e.preventDefault()}
                    >
                      Status <IconChevronRight style={navIconStyle} /> {p.title}
                    </Button>
                  </Menu.Target>
                }
              />
            ))}
          <Route
            path="*"
            element={
              <Menu.Target>
                <Button
                  className={classes.link}
                  leftSection={<IconServer style={navIconStyle} />}
                  rightSection={<IconChevronDown style={navIconStyle} />}
                  px={navLinkXPadding}
                >
                  Status
                </Button>
              </Menu.Target>
            }
          />
        </Routes>

        <Menu.Dropdown>
          <Menu.Label>Monitoring status</Menu.Label>
          {monitoringStatusPages
            .filter((p) => !agentMode || p.inAgentMode)
            .map((p) => (
              <Menu.Item
                key={p.path}
                component={NavLink}
                to={p.path}
                leftSection={p.icon}
              >
                {p.title}
              </Menu.Item>
            ))}

          <Menu.Divider />
          <Menu.Label>Server status</Menu.Label>
          {serverStatusPages
            .filter((p) => !agentMode || p.inAgentMode)
            .map((p) => (
              <Menu.Item
                key={p.path}
                component={NavLink}
                to={p.path}
                leftSection={p.icon}
              >
                {p.title}
              </Menu.Item>
            ))}
        </Menu.Dropdown>
      </Menu>
    </>
  );

  const navActionIcons = (
    <>
      <ThemeSelector />
      <NotificationsIcon />
      <SettingsMenu />
      <ActionIcon
        component="a"
        href="https://prometheus.io/docs/prometheus/latest/getting_started/"
        target="_blank"
        color="gray"
        title="Documentation"
        aria-label="Documentation"
        size={rem(32)}
      >
        <IconBook style={actionIconStyle} />
      </ActionIcon>
    </>
  );

  return (
    <BrowserRouter basename={pathPrefix}>
      <QueryParamProvider adapter={ReactRouter6Adapter}>
        <MantineProvider defaultColorScheme="auto" theme={theme}>
          <CodeHighlightAdapterProvider adapter={highlightJsAdapter}>
            <Notifications position="top-right" />

            <QueryClientProvider client={queryClient}>
              <AppShell
                header={{ height: 56 }}
                navbar={{
                  width: 300,
                  // TODO: On pages with a long title like "/status", the navbar
                  // breaks in an ugly way for narrow windows. Fix this.
                  breakpoint: "sm",
                  collapsed: { desktop: true, mobile: !opened },
                }}
                padding="md"
              >
                <NotificationsProvider>
                  <AppShell.Header bg="rgb(65, 73, 81)" c="#fff">
                    <Group h="100%" px="md" wrap="nowrap">
                      <Group
                        style={{ flex: 1 }}
                        justify="space-between"
                        wrap="nowrap"
                      >
                        <Group gap={40} wrap="nowrap">
                          <Link
                            to="/"
                            style={{ textDecoration: "none", color: "white" }}
                          >
                            <Group gap={10} wrap="nowrap">
                              <img src={PrometheusLogo} height={30} />
                              <Text hiddenFrom="sm" fz={20}>
                                Prometheus
                              </Text>
                              <Text visibleFrom="md" fz={20}>
                                Prometheus
                              </Text>
                              <Text fz={20}>{agentMode && "Agent"}</Text>
                            </Group>
                          </Link>
                          <Group gap={12} visibleFrom="sm" wrap="nowrap">
                            {navLinks}
                          </Group>
                        </Group>
                        <Group visibleFrom="xs" wrap="nowrap" gap="xs">
                          {navActionIcons}
                        </Group>
                      </Group>
                      <Burger
                        opened={opened}
                        onClick={toggle}
                        hiddenFrom="sm"
                        size="sm"
                        color="gray.2"
                      />
                    </Group>
                  </AppShell.Header>

                  <AppShell.Navbar py="md" px={4} bg="rgb(65, 73, 81)" c="#fff">
                    {navLinks}
                    <Group mt="md" hiddenFrom="xs" justify="center">
                      {navActionIcons}
                    </Group>
                  </AppShell.Navbar>
                </NotificationsProvider>

                <AppShell.Main>
                  <ErrorBoundary key={location.pathname}>
                    <Suspense
                      fallback={
                        <Box mt="lg">
                          {Array.from(Array(10), (_, i) => (
                            <Skeleton
                              key={i}
                              height={40}
                              mb={15}
                              width={1000}
                              mx="auto"
                            />
                          ))}
                        </Box>
                      }
                    >
                      <Routes>
                        <Route
                          path="/"
                          element={
                            <Navigate
                              to={agentMode ? "/agent" : "/query"}
                              replace
                            />
                          }
                        />
                        {agentMode ? (
                          <Route
                            path="/agent"
                            element={
                              <ReadinessWrapper>
                                <AgentPage />
                              </ReadinessWrapper>
                            }
                          />
                        ) : (
                          <>
                            <Route
                              path="/query"
                              element={
                                <ReadinessWrapper>
                                  <QueryPage />
                                </ReadinessWrapper>
                              }
                            />
                            <Route
                              path="/alerts"
                              element={
                                <ReadinessWrapper>
                                  <AlertsPage />
                                </ReadinessWrapper>
                              }
                            />
                          </>
                        )}
                        {allStatusPages.map((p) => (
                          <Route
                            key={p.path}
                            path={p.path}
                            element={
                              <ReadinessWrapper>{p.element}</ReadinessWrapper>
                            }
                          />
                        ))}
                      </Routes>
                    </Suspense>
                  </ErrorBoundary>
                </AppShell.Main>
              </AppShell>
              {/* <ReactQueryDevtools initialIsOpen={false} /> */}
            </QueryClientProvider>
          </CodeHighlightAdapterProvider>
        </MantineProvider>
      </QueryParamProvider>
    </BrowserRouter>
  );
}

export default App;

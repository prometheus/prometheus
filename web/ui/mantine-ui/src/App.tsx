import "@mantine/core/styles.css";
import "@mantine/code-highlight/styles.css";
import "@mantine/notifications/styles.css";
import "@mantine/dates/styles.css";
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
  IconAdjustments,
  IconBellFilled,
  IconChevronDown,
  IconChevronRight,
  IconCloudDataConnection,
  IconDatabase,
  IconDatabaseSearch,
  IconFileAnalytics,
  IconFlag,
  IconHeartRateMonitor,
  IconInfoCircle,
  IconServerCog,
  IconSettings,
} from "@tabler/icons-react";
import {
  BrowserRouter,
  NavLink,
  Navigate,
  Route,
  Routes,
} from "react-router-dom";
import { IconTable } from "@tabler/icons-react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
// import { ReactQueryDevtools } from "react-query/devtools";
import QueryPage from "./pages/query/QueryPage";
import AlertsPage from "./pages/AlertsPage";
import RulesPage from "./pages/RulesPage";
import TargetsPage from "./pages/TargetsPage";
import ServiceDiscoveryPage from "./pages/ServiceDiscoveryPage";
import StatusPage from "./pages/StatusPage";
import TSDBStatusPage from "./pages/TSDBStatusPage";
import FlagsPage from "./pages/FlagsPage";
import ConfigPage from "./pages/ConfigPage";
import AgentPage from "./pages/AgentPage";
import { Suspense, useContext } from "react";
import ErrorBoundary from "./ErrorBoundary";
import { ThemeSelector } from "./ThemeSelector";
import { SettingsContext } from "./settings";
import { Notifications } from "@mantine/notifications";

const queryClient = new QueryClient();

const monitoringStatusPages = [
  {
    title: "Targets",
    path: "/targets",
    icon: <IconHeartRateMonitor style={{ width: rem(14), height: rem(14) }} />,
    element: <TargetsPage />,
  },
  {
    title: "Rules",
    path: "/rules",
    icon: <IconTable style={{ width: rem(14), height: rem(14) }} />,
    element: <RulesPage />,
  },
  {
    title: "Service discovery",
    path: "/service-discovery",
    icon: (
      <IconCloudDataConnection style={{ width: rem(14), height: rem(14) }} />
    ),
    element: <ServiceDiscoveryPage />,
  },
];

const serverStatusPages = [
  {
    title: "Runtime & build information",
    path: "/status",
    icon: <IconInfoCircle style={{ width: rem(14), height: rem(14) }} />,
    element: <StatusPage />,
  },
  {
    title: "TSDB status",
    path: "/tsdb-status",
    icon: <IconDatabase style={{ width: rem(14), height: rem(14) }} />,
    element: <TSDBStatusPage />,
  },
  {
    title: "Command-line flags",
    path: "/flags",
    icon: <IconFlag style={{ width: rem(14), height: rem(14) }} />,
    element: <FlagsPage />,
  },
  {
    title: "Configuration",
    path: "/config",
    icon: <IconServerCog style={{ width: rem(14), height: rem(14) }} />,
    element: <ConfigPage />,
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

const navLinkIconSize = 15;
const navLinkXPadding = "md";

function App() {
  const [opened, { toggle }] = useDisclosure();
  const { agentMode } = useContext(SettingsContext);

  const navLinks = (
    <>
      <Button
        component={NavLink}
        to="/query"
        className={classes.link}
        leftSection={<IconDatabaseSearch size={navLinkIconSize} />}
        px={navLinkXPadding}
      >
        Query
      </Button>
      <Button
        component={NavLink}
        to="/alerts"
        className={classes.link}
        leftSection={<IconBellFilled size={navLinkIconSize} />}
        px={navLinkXPadding}
      >
        Alerts
      </Button>

      <Menu shadow="md" width={230}>
        <Routes>
          {allStatusPages.map((p) => (
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
                    rightSection={<IconChevronDown size={navLinkIconSize} />}
                    px={navLinkXPadding}
                  >
                    Status <IconChevronRight size={navLinkIconSize} /> {p.title}
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
                  component={NavLink}
                  to="/"
                  className={classes.link}
                  leftSection={<IconFileAnalytics size={navLinkIconSize} />}
                  rightSection={<IconChevronDown size={navLinkIconSize} />}
                  onClick={(e) => {
                    e.preventDefault();
                  }}
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
          {monitoringStatusPages.map((p) => (
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
          {serverStatusPages.map((p) => (
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

      {/* <Button
        component="a"
        href="https://prometheus.io/docs/prometheus/latest/getting_started/"
        className={classes.link}
        leftSection={<IconHelp size={navLinkIconSize} />}
        target="_blank"
        px={navLinkXPadding}
      >
        Help
      </Button> */}
    </>
  );

  return (
    <BrowserRouter>
      <MantineProvider defaultColorScheme="auto" theme={theme}>
        <Notifications position="top-right" />

        <QueryClientProvider client={queryClient}>
          <AppShell
            header={{ height: 56 }}
            navbar={{
              width: 300,
              breakpoint: "sm",
              collapsed: { desktop: true, mobile: !opened },
            }}
            padding="md"
          >
            <AppShell.Header bg="rgb(65, 73, 81)" c="#fff">
              <Group h="100%" px="md">
                <Group style={{ flex: 1 }} justify="space-between">
                  <Group gap={10} w={150}>
                    <img src={PrometheusLogo} height={30} />
                    <Text fz={20}>Prometheus{agentMode && " Agent"}</Text>
                  </Group>
                  <Group gap={12} visibleFrom="sm">
                    {navLinks}
                  </Group>
                  <Group w={180} justify="flex-end">
                    {<ThemeSelector />}
                    <Menu shadow="md" width={200}>
                      <Menu.Target>
                        <ActionIcon
                          // variant=""
                          color="gray"
                          aria-label="Settings"
                          size="md"
                        >
                          <IconSettings size={navLinkIconSize} />
                        </ActionIcon>
                      </Menu.Target>
                      <Menu.Dropdown>
                        <Menu.Item
                          component={NavLink}
                          to="/"
                          leftSection={<IconAdjustments />}
                        >
                          Settings
                        </Menu.Item>
                      </Menu.Dropdown>
                    </Menu>
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
            </AppShell.Navbar>

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
                        <Navigate to={agentMode ? "/agent" : "/query"} />
                      }
                    />
                    <Route path="/query" element={<QueryPage />} />
                    <Route path="/agent" element={<AgentPage />} />
                    <Route path="/alerts" element={<AlertsPage />} />
                    {allStatusPages.map((p) => (
                      <Route key={p.path} path={p.path} element={p.element} />
                    ))}
                  </Routes>
                </Suspense>
              </ErrorBoundary>
            </AppShell.Main>
          </AppShell>
          {/* <ReactQueryDevtools initialIsOpen={false} /> */}
        </QueryClientProvider>
      </MantineProvider>
    </BrowserRouter>
  );
}

export default App;

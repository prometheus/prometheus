import "@mantine/core/styles.css";
import "@mantine/code-highlight/styles.css";
import "@mantine/notifications/styles.css";
import "@mantine/dates/styles.css";
import classes from "./App.module.css";
import PrometheusLogo from "./images/prometheus-logo.svg";

import {
  ActionIcon,
  Affix,
  AppShell,
  Box,
  Burger,
  Button,
  Group,
  MantineProvider,
  Menu,
  Skeleton,
  Text,
  Transition,
  createTheme,
  rem,
} from "@mantine/core";
import { useDisclosure, useWindowScroll } from "@mantine/hooks";
import {
  IconAdjustments,
  IconArrowUp,
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
import { useAppDispatch } from "./state/hooks";
import { updateSettings } from "./state/settingsSlice";

const queryClient = new QueryClient();

const navIconStyle = { width: rem(15), height: rem(15) };

const mainNavPages = [
  {
    title: "Query",
    path: "/query",
    icon: <IconDatabaseSearch style={navIconStyle} />,
    element: <QueryPage />,
  },
  {
    title: "Alerts",
    path: "/alerts",
    icon: <IconBellFilled style={navIconStyle} />,
    element: <AlertsPage />,
  },
];

const monitoringStatusPages = [
  {
    title: "Targets",
    path: "/targets",
    icon: <IconHeartRateMonitor style={navIconStyle} />,
    element: <TargetsPage />,
  },
  {
    title: "Rules",
    path: "/rules",
    icon: <IconTable style={navIconStyle} />,
    element: <RulesPage />,
  },
  {
    title: "Service discovery",
    path: "/service-discovery",
    icon: <IconCloudDataConnection style={navIconStyle} />,
    element: <ServiceDiscoveryPage />,
  },
];

const serverStatusPages = [
  {
    title: "Runtime & build information",
    path: "/status",
    icon: <IconInfoCircle style={navIconStyle} />,
    element: <StatusPage />,
  },
  {
    title: "TSDB status",
    path: "/tsdb-status",
    icon: <IconDatabase style={navIconStyle} />,
    element: <TSDBStatusPage />,
  },
  {
    title: "Command-line flags",
    path: "/flags",
    icon: <IconFlag style={navIconStyle} />,
    element: <FlagsPage />,
  },
  {
    title: "Configuration",
    path: "/config",
    icon: <IconServerCog style={navIconStyle} />,
    element: <ConfigPage />,
  },
];

const allStatusPages = [...monitoringStatusPages, ...serverStatusPages];
const allPages = [...mainNavPages, ...allStatusPages];

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

// This dynamically/generically determines the pathPrefix by stripping the first known
// endpoint suffix from the window location path. It works out of the box for both direct
// hosting and reverse proxy deployments with no additional configurations required.
const getPathPrefix = (path: string) => {
  if (path.endsWith("/")) {
    path = path.slice(0, -1);
  }

  const pagePath = allPages.find((p) => path.endsWith(p.path))?.path;
  return path.slice(0, path.length - (pagePath || "").length);
};

const navLinkXPadding = "md";

function App() {
  const [scroll, scrollTo] = useWindowScroll();
  const [opened, { toggle }] = useDisclosure();
  const { agentMode } = useContext(SettingsContext);

  const pathPrefix = getPathPrefix(window.location.pathname);
  const dispatch = useAppDispatch();
  dispatch(updateSettings({ pathPrefix }));

  const navLinks = (
    <>
      {mainNavPages.map((p) => (
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
                    rightSection={<IconChevronDown style={navIconStyle} />}
                    px={navLinkXPadding}
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
                  component={NavLink}
                  to="/"
                  className={classes.link}
                  leftSection={<IconFileAnalytics style={navIconStyle} />}
                  rightSection={<IconChevronDown style={navIconStyle} />}
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
        leftSection={<IconHelp style={navIconStyle} />}
        target="_blank"
        px={navLinkXPadding}
      >
        Help
      </Button> */}
    </>
  );

  return (
    <BrowserRouter basename={pathPrefix}>
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
              <Affix position={{ bottom: 20, right: 20 }}>
                <Transition transition="slide-up" mounted={scroll.y > 0}>
                  {(transitionStyles) => (
                    <Button
                      leftSection={
                        <IconArrowUp
                          style={{ width: rem(16), height: rem(16) }}
                        />
                      }
                      style={transitionStyles}
                      onClick={() => scrollTo({ y: 0 })}
                    >
                      Scroll to top
                    </Button>
                  )}
                </Transition>
              </Affix>
            </AppShell.Main>
          </AppShell>
          {/* <ReactQueryDevtools initialIsOpen={false} /> */}
        </QueryClientProvider>
      </MantineProvider>
    </BrowserRouter>
  );
}

export default App;

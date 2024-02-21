import "@mantine/core/styles.css";
import "@mantine/code-highlight/styles.css";
import classes from "./App.module.css";
import PrometheusLogo from "./images/prometheus-logo.svg";

import {
    AppShell,
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
    IconBellFilled,
    IconChartAreaFilled,
    IconChevronDown,
    IconChevronRight,
    IconCloudDataConnection,
    IconDatabase,
    IconFileAnalytics,
    IconFlag,
    IconHeartRateMonitor,
    IconHelp,
    IconInfoCircle,
    IconServerCog,
} from "@tabler/icons-react";
import {
    BrowserRouter,
    NavLink,
    Navigate,
    Route,
    Routes,
} from "react-router-dom";
import Graph from "./pages/graph";
import Alerts from "./pages/alerts";
import { IconTable } from "@tabler/icons-react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
// import { ReactQueryDevtools } from "react-query/devtools";
import Rules from "./pages/rules";
import Targets from "./pages/targets";
import ServiceDiscovery from "./pages/service-discovery";
import Status from "./pages/status";
import TSDBStatus from "./pages/tsdb-status";
import Flags from "./pages/flags";
import Config from "./pages/config";
import { Suspense, useContext } from "react";
import ErrorBoundary from "./error-boundary";
import { ThemeSelector } from "./theme-selector";
import { SettingsContext } from "./settings";
import Agent from "./pages/agent";

const queryClient = new QueryClient();

const monitoringStatusPages = [
    {
        title: "Targets",
        path: "/targets",
        icon: <IconHeartRateMonitor style={{ width: rem(14), height: rem(14) }} />,
        element: <Targets />,
    },
    {
        title: "Rules",
        path: "/rules",
        icon: <IconTable style={{ width: rem(14), height: rem(14) }} />,
        element: <Rules />,
    },
    {
        title: "Service discovery",
        path: "/service-discovery",
        icon: (
            <IconCloudDataConnection style={{ width: rem(14), height: rem(14) }} />
        ),
        element: <ServiceDiscovery />,
    },
];

const serverStatusPages = [
    {
        title: "Runtime & build information",
        path: "/status",
        icon: <IconInfoCircle style={{ width: rem(14), height: rem(14) }} />,
        element: <Status />,
    },
    {
        title: "TSDB status",
        path: "/tsdb-status",
        icon: <IconDatabase style={{ width: rem(14), height: rem(14) }} />,
        element: <TSDBStatus />,
    },
    {
        title: "Command-line flags",
        path: "/flags",
        icon: <IconFlag style={{ width: rem(14), height: rem(14) }} />,
        element: <Flags />,
    },
    {
        title: "Configuration",
        path: "/config",
        icon: <IconServerCog style={{ width: rem(14), height: rem(14) }} />,
        element: <Config />,
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

function App() {
    const [opened, { toggle }] = useDisclosure();
    const { agentMode } = useContext(SettingsContext);

    const navLinks = (
        <>
            <Button
                component={NavLink}
                to="/graph"
                className={classes.link}
                leftSection={<IconChartAreaFilled size={16} />}
            >
                Graph
            </Button>
            <Button
                component={NavLink}
                to="/alerts"
                className={classes.link}
                leftSection={<IconBellFilled size={16} />}
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
                                        rightSection={<IconChevronDown size={16} />}
                                    >
                                        Status <IconChevronRight size={16} /> {p.title}
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
                                    leftSection={<IconFileAnalytics size={16} />}
                                    rightSection={<IconChevronDown size={16} />}
                                    onClick={(e) => {
                                        e.preventDefault();
                                    }}
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

            <Button
                component="a"
                href="https://prometheus.io/docs/prometheus/latest/getting_started/"
                className={classes.link}
                leftSection={<IconHelp size={16} />}
                target="_blank"
            >
                Help
            </Button>
        </>
    );

    return (
        <BrowserRouter>
            <MantineProvider defaultColorScheme="auto" theme={theme}>
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
                                <Group style={{ flex: 1 }}>
                                    <Group gap={10}>
                                        <img src={PrometheusLogo} height={30} />
                                        <Text fz={20}>Prometheus{agentMode && " Agent"}</Text>
                                    </Group>
                                    <Group ml="lg" gap={12} visibleFrom="sm">
                                        {navLinks}
                                    </Group>
                                </Group>
                                <Burger
                                    opened={opened}
                                    onClick={toggle}
                                    hiddenFrom="sm"
                                    size="sm"
                                    color="gray.2"
                                />
                                {<ThemeSelector />}
                            </Group>
                        </AppShell.Header>

                        <AppShell.Navbar py="md" px={4} bg="rgb(65, 73, 81)" c="#fff">
                            {navLinks}
                        </AppShell.Navbar>

                        <AppShell.Main>
                            <ErrorBoundary key={location.pathname}>
                                <Suspense
                                    fallback={Array.from(Array(10), (_, i) => (
                                        <Skeleton key={i} height={40} mb={15} width={1000} />
                                    ))}
                                >
                                    <Routes>
                                        <Route
                                            path="/"
                                            element={
                                                <Navigate to={agentMode ? "/agent" : "/graph"} />
                                            }
                                        />
                                        <Route path="/graph" element={<Graph />} />
                                        <Route path="/agent" element={<Agent />} />
                                        <Route path="/alerts" element={<Alerts />} />
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
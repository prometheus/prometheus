import { FC } from 'react';
import { Container } from 'reactstrap';
import Navigation from './Navbar';

import { BrowserRouter as Router, Redirect, Route, Switch } from 'react-router-dom';
import { PathPrefixContext } from './contexts/PathPrefixContext';
import { ThemeContext, themeName, themeSetting } from './contexts/ThemeContext';
import { ReadyContext } from './contexts/ReadyContext';
import { AnimateLogoContext } from './contexts/AnimateLogoContext';
import { useLocalStorage } from './hooks/useLocalStorage';
import useMedia from './hooks/useMedia';
import {
  AgentPage,
  AlertsPage,
  ConfigPage,
  FlagsPage,
  PanelListPage,
  RulesPage,
  ServiceDiscoveryPage,
  StatusPage,
  TargetsPage,
  TSDBStatusPage,
} from './pages';
import { Theme, themeLocalStorageKey } from './Theme';

interface AppProps {
  consolesLink: string | null;
  agentMode: boolean;
  ready: boolean;
}

const App: FC<AppProps> = ({ consolesLink, agentMode, ready }) => {
  // This dynamically/generically determines the pathPrefix by stripping the first known
  // endpoint suffix from the window location path. It works out of the box for both direct
  // hosting and reverse proxy deployments with no additional configurations required.
  let basePath = window.location.pathname;
  const paths = [
    '/agent',
    '/graph',
    '/alerts',
    '/status',
    '/tsdb-status',
    '/flags',
    '/config',
    '/rules',
    '/targets',
    '/service-discovery',
  ];
  if (basePath.endsWith('/')) {
    basePath = basePath.slice(0, -1);
  }
  if (basePath.length > 1) {
    for (let i = 0; i < paths.length; i++) {
      if (basePath.endsWith(paths[i])) {
        basePath = basePath.slice(0, basePath.length - paths[i].length);
        break;
      }
    }
  }

  const [userTheme, setUserTheme] = useLocalStorage<themeSetting>(themeLocalStorageKey, 'auto');
  const browserHasThemes = useMedia('(prefers-color-scheme)');
  const browserWantsDarkTheme = useMedia('(prefers-color-scheme: dark)');
  const [animateLogo, setAnimateLogo] = useLocalStorage<boolean>('animateLogo', false);

  let theme: themeName;
  if (userTheme !== 'auto') {
    theme = userTheme;
  } else {
    theme = browserHasThemes ? (browserWantsDarkTheme ? 'dark' : 'light') : 'light';
  }

  return (
    <ThemeContext.Provider
      value={{ theme: theme, userPreference: userTheme, setTheme: (t: themeSetting) => setUserTheme(t) }}
    >
      <Theme />
      <PathPrefixContext.Provider value={basePath}>
        <ReadyContext.Provider value={ready}>
          <Router basename={basePath}>
            <AnimateLogoContext.Provider value={animateLogo}>
              <Navigation consolesLink={consolesLink} agentMode={agentMode} animateLogo={animateLogo} />
              <Container fluid style={{ paddingTop: 70 }}>
                <Switch>
                  <Redirect exact from="/" to={agentMode ? '/agent' : '/graph'} />
                  {/*
              NOTE: Any route added here needs to also be added to the list of
              React-handled router paths ("reactRouterPaths") in /web/web.go.
            */}
                  <Route path="/agent">
                    <AgentPage />
                  </Route>
                  <Route path="/graph">
                    <PanelListPage />
                  </Route>
                  <Route path="/alerts">
                    <AlertsPage />
                  </Route>
                  <Route path="/config">
                    <ConfigPage />
                  </Route>
                  <Route path="/flags">
                    <FlagsPage />
                  </Route>
                  <Route path="/rules">
                    <RulesPage />
                  </Route>
                  <Route path="/service-discovery">
                    <ServiceDiscoveryPage />
                  </Route>
                  <Route path="/status">
                    <StatusPage agentMode={agentMode} setAnimateLogo={setAnimateLogo} />
                  </Route>
                  <Route path="/tsdb-status">
                    <TSDBStatusPage />
                  </Route>
                  <Route path="/targets">
                    <TargetsPage />
                  </Route>
                </Switch>
              </Container>
            </AnimateLogoContext.Provider>
          </Router>
        </ReadyContext.Provider>
      </PathPrefixContext.Provider>
    </ThemeContext.Provider>
  );
};

export default App;

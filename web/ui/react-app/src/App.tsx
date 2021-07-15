import React, { FC } from 'react';
import Navigation from './Navbar';
import { Container } from 'reactstrap';

import { Router, Redirect } from '@reach/router';
import useMedia from 'use-media';
import {
  AlertsPage,
  ConfigPage,
  FlagsPage,
  RulesPage,
  ServiceDiscoveryPage,
  StatusPage,
  TargetsPage,
  TSDBStatusPage,
  PanelListPage,
} from './pages';
import { PathPrefixContext } from './contexts/PathPrefixContext';
import { ThemeContext, themeName, themeSetting } from './contexts/ThemeContext';
import { Theme, themeLocalStorageKey } from './Theme';
import { useLocalStorage } from './hooks/useLocalStorage';

interface AppProps {
  consolesLink: string | null;
}

const App: FC<AppProps> = ({ consolesLink }) => {
  // This dynamically/generically determines the pathPrefix by stripping the first known
  // endpoint suffix from the window location path. It works out of the box for both direct
  // hosting and reverse proxy deployments with no additional configurations required.
  let basePath = window.location.pathname;
  const paths = [
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
        <Navigation consolesLink={consolesLink} />
        <Container fluid style={{ paddingTop: 70 }}>
          <Router basepath={`${basePath}`}>
            <Redirect from="/" to={`graph`} noThrow />
            {/*
              NOTE: Any route added here needs to also be added to the list of
              React-handled router paths ("reactRouterPaths") in /web/web.go.
            */}
            <PanelListPage path="/graph" />
            <AlertsPage path="/alerts" />
            <ConfigPage path="/config" />
            <FlagsPage path="/flags" />
            <RulesPage path="/rules" />
            <ServiceDiscoveryPage path="/service-discovery" />
            <StatusPage path="/status" />
            <TSDBStatusPage path="/tsdb-status" />
            <TargetsPage path="/targets" />
          </Router>
        </Container>
      </PathPrefixContext.Provider>
    </ThemeContext.Provider>
  );
};

export default App;

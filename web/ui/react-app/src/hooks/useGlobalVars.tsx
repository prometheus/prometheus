import React, { useContext } from 'react';
import PathPrefixProps from '../types/PathPrefixProps';
import { isPresent } from '../utils';

type contextProps = PathPrefixProps & {
  consolesLink?: string | null;
};

// Declared/defined in public/index.html, value replaced by Prometheus when serving bundle.
declare const GLOBAL_PATH_PREFIX: string;
declare const GLOBAL_CONSOLES_LINK: string;

const globalVarsContext = React.createContext<contextProps>({
  pathPrefix: '',
  consolesLink: null,
});

export const GlobalVarsProvider: React.FC = ({ children }) => {
  let pathPrefix = GLOBAL_PATH_PREFIX;
  let consolesLink: string | null = GLOBAL_CONSOLES_LINK;

  if (GLOBAL_PATH_PREFIX === 'PATH_PREFIX_PLACEHOLDER' || GLOBAL_PATH_PREFIX === '/' || !isPresent(GLOBAL_PATH_PREFIX)) {
    // Either we are running the app outside of Prometheus, so the placeholder value in
    // the index.html didn't get replaced, or we have a '/' prefix, which we also need to
    // normalize to '' to make concatenations work (prefixes like '/foo/bar/' already get
    // their trailing slash stripped by Prometheus).
    pathPrefix = '';
  }

  if (
    GLOBAL_CONSOLES_LINK === 'CONSOLES_LINK_PLACEHOLDER' ||
    GLOBAL_CONSOLES_LINK === '' ||
    !isPresent(GLOBAL_CONSOLES_LINK)
  ) {
    consolesLink = null;
  }

  return <globalVarsContext.Provider value={{ pathPrefix, consolesLink }}>{children}</globalVarsContext.Provider>;
};

const useGlobalVars = () => {
  const { pathPrefix, consolesLink } = useContext(globalVarsContext);
  return { pathPrefix, consolesLink };
};

export default useGlobalVars;

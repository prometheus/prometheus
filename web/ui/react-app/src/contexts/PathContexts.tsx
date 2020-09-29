import React from 'react';

const PathPrefixContext = React.createContext('');

function usePathPrefix() {
  const context = React.useContext(PathPrefixContext);
  return context;
}

export { usePathPrefix, PathPrefixContext };

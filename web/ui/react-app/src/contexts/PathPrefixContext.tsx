import React from 'react';

const PathPrefixContext = React.createContext('');

function usePathPrefix() {
  return React.useContext(PathPrefixContext);
}

export { usePathPrefix, PathPrefixContext };

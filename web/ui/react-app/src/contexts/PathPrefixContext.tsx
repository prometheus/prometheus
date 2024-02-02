import React from 'react';

const PathPrefixContext = React.createContext('');

function usePathPrefix(): string {
  return React.useContext(PathPrefixContext);
}

export { usePathPrefix, PathPrefixContext };

import React from 'react';

const PathPrefixContext = React.createContext('');

function usePathPrefix() {
  const context = React.useContext(PathPrefixContext);
  return context;
}

const APIPathContext = React.createContext(
  // When the React version of the UI is relocated from /new to /,
  // remove the "../" prefix here and all of the things will still work.
  '../api/v1'
);

function useAPIPath() {
  const context = React.useContext(APIPathContext);
  return context;
}

export { useAPIPath, usePathPrefix, PathPrefixContext, APIPathContext };

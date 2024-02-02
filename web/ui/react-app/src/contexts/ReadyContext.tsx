import React from 'react';

const ReadyContext = React.createContext(false);

function useReady(): boolean {
  return React.useContext(ReadyContext);
}

export { useReady, ReadyContext };

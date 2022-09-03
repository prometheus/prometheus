import React from 'react';

const ClipboardContext = React.createContext((msg: string) => {
  return;
});

function useClipboardContext() {
  return React.useContext(ClipboardContext);
}

export { useClipboardContext, ClipboardContext };

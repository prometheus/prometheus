import React from 'react';

const ToastContext = React.createContext((msg: string) => {
  return;
});

function useToastContext() {
  return React.useContext(ToastContext);
}

export { useToastContext, ToastContext };

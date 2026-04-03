import React, { createContext, FC, useContext } from 'react';

const TargetScrapeProxyContext = createContext<boolean>(false);

export const TargetScrapeProxyProvider: FC<{ enabled: boolean; children: React.ReactNode }> = ({ enabled, children }) => (
  <TargetScrapeProxyContext.Provider value={enabled}>{children}</TargetScrapeProxyContext.Provider>
);

export const useTargetScrapeProxyEnabled = () => useContext(TargetScrapeProxyContext);


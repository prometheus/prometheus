import React, { createContext, FC, useContext } from 'react';

const TargetScrapeProxyContext = createContext<boolean>(false);

const TargetScrapeProxyProvider: FC<{ enabled: boolean; children: React.ReactNode }> = ({ enabled, children }) => (
  <TargetScrapeProxyContext.Provider value={enabled}>{children}</TargetScrapeProxyContext.Provider>
);

function useTargetScrapeProxyEnabled(): boolean {
  return useContext(TargetScrapeProxyContext);
}

export { TargetScrapeProxyProvider, useTargetScrapeProxyEnabled };

import { createContext, useContext } from "react";

import type { Locale } from "./i18n";

export type I18n = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: string, values?: Record<string, string | number>) => string;
};

const fallbackI18n: I18n = {
  locale: "en",
  setLocale: () => undefined,
  t: (key) => key,
};

export const I18nContext = createContext<I18n>(fallbackI18n);

export function useI18n(): I18n {
  return useContext(I18nContext);
}

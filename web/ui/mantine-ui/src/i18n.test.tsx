import { act, renderHook } from "@testing-library/react";
import { PropsWithChildren } from "react";
import { describe, expect, it } from "vitest";

import { I18nProvider } from "./i18n";
import { useI18n } from "./i18nContext";

const wrapper = ({ children }: PropsWithChildren) => (
  <I18nProvider>{children}</I18nProvider>
);

describe("i18n", () => {
  it("uses Swedish messages after switching locale", () => {
    const { result } = renderHook(() => useI18n(), { wrapper });

    act(() => result.current.setLocale("sv"));

    expect(result.current.t("nav.alerts")).toBe("Larm");
  });

  it("uses English as fallback", () => {
    const { result } = renderHook(() => useI18n(), { wrapper });

    expect(result.current.t("nav.alerts")).toBe("Alerts");
  });

  it("interpolates named values", () => {
    const { result } = renderHook(() => useI18n(), { wrapper });

    act(() => result.current.setLocale("sv"));

    expect(result.current.t("alerts.emptyGroupsSummary", { count: 3 })).toBe(
      "Döljer 3 tomma grupper på grund av filter eller avsaknad av regler."
    );
  });
});

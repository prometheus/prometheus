import {
  Popover,
  ActionIcon,
  Fieldset,
  Checkbox,
  Stack,
  Group,
  NumberInput,
  Select,
} from "@mantine/core";
import { IconSettings } from "@tabler/icons-react";
import { FC } from "react";
import { useAppDispatch } from "../state/hooks";
import { updateSettings, useSettings } from "../state/settingsSlice";
import { actionIconStyle } from "../styles";
import { useI18n } from "../i18nContext";

const SettingsMenu: FC = () => {
  const {
    useLocalTime,
    enableQueryHistory,
    enableAutocomplete,
    enableSyntaxHighlighting,
    enableLinter,
    showAnnotations,
    showQueryWarnings,
    showQueryInfoNotices,
    ruleGroupsPerPage,
    alertGroupsPerPage,
  } = useSettings();
  const dispatch = useAppDispatch();
  const { locale, setLocale, t } = useI18n();

  return (
    <Popover position="bottom" withArrow shadow="md">
      <Popover.Target>
        <ActionIcon
          color="gray"
          title={t("settings.settings")}
          aria-label={t("settings.settings")}
          size={32}
        >
          <IconSettings style={actionIconStyle} />
        </ActionIcon>
      </Popover.Target>
      <Popover.Dropdown>
        <Group align="flex-start">
          <Stack>
            <Fieldset p="md" legend={t("settings.global")}>
              <Select
                label={t("settings.language")}
                data={[
                  { value: "en", label: t("settings.english") },
                  { value: "sv", label: t("settings.swedish") },
                ]}
                value={locale}
                onChange={(value) => {
                  if (value === "en" || value === "sv") {
                    setLocale(value);
                  }
                }}
              />
              <Checkbox
                checked={useLocalTime}
                label={t("settings.useLocalTime")}
                onChange={(event) =>
                  dispatch(
                    updateSettings({
                      useLocalTime: event.currentTarget.checked,
                    })
                  )
                }
              />
            </Fieldset>

            <Fieldset p="md" legend={t("settings.queryPage")}>
              <Stack>
                <Checkbox
                  checked={enableQueryHistory}
                  label={t("settings.enableQueryHistory")}
                  onChange={(event) =>
                    dispatch(
                      updateSettings({
                        enableQueryHistory: event.currentTarget.checked,
                      })
                    )
                  }
                />
                <Checkbox
                  checked={enableAutocomplete}
                  label={t("settings.enableAutocomplete")}
                  onChange={(event) =>
                    dispatch(
                      updateSettings({
                        enableAutocomplete: event.currentTarget.checked,
                      })
                    )
                  }
                />
                <Checkbox
                  checked={enableSyntaxHighlighting}
                  label={t("settings.enableSyntaxHighlighting")}
                  onChange={(event) =>
                    dispatch(
                      updateSettings({
                        enableSyntaxHighlighting: event.currentTarget.checked,
                      })
                    )
                  }
                />
                <Checkbox
                  checked={enableLinter}
                  label={t("settings.enableLinter")}
                  onChange={(event) =>
                    dispatch(
                      updateSettings({
                        enableLinter: event.currentTarget.checked,
                      })
                    )
                  }
                />
                <Checkbox
                  checked={showQueryWarnings}
                  label={t("settings.showQueryWarnings")}
                  onChange={(event) =>
                    dispatch(
                      updateSettings({
                        showQueryWarnings: event.currentTarget.checked,
                      })
                    )
                  }
                />
                <Checkbox
                  checked={showQueryInfoNotices}
                  label={t("settings.showQueryInfoNotices")}
                  onChange={(event) =>
                    dispatch(
                      updateSettings({
                        showQueryInfoNotices: event.currentTarget.checked,
                      })
                    )
                  }
                />
              </Stack>
            </Fieldset>
          </Stack>

          <Stack>
            <Fieldset p="md" legend={t("settings.alertsPage")}>
              <Stack>
                <Checkbox
                  checked={showAnnotations}
                  label={t("settings.showExpandedAnnotations")}
                  onChange={(event) =>
                    dispatch(
                      updateSettings({
                        showAnnotations: event.currentTarget.checked,
                      })
                    )
                  }
                />
                <NumberInput
                  min={1}
                  allowDecimal={false}
                  label={t("settings.alertGroupsPerPage")}
                  value={alertGroupsPerPage}
                  onChange={(value) => {
                    if (typeof value !== "number") {
                      return;
                    }

                    dispatch(
                      updateSettings({
                        alertGroupsPerPage: value,
                      })
                    );
                  }}
                />
              </Stack>
            </Fieldset>
            <Fieldset p="md" legend={t("settings.rulesPage")}>
              <NumberInput
                min={1}
                allowDecimal={false}
                label={t("settings.ruleGroupsPerPage")}
                value={ruleGroupsPerPage}
                onChange={(value) => {
                  if (typeof value !== "number") {
                    return;
                  }

                  dispatch(
                    updateSettings({
                      ruleGroupsPerPage: value,
                    })
                  );
                }}
              />
            </Fieldset>
          </Stack>
        </Group>
      </Popover.Dropdown>
    </Popover>
  );
};

export default SettingsMenu;

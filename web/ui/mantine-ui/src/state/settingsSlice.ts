import { PayloadAction, createSlice } from "@reduxjs/toolkit";

interface Settings {
  pathPrefix: string;
  useLocalTime: boolean;
  enableQueryHistory: boolean;
  enableAutocomplete: boolean;
  enableSyntaxHighlighting: boolean;
  enableLinter: boolean;
  showAnnotations: boolean;
}

const initialState: Settings = {
  pathPrefix: "",
  useLocalTime: false,
  enableQueryHistory: false,
  enableAutocomplete: true,
  enableSyntaxHighlighting: true,
  enableLinter: true,
  showAnnotations: false,
};

export const settingsSlice = createSlice({
  name: "settings",
  initialState,
  reducers: {
    updateSettings: (state, { payload }: PayloadAction<Partial<Settings>>) => {
      Object.assign(state, payload);
    },
  },
});

export const { updateSettings } = settingsSlice.actions;

export default settingsSlice.reducer;

import { PayloadAction, createSlice } from "@reduxjs/toolkit";

interface Settings {
  pathPrefix: string;
}

const initialState: Settings = {
  pathPrefix: "",
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

import { PayloadAction, createSlice } from "@reduxjs/toolkit";

interface AlertFilters {
  state: string[];
}

interface AlertsPage {
  filters: AlertFilters;
}

const initialState: AlertsPage = {
  filters: {
    state: [],
  },
};

export const alertsPageSlice = createSlice({
  name: "alertsPage",
  initialState,
  reducers: {
    updateAlertFilters: (
      state,
      { payload }: PayloadAction<Partial<AlertFilters>>
    ) => {
      Object.assign(state.filters, payload);
    },
  },
});

export const { updateAlertFilters } = alertsPageSlice.actions;

export default alertsPageSlice.reducer;

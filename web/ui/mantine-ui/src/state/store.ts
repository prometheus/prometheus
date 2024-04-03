import { configureStore } from "@reduxjs/toolkit";
import queryPageSlice from "./queryPageSlice";
import settingsSlice from "./settingsSlice";
import targetsPageSlice from "./targetsPageSlice";
import alertsPageSlice from "./alertsPageSlice";
import { localStorageMiddleware } from "./localStorageMiddleware";

const store = configureStore({
  reducer: {
    settings: settingsSlice,
    queryPage: queryPageSlice,
    targetsPage: targetsPageSlice,
    alertsPage: alertsPageSlice,
  },
  middleware: (getDefaultMiddleware) =>
    getDefaultMiddleware().prepend(localStorageMiddleware.middleware),
});

// Infer the `RootState` and `AppDispatch` types from the store itself
export type RootState = ReturnType<typeof store.getState>;
// Inferred type: {posts: PostsState, comments: CommentsState, users: UsersState}
export type AppDispatch = typeof store.dispatch;

export default store;

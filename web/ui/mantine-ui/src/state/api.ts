import { createApi, fetchBaseQuery } from "@reduxjs/toolkit/query/react";
import { ErrorAPIResponse, SuccessAPIResponse } from "../api/api";
import { InstantQueryResult } from "../api/responseTypes/query";

// Define a service using a base URL and expected endpoints
export const prometheusApi = createApi({
  reducerPath: "prometheusApi",
  baseQuery: fetchBaseQuery({ baseUrl: "/api/v1/" }),
  keepUnusedDataFor: 0, // Turn off caching.
  endpoints: (builder) => ({
    instantQuery: builder.query<
      SuccessAPIResponse<InstantQueryResult>,
      { query: string; time: number }
    >({
      query: ({ query, time }) => {
        return {
          url: `query`,
          params: {
            query,
            time,
          },
        };
        //`query?query=${encodeURIComponent(query)}&time=${time}`,
      },
      transformErrorResponse: (error): string => {
        if (!error.data) {
          return "Failed to fetch data";
        }

        return (error.data as ErrorAPIResponse).error;
      },
      // transformResponse: (
      //   response: APIResponse<InstantQueryResult>
      // ): SuccessAPIResponse<InstantQueryResult> => {
      //   if (!response.status) {
      //     throw new Error("Invalid response");
      //   }
      //   if (response.status === "error") {
      //     throw new Error(response.error);
      //   }
      //   return response;
      // },
    }),
  }),
});

// Export hooks for usage in functional components, which are
// auto-generated based on the defined endpoints
export const { useInstantQueryQuery, useLazyInstantQueryQuery } = prometheusApi;

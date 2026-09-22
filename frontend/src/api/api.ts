import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react'

export type Space = { id: string; name: string; description?: string; address?: string; role: string }
export type Item = { id: string; name: string; description?: string; boxId?: string; state: string; photoCount: number }

export const api = createApi({
  reducerPath: 'api',
  baseQuery: fetchBaseQuery({ baseUrl: '/api/v1/', credentials: 'include' }),
  tagTypes: ['Space', 'Item', 'Box', 'Location'],
  endpoints: (build) => ({
    spaces: build.query<Space[], void>({ query: () => 'spaces', providesTags: ['Space'] }),
    items: build.query<Item[], string>({ query: (spaceId) => `spaces/${spaceId}/items`, providesTags: ['Item'] }),
    search: build.query<Item[], { spaceId: string; q: string }>({ query: ({ spaceId, q }) => `spaces/${spaceId}/search?q=${encodeURIComponent(q)}` }),
    createSpace: build.mutation<Space, { name: string; description?: string; address?: string }>({ query: (body) => ({ url: 'spaces', method: 'POST', body }), invalidatesTags: ['Space'] }),
    createBox: build.mutation<{ id: string }, { spaceId: string; name: string; locationId?: string }>({ query: ({ spaceId, ...body }) => ({ url: `spaces/${spaceId}/boxes`, method: 'POST', body }), invalidatesTags: ['Box'] }),
    createItem: build.mutation<{ id: string }, { spaceId: string; name: string; description?: string; boxId?: string; mediaIds: string[] }>({ query: ({ spaceId, ...body }) => ({ url: `spaces/${spaceId}/items`, method: 'POST', body }), invalidatesTags: ['Item', 'Box'] })
  })
})
export const { useSpacesQuery, useItemsQuery, useSearchQuery, useCreateSpaceMutation } = api

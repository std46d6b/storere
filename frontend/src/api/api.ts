import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react'

export type ApiError = { code: string; message: string; fieldErrors?: Record<string, string> }
export type User = { id: string; username: string; displayName: string; status?: string }
export type Space = { id: string; name: string; description?: string; address?: string; role: 'owner' | 'admin' | 'editor' | 'viewer' }
export type Media = { id: string; contentType: string; byteSize: number; url: string }
export type Tag = { id: string; name: string; color?: string }
export type Item = { id: string; name: string; description?: string; boxId?: string; state: string; photoCount: number; media?: Media[]; tags?: Tag[]; boxName?: string; locationName?: string }
export type Box = { id: string; name: string; description?: string; currentLocationId?: string; temporaryLocationId?: string; state: string; itemCount: number }
export type Location = { id: string; name: string; code?: string; description?: string; icon?: string; parentLocationId?: string; state: string }

type Credentials = { username: string; password: string; displayName?: string }
type CreateSpace = { name: string; description?: string; address?: string }

export const api = createApi({
  reducerPath: 'api',
  baseQuery: fetchBaseQuery({ baseUrl: import.meta.env.MODE === 'test' ? 'http://localhost/api/v1/' : '/api/v1/', credentials: 'include' }),
  tagTypes: ['Session', 'Space', 'Item', 'Box', 'Location', 'Media', 'Tag'],
  endpoints: (build) => ({
    me: build.query<User, void>({ query: () => 'auth/me', providesTags: ['Session'] }),
    login: build.mutation<User, Credentials>({ query: (body) => ({ url: 'auth/login', method: 'POST', body }), invalidatesTags: ['Session'] }),
    register: build.mutation<User, Required<Credentials>>({ query: (body) => ({ url: 'auth/register', method: 'POST', body }), invalidatesTags: ['Session'] }),
    logout: build.mutation<void, void>({ query: () => ({ url: 'auth/logout', method: 'POST' }), invalidatesTags: ['Session', 'Space', 'Item', 'Box', 'Location', 'Media', 'Tag'] }),
    spaces: build.query<Space[], void>({ query: () => 'spaces', providesTags: ['Space'] }),
    createSpace: build.mutation<Space, CreateSpace>({ query: (body) => ({ url: 'spaces', method: 'POST', body }), invalidatesTags: ['Space'] }),
    items: build.query<Item[], string>({ query: (spaceId) => `spaces/${spaceId}/items`, providesTags: ['Item'] }),
    search: build.query<Item[], { spaceId: string; q: string }>({ query: ({ spaceId, q }) => `spaces/${spaceId}/search?q=${encodeURIComponent(q)}`, providesTags: ['Item'] }),
    boxes: build.query<Box[], string>({ query: (spaceId) => `spaces/${spaceId}/boxes`, providesTags: ['Box'] }),
    locations: build.query<Location[], string>({ query: (spaceId) => `spaces/${spaceId}/locations`, providesTags: ['Location'] }),
    createLocation: build.mutation<Location, { spaceId: string; name: string; code?: string; description?: string; icon?: string }>({ query: ({ spaceId, ...body }) => ({ url: `spaces/${spaceId}/locations`, method: 'POST', body }), invalidatesTags: ['Location'] }),
    createBox: build.mutation<Box, { spaceId: string; name: string; description?: string; locationId?: string }>({ query: ({ spaceId, ...body }) => ({ url: `spaces/${spaceId}/boxes`, method: 'POST', body }), invalidatesTags: ['Box'] }),
    createItem: build.mutation<Item, { spaceId: string; name: string; description?: string; boxId?: string; mediaIds: string[] }>({ query: ({ spaceId, ...body }) => ({ url: `spaces/${spaceId}/items`, method: 'POST', body }), invalidatesTags: ['Item', 'Box'] }),
    uploadMedia: build.mutation<Media, { spaceId: string; file: File }>({
      query: ({ spaceId, file }) => {
        const body = new FormData()
        body.set('spaceId', spaceId)
        body.set('file', file)
        return { url: 'media', method: 'POST', body }
      },
      invalidatesTags: ['Media']
    })
  })
})

export const {
  useMeQuery, useLoginMutation, useRegisterMutation, useLogoutMutation,
  useSpacesQuery, useCreateSpaceMutation, useItemsQuery, useSearchQuery,
  useBoxesQuery, useLocationsQuery, useCreateLocationMutation, useCreateBoxMutation,
  useCreateItemMutation, useUploadMediaMutation
} = api

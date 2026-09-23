import { Provider } from 'react-redux'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'
import { store } from './app/store'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  store.dispatch({ type: 'api/resetApiState' })
})

describe('App', () => {
  it('shows sign in when there is no active session', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 'unauthorized', message: 'Sign in required' }), { status: 401, headers: { 'Content-Type': 'application/json' } })))

    render(<Provider store={store}><App /></Provider>)

    await waitFor(() => expect(screen.getByRole('heading', { name: /войти/i })).toBeInTheDocument())
    expect(screen.getByRole('button', { name: /войти/i })).toBeInTheDocument()
  })

  it('enters the first-space setup after a successful login', async () => {
    let meCalls = 0
    vi.stubGlobal('fetch', vi.fn(async (request: Request) => {
      if (request.url.endsWith('/auth/me')) {
        meCalls += 1
        if (meCalls === 1) return new Response(JSON.stringify({ code: 'unauthorized', message: 'Sign in required' }), { status: 401, headers: { 'Content-Type': 'application/json' } })
        return new Response(JSON.stringify({ id: 'user-1', username: 'misha', displayName: 'Misha' }), { headers: { 'Content-Type': 'application/json' } })
      }
      if (request.url.endsWith('/auth/login')) return new Response(JSON.stringify({ id: 'user-1' }), { headers: { 'Content-Type': 'application/json' } })
      if (request.url.endsWith('/spaces')) return new Response('[]', { headers: { 'Content-Type': 'application/json' } })
      return new Response('{}', { status: 404, headers: { 'Content-Type': 'application/json' } })
    }))

    render(<Provider store={store}><App /></Provider>)
    await screen.findByRole('heading', { name: /войти/i })
    fireEvent.change(screen.getByLabelText(/имя пользователя/i), { target: { value: 'misha' } })
    fireEvent.change(screen.getByLabelText(/^пароль/i), { target: { value: 'very-long-password' } })
    fireEvent.click(screen.getByRole('button', { name: /^войти$/i }))

    await waitFor(() => expect(screen.getByRole('heading', { name: /где находятся ваши вещи/i })).toBeInTheDocument())
  })

  it('opens a box to show its contents, history, and settings', async () => {
    const calls: { url: string; method: string }[] = []
    vi.stubGlobal('fetch', vi.fn(async (request: Request) => {
      calls.push({ url: request.url, method: request.method })
      const url = new URL(request.url)
      const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
      if (url.pathname.endsWith('/auth/me')) return json({ id: 'user-1', username: 'misha', displayName: 'Misha' })
      if (url.pathname.endsWith('/spaces')) return json([{ id: 'space-1', name: 'Квартира', role: 'owner' }])
      if (url.pathname.endsWith('/locations')) return json([{ id: 'location-1', name: 'Кладовая', state: 'active' }])
      if (url.pathname.endsWith('/boxes')) return json([{ id: 'box-1', name: 'Архив', description: 'Документы', currentLocationId: 'location-1', state: 'active', itemCount: 1 }])
      if (url.pathname.endsWith('/items')) return json([{ id: 'item-1', name: 'Паспорт', boxId: 'box-1', state: 'active', photoCount: 1, media: [{ id: 'media-1', url: '/api/v1/media/media-1' }] }])
      if (url.pathname.endsWith('/box/box-1/timeline')) return json([{ action: 'created', occurredAt: '2026-09-23T00:00:00Z' }])
      if (url.pathname.endsWith('/boxes/box-1') && request.method === 'PATCH') return new Response(null, { status: 204 })
      if (url.pathname.endsWith('/boxes/box-1/move') && request.method === 'PATCH') return new Response(null, { status: 204 })
      return json({ code: 'not_found', message: 'Unexpected request' }, 404)
    }))

    render(<Provider store={store}><App /></Provider>)
    await screen.findByRole('heading', { name: /мои вещи/i })
    fireEvent.click(screen.getByRole('button', { name: /коробки/i }))
    await screen.findByRole('button', { name: /архив/i })
    fireEvent.click(screen.getByRole('button', { name: /архив/i }))

    expect((await screen.findAllByRole('heading', { name: 'Архив' })).length).toBeGreaterThan(1)
    expect(screen.getByText('Паспорт')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Паспорт' })).toHaveAttribute('src', '/api/v1/media/media-1')
    fireEvent.click(screen.getByRole('button', { name: 'Развернуть карточку' }))
    expect(screen.getByRole('dialog')).toHaveClass('expanded')
    expect(screen.getByRole('button', { name: 'Сузить карточку' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: /история/i }))
    expect(await screen.findByText('created')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: /настройки/i }))
    fireEvent.click(screen.getByRole('button', { name: /сохранить место/i }))
    await waitFor(() => expect(calls).toContainEqual(expect.objectContaining({ method: 'PATCH', url: expect.stringContaining('/boxes/box-1/move') })))
  })
})

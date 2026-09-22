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
})

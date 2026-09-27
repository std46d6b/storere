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
		vi.stubGlobal(
			'fetch',
			vi.fn().mockResolvedValue(
				new Response(JSON.stringify({ code: 'unauthorized', message: 'Sign in required' }), {
					status: 401,
					headers: { 'Content-Type': 'application/json' }
				})
			)
		)

		render(
			<Provider store={store}>
				<App />
			</Provider>
		)

		await waitFor(() => expect(screen.getByRole('heading', { name: /войти/i })).toBeInTheDocument())
		expect(screen.getByRole('button', { name: /войти/i })).toBeInTheDocument()
	})

	it('enters the first-space setup after a successful login', async () => {
		let meCalls = 0
		vi.stubGlobal(
			'fetch',
			vi.fn(async (request: Request) => {
				if (request.url.endsWith('/auth/me')) {
					meCalls += 1
					if (meCalls === 1)
						return new Response(
							JSON.stringify({ code: 'unauthorized', message: 'Sign in required' }),
							{ status: 401, headers: { 'Content-Type': 'application/json' } }
						)
					return new Response(
						JSON.stringify({ id: 'user-1', username: 'misha', displayName: 'Misha' }),
						{ headers: { 'Content-Type': 'application/json' } }
					)
				}
				if (request.url.endsWith('/auth/login'))
					return new Response(JSON.stringify({ id: 'user-1' }), {
						headers: { 'Content-Type': 'application/json' }
					})
				if (request.url.endsWith('/spaces'))
					return new Response('[]', { headers: { 'Content-Type': 'application/json' } })
				return new Response('{}', { status: 404, headers: { 'Content-Type': 'application/json' } })
			})
		)

		render(
			<Provider store={store}>
				<App />
			</Provider>
		)
		await screen.findByRole('heading', { name: /войти/i })
		fireEvent.change(screen.getByLabelText(/имя пользователя/i), { target: { value: 'misha' } })
		fireEvent.change(screen.getByLabelText(/^пароль/i), { target: { value: 'very-long-password' } })
		fireEvent.click(screen.getByRole('button', { name: /^войти$/i }))

		await waitFor(() =>
			expect(screen.getByRole('heading', { name: /где находятся ваши вещи/i })).toBeInTheDocument()
		)
	})

	it('opens a box to show its contents, history, and settings', async () => {
		const calls: { url: string; method: string; body?: unknown }[] = []
		vi.stubGlobal(
			'fetch',
			vi.fn(async (request: Request) => {
				calls.push({
					url: request.url,
					method: request.method,
					body: ['PATCH', 'POST'].includes(request.method)
						? await request.clone().json()
						: undefined
				})
				const url = new URL(request.url)
				const json = (body: unknown, status = 200) =>
					new Response(JSON.stringify(body), {
						status,
						headers: { 'Content-Type': 'application/json' }
					})
				if (url.pathname.endsWith('/auth/me'))
					return json({ id: 'user-1', username: 'misha', displayName: 'Misha' })
				if (url.pathname.endsWith('/spaces'))
					return json([{ id: 'space-1', name: 'Квартира', role: 'owner' }])
				if (url.pathname.endsWith('/locations'))
					return json([{ id: 'location-1', name: 'Кладовая', state: 'active' }])
				if (url.pathname.endsWith('/boxes'))
					return json([
						{
							id: 'box-1',
							name: 'Архив',
							description: 'Документы',
							currentLocationId: 'location-1',
							state: 'active',
							itemCount: 1
						},
						{
							id: 'box-2',
							name: 'Инструменты',
							state: 'active',
							itemCount: 1
						}
					])
				if (url.pathname.endsWith('/items'))
					return json([
						{
							id: 'item-1',
							name: 'Паспорт',
							boxId: 'box-1',
							state: 'active',
							photoCount: 2,
							media: [
								{ id: 'media-1', url: '/api/v1/media/media-1' },
								{ id: 'media-2', url: '/api/v1/media/media-2' }
							]
						},
						{
							id: 'item-2',
							name: 'Ключи',
							boxId: 'box-2',
							state: 'active',
							photoCount: 0
						},
						{
							id: 'item-3',
							name: 'Запасной ключ',
							state: 'active',
							photoCount: 0
						}
					])
				if (url.pathname.endsWith('/search'))
					return json([
						{
							id: 'item-1',
							name: 'Паспорт',
							boxId: 'box-1',
							state: 'active',
							photoCount: 2,
							media: [
								{ id: 'media-1', url: '/api/v1/media/media-1' },
								{ id: 'media-2', url: '/api/v1/media/media-2' }
							]
						}
					])
				if (url.pathname.endsWith('/box/box-1/timeline'))
					return json([{ action: 'created', occurredAt: '2026-09-23T00:00:00Z' }])
				if (url.pathname.endsWith('/boxes/box-1') && request.method === 'PATCH')
					return new Response(null, { status: 204 })
				if (url.pathname.endsWith('/items/item-1/media') && request.method === 'PATCH')
					return new Response(null, { status: 204 })
				if (url.pathname.endsWith('/items/item-1/media/media-1') && request.method === 'DELETE')
					return new Response(null, { status: 204 })
				if (url.pathname.endsWith('/items/item-1') && request.method === 'PATCH')
					return new Response(null, { status: 204 })
				if (url.pathname.endsWith('/boxes/box-1/move') && request.method === 'PATCH')
					return new Response(null, { status: 204 })
				return json({ code: 'not_found', message: 'Unexpected request' }, 404)
			})
		)

		render(
			<Provider store={store}>
				<App />
			</Provider>
		)
		await screen.findByRole('heading', { name: /мои вещи/i })
		fireEvent.click(screen.getByRole('button', { name: /добавить/i }))
		expect(screen.getByRole('heading', { name: 'Новая вещь' })).toBeInTheDocument()
		fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
		fireEvent.click(screen.getByRole('button', { name: '▣ Коробки' }))
		fireEvent.click(screen.getByRole('button', { name: /добавить/i }))
		expect(screen.getByRole('heading', { name: 'Новая коробка' })).toBeInTheDocument()
		fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
		fireEvent.click(screen.getByRole('button', { name: '⌖ Места' }))
		fireEvent.click(screen.getByRole('button', { name: /добавить/i }))
		expect(screen.getByRole('heading', { name: 'Новая локация' })).toBeInTheDocument()
		fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
		fireEvent.click(screen.getByRole('button', { name: '⌕ Вещи' }))
		expect(screen.getByText('Misha · @misha')).toBeInTheDocument()
		expect(screen.getByRole('button', { name: 'Выйти' }).previousElementSibling).toHaveTextContent(
			'Misha · @misha'
		)
		expect((await screen.findByAltText('')).parentElement).toHaveClass('preview-image')
		expect(screen.getByText('Архив', { selector: '.preview-photo-count' })).toBeInTheDocument()
		fireEvent.change(screen.getByPlaceholderText('Найти вещь…'), { target: { value: 'паспорт' } })
		await waitFor(() =>
			expect(screen.getByAltText('')).toHaveAttribute('src', '/api/v1/media/media-1')
		)
		fireEvent.change(screen.getByPlaceholderText('Найти вещь…'), { target: { value: '' } })
		fireEvent.click(screen.getByRole('button', { name: 'Фильтр по коробке: Все коробки' }))
		fireEvent.change(screen.getByRole('searchbox', { name: 'Поиск коробки' }), {
			target: { value: 'инстру' }
		})
		fireEvent.click(screen.getByRole('option', { name: 'Инструменты' }))
		expect(screen.queryByRole('button', { name: /паспорт/i })).not.toBeInTheDocument()
		expect(screen.getByRole('button', { name: /ключи без описания/i })).toBeInTheDocument()
		fireEvent.click(screen.getByRole('button', { name: 'Фильтр по коробке: Инструменты' }))
		fireEvent.click(screen.getByRole('option', { name: 'Без коробки' }))
		expect(screen.getByRole('button', { name: /запасной ключ без описания/i })).toBeInTheDocument()
		fireEvent.click(screen.getByRole('button', { name: 'Фильтр по коробке: Без коробки' }))
		fireEvent.click(screen.getByRole('option', { name: 'Все коробки' }))
		fireEvent.click(await screen.findByRole('button', { name: /паспорт/i }))
		const detailPhoto = await screen.findByRole('img', { name: 'Фото 1 вещи: Паспорт' })
		expect(detailPhoto).toHaveAttribute('src', '/api/v1/media/media-1')
		expect(detailPhoto.parentElement?.parentElement).toHaveClass('item-detail-gallery')
		fireEvent.click(screen.getByRole('tab', { name: /настройки/i }))
		const itemBox = screen.getByRole('combobox', { name: 'Коробка' })
		expect(itemBox).toHaveValue('box-1')
		expect(screen.getByRole('img', { name: 'Фотография 2' })).toHaveAttribute(
			'src',
			'/api/v1/media/media-2'
		)
		fireEvent.click(screen.getByRole('button', { name: 'Переместить фото 2 раньше' }))
		await waitFor(() =>
			expect(calls).toContainEqual(
				expect.objectContaining({
					method: 'PATCH',
					url: expect.stringContaining('/items/item-1/media'),
					body: { mediaIds: ['media-2', 'media-1'] }
				})
			)
		)
		fireEvent.click(screen.getByRole('button', { name: 'Удалить фото 2' }))
		expect(
			screen.getByRole('alertdialog', { name: 'Подтверждение удаления фото' })
		).toBeInTheDocument()
		fireEvent.click(screen.getByRole('button', { name: 'Удалить фото' }))
		fireEvent.change(itemBox, { target: { value: '' } })
		fireEvent.click(screen.getByRole('button', { name: 'Сохранить настройки' }))
		await waitFor(() =>
			expect(calls).toContainEqual(
				expect.objectContaining({
					method: 'PATCH',
					url: expect.stringContaining('/items/item-1'),
					body: expect.objectContaining({ boxId: '' })
				})
			)
		)
		fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
		fireEvent.click(screen.getByRole('button', { name: '▣ Коробки' }))
		await screen.findByRole('button', { name: /архив/i })
		fireEvent.click(screen.getByRole('button', { name: /архив/i }))

		expect((await screen.findAllByRole('heading', { name: 'Архив' })).length).toBeGreaterThan(1)
		expect(screen.getByText('Паспорт')).toBeInTheDocument()
		fireEvent.keyDown(document, { key: 'Escape' })
		expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
		fireEvent.click(screen.getByRole('button', { name: /архив/i }))
		expect(screen.getByRole('img', { name: 'Паспорт' })).toHaveAttribute(
			'src',
			'/api/v1/media/media-1'
		)
		fireEvent.click(screen.getByRole('button', { name: 'Развернуть карточку' }))
		expect(screen.getByRole('dialog')).toHaveClass('expanded')
		expect(screen.getByRole('button', { name: 'Сузить карточку' })).toBeInTheDocument()
		fireEvent.click(screen.getByRole('tab', { name: /история/i }))
		expect(await screen.findByText('created')).toBeInTheDocument()
		fireEvent.click(screen.getByRole('tab', { name: /настройки/i }))
		fireEvent.click(screen.getByRole('button', { name: /сохранить место/i }))
		await waitFor(() =>
			expect(calls).toContainEqual(
				expect.objectContaining({
					method: 'PATCH',
					url: expect.stringContaining('/boxes/box-1/move')
				})
			)
		)
	})
})

import { FormEvent, useEffect, useId, useState } from 'react'
import {
	ApiError,
	Box,
	Item,
	Location,
	Media,
	Space,
	useBoxesQuery,
	useCreateBoxMutation,
	useCreateItemMutation,
	useCreateLocationMutation,
	useCreateSpaceMutation,
	useItemsQuery,
	useLocationsQuery,
	useLoginMutation,
	useLogoutMutation,
	useMeQuery,
	useMoveBoxMutation,
	useRegisterMutation,
	useSearchQuery,
	useSpacesQuery,
	useTimelineQuery,
	useUpdateBoxMutation,
	useUpdateItemMutation,
	useDeleteItemMutation,
	useDeleteBoxMutation,
	useDeleteLocationMutation,
	useAddItemMediaMutation,
	useReorderItemMediaMutation,
	useRemoveItemMediaMutation,
	useUpdateLocationMutation,
	useUploadMediaMutation
} from './api/api'

type Entity =
	| { kind: 'item'; value: Item }
	| { kind: 'box'; value: Box }
	| { kind: 'location'; value: Location }
function message(error: unknown) {
	return (
		(error as { data?: ApiError })?.data?.message ??
		'Не удалось выполнить запрос. Попробуйте ещё раз.'
	)
}
function stateMeta(state: string) {
	if (state === 'deleted') return { icon: '×', label: 'Удалено' }
	if (state === 'temporarily_removed') return { icon: '⌛', label: 'Временно убрано' }
	if (state === 'archived') return { icon: '▧', label: 'В архиве' }
	return { icon: '●', label: 'Активно' }
}

function AuthPage() {
	const [registering, setRegistering] = useState(false)
	const [login, loginState] = useLoginMutation()
	const [register, registerState] = useRegisterMutation()
	const pending = loginState.isLoading || registerState.isLoading
	async function submit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault()
		const form = new FormData(event.currentTarget)
		const username = String(form.get('username') ?? '')
		const password = String(form.get('password') ?? '')
		if (registering)
			await register({
				username,
				password,
				displayName: String(form.get('displayName') ?? '')
			}).unwrap()
		else await login({ username, password }).unwrap()
	}
	return (
		<main className="auth-page">
			<section className="auth-card" aria-labelledby="auth-title">
				<strong className="brand">storere</strong>
				<p className="eyebrow">ЛИЧНЫЙ ИНВЕНТАРЬ</p>
				<h1 id="auth-title">{registering ? 'Создать аккаунт' : 'Войти'}</h1>
				<p className="muted">Храните вещи, коробки и места в одном понятном каталоге.</p>
				<form onSubmit={submit}>
					{registering && (
						<label>
							Отображаемое имя
							<input name="displayName" required autoComplete="name" />
						</label>
					)}
					<label>
						Имя пользователя
						<input name="username" required minLength={3} autoComplete="username" />
					</label>
					<label>
						Пароль
						<input
							name="password"
							required
							minLength={12}
							type="password"
							autoComplete={registering ? 'new-password' : 'current-password'}
						/>
					</label>
					{(loginState.error || registerState.error) && (
						<p role="alert" className="form-error">
							{message(loginState.error || registerState.error)}
						</p>
					)}
					<button className="primary wide" disabled={pending}>
						{pending ? 'Подождите…' : registering ? 'Зарегистрироваться' : 'Войти'}
					</button>
				</form>
				<button className="link-button" onClick={() => setRegistering(!registering)}>
					{registering ? 'У меня уже есть аккаунт' : 'Создать аккаунт'}
				</button>
			</section>
		</main>
	)
}
function SpaceSetup() {
	const [createSpace, state] = useCreateSpaceMutation()
	async function submit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault()
		const form = new FormData(event.currentTarget)
		await createSpace({
			name: String(form.get('name')),
			description: String(form.get('description') || ''),
			address: String(form.get('address') || '')
		}).unwrap()
	}
	return (
		<section className="empty setup">
			<p className="eyebrow">ПЕРВОЕ ПРОСТРАНСТВО</p>
			<h1>Где находятся ваши вещи?</h1>
			<p>Например, «Квартира на Лесной» или «Дача».</p>
			<form onSubmit={submit}>
				<label>
					Название
					<input name="name" required autoFocus />
				</label>
				<label>
					Описание
					<textarea name="description" />
				</label>
				<label>
					Адрес <span className="muted">(необязательно)</span>
					<input name="address" />
				</label>
				{state.error && (
					<p role="alert" className="form-error">
						{message(state.error)}
					</p>
				)}
				<button className="primary" disabled={state.isLoading}>
					Создать пространство
				</button>
			</form>
		</section>
	)
}
function PhotoViewer({ src, alt, onClose }: { src: string; alt: string; onClose: () => void }) {
	return (
		<div
			className="photo-viewer"
			role="dialog"
			aria-modal="true"
			aria-label={alt}
			onClick={onClose}>
			<button className="photo-close" aria-label="Закрыть фото" onClick={onClose}>
				×
			</button>
			<img src={src} alt={alt} onClick={(event) => event.stopPropagation()} />
		</div>
	)
}
function CreateSpaceDialog({ onClose }: { onClose: () => void }) {
	const [createSpace, state] = useCreateSpaceMutation()
	async function submit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault()
		const form = new FormData(event.currentTarget)
		await createSpace({
			name: String(form.get('name')),
			description: String(form.get('description') || ''),
			address: String(form.get('address') || '')
		}).unwrap()
		onClose()
	}
	return (
		<div className="backdrop" onMouseDown={onClose}>
			<form
				className="dialog"
				role="dialog"
				aria-modal="true"
				aria-labelledby="space-title"
				onMouseDown={(event) => event.stopPropagation()}
				onSubmit={submit}>
				<button className="close" onClick={onClose} aria-label="Закрыть">
					×
				</button>
				<p className="eyebrow">ПРОСТРАНСТВО</p>
				<h2 id="space-title">Добавить пространство</h2>
				<label>
					Название
					<input name="name" required autoFocus />
				</label>
				<label>
					Описание
					<textarea name="description" />
				</label>
				<label>
					Адрес
					<input name="address" />
				</label>
				{state.error && (
					<p role="alert" className="form-error">
						{message(state.error)}
					</p>
				)}
				<button className="primary" disabled={state.isLoading}>
					{state.isLoading ? 'Создаём…' : 'Добавить'}
				</button>
			</form>
		</div>
	)
}

function AddDialog({
	space,
	initialKind,
	initialBoxId,
	onClose
}: {
	space: Space
	initialKind: 'item' | 'box' | 'location'
	initialBoxId?: string
	onClose: () => void
}) {
	const [kind, setKind] = useState<'menu' | 'item' | 'box' | 'location'>(initialKind)
	const [media, setMedia] = useState<Media | null>(null)
	const [uploadMedia, uploadState] = useUploadMediaMutation()
	const [createItem, itemState] = useCreateItemMutation()
	const [createBox, boxState] = useCreateBoxMutation()
	const [createLocation, locationState] = useCreateLocationMutation()
	const { data: locations = [] } = useLocationsQuery(space.id)
	const { data: boxes = [] } = useBoxesQuery(space.id)
	const state = kind === 'item' ? itemState : kind === 'box' ? boxState : locationState
	useEffect(() => {
		function onKeyDown(event: KeyboardEvent) {
			if (event.defaultPrevented || event.key !== 'Escape') return
			event.preventDefault()
			onClose()
		}
		window.addEventListener('keydown', onKeyDown)
		return () => window.removeEventListener('keydown', onKeyDown)
	}, [onClose])
	async function upload(file?: File) {
		if (file) setMedia(await uploadMedia({ spaceId: space.id, file }).unwrap())
	}
	async function submit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault()
		const form = new FormData(event.currentTarget)
		if (kind === 'location')
			await createLocation({
				spaceId: space.id,
				name: String(form.get('name')),
				code: String(form.get('code') || ''),
				description: String(form.get('description') || '')
			}).unwrap()
		if (kind === 'box')
			await createBox({
				spaceId: space.id,
				name: String(form.get('name')),
				description: String(form.get('description') || ''),
				locationId: String(form.get('locationId') || '')
			}).unwrap()
		if (kind === 'item' && media)
			await createItem({
				spaceId: space.id,
				name: String(form.get('name')),
				description: String(form.get('description') || ''),
				boxId: String(form.get('boxId') || ''),
				mediaIds: [media.id]
			}).unwrap()
		onClose()
	}
	return (
		<div className="backdrop" onMouseDown={onClose}>
			<section
				className="dialog"
				role="dialog"
				aria-modal="true"
				aria-labelledby="add-title"
				onMouseDown={(event) => event.stopPropagation()}>
				<button className="close" onClick={onClose} aria-label="Закрыть">
					×
				</button>
				{kind === 'menu' ? (
					<>
						<p className="eyebrow">БЫСТРОЕ ДОБАВЛЕНИЕ</p>
						<h2 id="add-title">Что добавить?</h2>
						<button className="choice" onClick={() => setKind('item')}>
							▧{' '}
							<span>
								<b>Вещь</b>
								<small>Фото обязательно</small>
							</span>
						</button>
						<button className="choice" onClick={() => setKind('box')}>
							□{' '}
							<span>
								<b>Коробку</b>
								<small>Контейнер для вещей</small>
							</span>
						</button>
						<button className="choice" onClick={() => setKind('location')}>
							⌖{' '}
							<span>
								<b>Место</b>
								<small>Балкон, полка, комната</small>
							</span>
						</button>
					</>
				) : (
					<form onSubmit={submit}>
						<p className="eyebrow">ДОБАВЛЕНИЕ</p>
						<h2 id="add-title">
							Новая {kind === 'item' ? 'вещь' : kind === 'box' ? 'коробка' : 'локация'}
						</h2>
						<label>
							Название
							<input name="name" required autoFocus />
						</label>
						<label>
							Описание
							<textarea name="description" />
						</label>
						{kind === 'location' && (
							<label>
								Код
								<input name="code" placeholder="kitchen" />
							</label>
						)}
						{kind === 'box' && (
							<label>
								Место
								<select name="locationId">
									<option value="">Не указано</option>
									{locations.map((location) => (
										<option key={location.id} value={location.id}>
											{location.name}
										</option>
									))}
								</select>
							</label>
						)}
						{kind === 'item' && (
							<>
								<label>
									Коробка
									<select name="boxId" defaultValue={initialBoxId || ''}>
										<option value="">Без коробки</option>
										{boxes.map((box) => (
											<option key={box.id} value={box.id}>
												{box.name}
											</option>
										))}
									</select>
								</label>
								<label>
									Фотография
									<input
										aria-label="Фотография"
										type="file"
										accept="image/jpeg,image/png,image/gif,image/webp,image/heic,.heic"
										required={!media}
										onChange={(event) => upload(event.currentTarget.files?.[0])}
									/>
								</label>
								{media && <img className="upload-preview" src={media.url} alt="Загруженное фото" />}
								{uploadState.error && (
									<p role="alert" className="form-error">
										{message(uploadState.error)}
									</p>
								)}
							</>
						)}
						{state.error && (
							<p role="alert" className="form-error">
								{message(state.error)}
							</p>
						)}
						<button className="primary" disabled={state.isLoading || (kind === 'item' && !media)}>
							Создать
						</button>
					</form>
				)}
			</section>
		</div>
	)
}

function EntityDialog({
	entity,
	boxes,
	locations,
	items,
	spaceId,
	fullPage = false,
	disableEscape = false,
	onOpenItem,
	onOpenFull,
	onAddItem,
	onClose
}: {
	entity: Entity
	boxes: Box[]
	locations: Location[]
	items: Item[]
	spaceId: string
	fullPage?: boolean
	disableEscape?: boolean
	onOpenItem?: (item: Item) => void
	onOpenFull?: () => void
	onAddItem?: () => void
	onClose: () => void
}) {
	const titleID = useId()
	const [tab, setTab] = useState<'overview' | 'history' | 'settings'>('overview')
	const [expanded, setExpanded] = useState(false)
	const {
		data: history = [],
		isLoading: historyLoading,
		error: historyError
	} = useTimelineQuery({ entity: entity.kind, id: entity.value.id }, { skip: tab !== 'history' })
	const [updateItem, itemState] = useUpdateItemMutation()
	const [updateBox, boxState] = useUpdateBoxMutation()
	const [updateLocation, locationState] = useUpdateLocationMutation()
	const [moveBox, moveState] = useMoveBoxMutation()
	const [deleteItem, deleteItemState] = useDeleteItemMutation()
	const [deleteBox, deleteBoxState] = useDeleteBoxMutation()
	const [deleteLocation, deleteLocationState] = useDeleteLocationMutation()
	const [confirmDelete, setConfirmDelete] = useState(false)
	const [uploadMedia, uploadState] = useUploadMediaMutation()
	const [addItemMedia, addMediaState] = useAddItemMediaMutation()
	const [reorderItemMedia, reorderMediaState] = useReorderItemMediaMutation()
	const [removeItemMedia, removeMediaState] = useRemoveItemMediaMutation()
	const [gallery, setGallery] = useState<Media[]>(
		entity.kind === 'item' ? (entity.value.media ?? []) : []
	)
	useEffect(() => {
		setGallery(entity.kind === 'item' ? (entity.value.media ?? []) : [])
	}, [entity.kind, entity.value.id])
	const [mediaToDelete, setMediaToDelete] = useState<Media | null>(null)
	useEffect(() => {
		function onKeyDown(event: KeyboardEvent) {
			if (disableEscape || event.defaultPrevented || event.key !== 'Escape') return
			if (mediaToDelete) setMediaToDelete(null)
			else onClose()
		}
		window.addEventListener('keydown', onKeyDown)
		return () => window.removeEventListener('keydown', onKeyDown)
	}, [disableEscape, mediaToDelete, onClose])
	const current = entity.value
	const error = itemState.error || boxState.error || locationState.error || moveState.error
	const saving =
		itemState.isLoading || boxState.isLoading || locationState.isLoading || moveState.isLoading
	const kindLabel = entity.kind === 'item' ? 'Вещь' : entity.kind === 'box' ? 'Коробка' : 'Локация'
	async function save(event: FormEvent<HTMLFormElement>) {
		event.preventDefault()
		const form = new FormData(event.currentTarget)
		const body = {
			name: String(form.get('name')),
			description: String(form.get('description') || ''),
			state: String(form.get('state'))
		}
		if (entity.kind === 'item') {
			await updateItem({ id: current.id, ...body, boxId: String(form.get('boxId') || '') }).unwrap()
		}
		if (entity.kind === 'location') await updateLocation({ id: current.id, ...body }).unwrap()
		if (entity.kind === 'box') {
			await updateBox({ id: current.id, ...body }).unwrap()
			await moveBox({ id: current.id, locationId: String(form.get('locationId') || '') }).unwrap()
		}
	}
	const boxItems = entity.kind === 'box' ? items.filter((item) => item.boxId === current.id) : []
	const itemMedia = entity.kind === 'item' ? gallery : []
	async function uploadPhotos(files: FileList | null) {
		if (!files || entity.kind !== 'item') return
		for (const file of Array.from(files)) {
			const media = await uploadMedia({ spaceId, file }).unwrap()
			await addItemMedia({ id: current.id, mediaId: media.id }).unwrap()
			setGallery((photos) => [...photos, media])
		}
	}
	async function movePhoto(index: number, direction: -1 | 1) {
		const next = [...gallery]
		const target = index + direction
		if (!next[target]) return
		;[next[index], next[target]] = [next[target], next[index]]
		await reorderItemMedia({ id: current.id, mediaIds: next.map((media) => media.id) }).unwrap()
		setGallery(next)
	}
	async function confirmPhotoRemoval() {
		if (!mediaToDelete) return
		await removeItemMedia({ id: current.id, mediaId: mediaToDelete.id }).unwrap()
		setGallery((photos) => photos.filter((media) => media.id !== mediaToDelete.id))
		setMediaToDelete(null)
	}
	return (
		<div className={fullPage ? 'backdrop box-full-backdrop' : 'backdrop'} onMouseDown={onClose}>
			<section
				className={`dialog entity-dialog${expanded ? ' expanded' : ''}${fullPage ? ' box-full-page' : ''}`}
				role="dialog"
				aria-modal="true"
				aria-labelledby={titleID}
				onMouseDown={(event) => event.stopPropagation()}>
				{fullPage && (
					<button className="back-to-boxes" onClick={onClose} aria-label="Вернуться к коробкам">
						← Коробки
					</button>
				)}
				<button className="close" onClick={onClose} aria-label="Закрыть">
					×
				</button>
				{entity.kind === 'box' && (
					<button className="add-box-item" onClick={onAddItem} aria-label="Добавить вещь в коробку">
						＋ Вещь
					</button>
				)}
				<button
					className="resize-dialog"
					onClick={() => setExpanded(!expanded)}
					aria-label={expanded ? 'Сузить карточку' : 'Развернуть карточку'}>
					{expanded ? 'Сузить' : 'Развернуть'}
				</button>
				{entity.kind === 'box' && !fullPage && (
					<button
						className="open-box-full"
						onClick={onOpenFull}
						aria-label="Открыть коробку полностью">
						Открыть полностью
					</button>
				)}
				<p className="eyebrow">{kindLabel.toUpperCase()}</p>
				<h2 id={titleID}>{current.name}</h2>
				<p className="muted">{current.description || 'Без описания'}</p>
				<div className="tabs" role="tablist" aria-label={`${kindLabel} разделы`}>
					<button role="tab" aria-selected={tab === 'overview'} onClick={() => setTab('overview')}>
						Содержимое
					</button>
					<button role="tab" aria-selected={tab === 'history'} onClick={() => setTab('history')}>
						История
					</button>
					<button role="tab" aria-selected={tab === 'settings'} onClick={() => setTab('settings')}>
						Настройки
					</button>
				</div>
				{tab === 'overview' && (
					<section className="detail-panel">
						{entity.kind === 'box' ? (
							boxItems.length ? (
								<ul className="contents-list">
									{boxItems.map((item) => (
										<li key={item.id}>
											<button className="content-item" onClick={() => onOpenItem?.(item)}>
												{item.media?.[0] ? (
													<img src={item.media[0].url} alt={item.name} />
												) : (
													<span className="content-item-placeholder">▧</span>
												)}
												<span>
													<b>{item.name}</b>
													<small>{item.description || 'Без описания'}</small>
												</span>
											</button>
										</li>
									))}
								</ul>
							) : (
								<p className="muted">В этой коробке пока нет вещей.</p>
							)
						) : entity.kind === 'item' ? (
							itemMedia.length ? (
								<div className="item-detail-gallery">
									{itemMedia.map((media, index) => (
										<div className="item-detail-photo" key={media.id}>
											<img
												src={media.url}
												alt={
													itemMedia.length === 1
														? `Фото вещи: ${current.name}`
														: `Фото ${index + 1} вещи: ${current.name}`
												}
											/>
										</div>
									))}
								</div>
							) : (
								<p className="muted">Фото: {(current as Item).photoCount}.</p>
							)
						) : (
							<p className="muted">
								Откройте настройки, чтобы изменить название, описание или флаги.
							</p>
						)}
					</section>
				)}
				{tab === 'history' && (
					<section className="detail-panel">
						{historyLoading ? (
							<p role="status">Загружаем историю…</p>
						) : historyError ? (
							<p role="alert" className="form-error">
								{message(historyError)}
							</p>
						) : history.length ? (
							<ul className="history-list">
								{history.map((event, index) => (
									<li key={`${event.occurredAt}-${index}`}>
										<b>{event.action}</b>
										<time>{new Date(event.occurredAt).toLocaleString('ru-RU')}</time>
									</li>
								))}
							</ul>
						) : (
							<p className="muted">История пока пуста.</p>
						)}
					</section>
				)}
				{tab === 'settings' && (
					<form className="detail-panel" onSubmit={save}>
						<label>
							Название
							<input name="name" required defaultValue={current.name} />
						</label>
						<label>
							Описание
							<textarea name="description" defaultValue={current.description || ''} />
						</label>
						<label>
							Флаг
							<select name="state" defaultValue={current.state}>
								<option value="active">Активно</option>
								<option value="temporarily_removed">Временно убрано</option>
								<option value="archived">В архиве</option>
							</select>
						</label>
						{entity.kind === 'item' && (
							<>
								<label>
									Коробка
									<select name="boxId" defaultValue={(current as Item).boxId || ''}>
										<option value="">Без коробки</option>
										{boxes.map((box) => (
											<option key={box.id} value={box.id}>
												{box.name}
											</option>
										))}
									</select>
								</label>
								<section className="photo-manager" aria-label="Фотографии вещи">
									<div className="photo-manager-heading">
										<div>
											<b>Фотографии</b>
											<p>Первая фотография используется на карточке вещи.</p>
										</div>
										<label className="photo-upload-button">
											Добавить фото
											<input
												aria-label="Добавить фотографии"
												type="file"
												multiple
												accept="image/jpeg,image/png,image/gif,image/webp,image/heic,image/heif,.heic,.heif"
												disabled={uploadState.isLoading || addMediaState.isLoading}
												onChange={(event) => void uploadPhotos(event.currentTarget.files)}
											/>
										</label>
									</div>
									{itemMedia.length ? (
										<div className="photo-manager-grid">
											{itemMedia.map((media, index) => (
												<article className="photo-manager-card" key={media.id}>
													<img src={media.url} alt={`Фотография ${index + 1}`} />
													<span className="photo-position">
														{index === 0 ? 'Обложка' : `Фото ${index + 1}`}
													</span>
													<div className="photo-manager-actions">
														<button
															type="button"
															aria-label={`Переместить фото ${index + 1} раньше`}
															disabled={index === 0 || reorderMediaState.isLoading}
															onClick={() => void movePhoto(index, -1)}>
															←
														</button>
														<button
															type="button"
															aria-label={`Переместить фото ${index + 1} позже`}
															disabled={
																index === itemMedia.length - 1 || reorderMediaState.isLoading
															}
															onClick={() => void movePhoto(index, 1)}>
															→
														</button>
														<button
															type="button"
															className="photo-remove"
															aria-label={`Удалить фото ${index + 1}`}
															onClick={() => setMediaToDelete(media)}>
															Удалить
														</button>
													</div>
												</article>
											))}
										</div>
									) : (
										<p className="photo-manager-empty">
											Добавьте хотя бы одно фото, чтобы быстрее находить вещь.
										</p>
									)}
									{mediaToDelete && (
										<div
											className="photo-delete-confirm"
											role="alertdialog"
											aria-label="Подтверждение удаления фото">
											<b>Удалить эту фотографию?</b>
											<p>Она исчезнет только из этой карточки. Следующее фото станет обложкой.</p>
											<div>
												<button type="button" onClick={() => setMediaToDelete(null)}>
													Отмена
												</button>
												<button
													type="button"
													className="delete-button"
													disabled={removeMediaState.isLoading}
													onClick={() => void confirmPhotoRemoval()}>
													Удалить фото
												</button>
											</div>
										</div>
									)}
								</section>
							</>
						)}
						{entity.kind === 'box' && (
							<label>
								Текущее место
								<select name="locationId" defaultValue={(current as Box).currentLocationId || ''}>
									<option value="">Не указано</option>
									{locations.map((location) => (
										<option key={location.id} value={location.id}>
											{location.name}
										</option>
									))}
								</select>
							</label>
						)}
						{error && (
							<p role="alert" className="form-error">
								{message(error)}
							</p>
						)}
						<button className="primary" disabled={saving}>
							{entity.kind === 'box' ? 'Сохранить место' : 'Сохранить настройки'}
						</button>
						<button type="button" className="delete-button" onClick={() => setConfirmDelete(true)}>
							Удалить {kindLabel.toLowerCase()}
						</button>
						{confirmDelete && (
							<div className="delete-confirm">
								<p>
									Удалить «{current.name}»? Карточка будет деактивирована и доступна через фильтр
									«Удалённые».
								</p>
								<div>
									<button type="button" onClick={() => setConfirmDelete(false)}>
										Отмена
									</button>
									<button
										type="button"
										className="delete-button"
										disabled={
											deleteItemState.isLoading ||
											deleteBoxState.isLoading ||
											deleteLocationState.isLoading
										}
										onClick={async () => {
											if (entity.kind === 'item') await deleteItem(current.id).unwrap()
											if (entity.kind === 'box') await deleteBox(current.id).unwrap()
											if (entity.kind === 'location') await deleteLocation(current.id).unwrap()
											onClose()
										}}>
										Подтвердить удаление
									</button>
								</div>
							</div>
						)}
					</form>
				)}
			</section>
		</div>
	)
}

function BoxFilter({
	boxes,
	value,
	onChange
}: {
	boxes: Box[]
	value: string
	onChange: (value: string) => void
}) {
	const [open, setOpen] = useState(false)
	const [query, setQuery] = useState('')
	const selected = boxes.find((box) => box.id === value)
	const label = value === 'none' ? 'Без коробки' : selected?.name || 'Все коробки'
	const options = boxes.filter((box) =>
		box.name.toLocaleLowerCase().includes(query.toLocaleLowerCase())
	)
	function select(next: string) {
		onChange(next)
		setQuery('')
		setOpen(false)
	}
	return (
		<div className="box-filter">
			<span>Коробка</span>
			<button
				type="button"
				className="box-filter-trigger"
				aria-label={`Фильтр по коробке: ${label}`}
				aria-expanded={open}
				onClick={() => setOpen(!open)}>
				{label}
				<span aria-hidden="true">⌄</span>
			</button>
			{open && (
				<div className="box-filter-popover" role="listbox" aria-label="Коробки">
					<input
						type="search"
						role="searchbox"
						aria-label="Поиск коробки"
						value={query}
						onChange={(event) => setQuery(event.target.value)}
						onKeyDown={(event) => event.key === 'Escape' && setOpen(false)}
						autoFocus
					/>
					<button role="option" aria-selected={value === 'all'} onClick={() => select('all')}>
						Все коробки
					</button>
					<button role="option" aria-selected={value === 'none'} onClick={() => select('none')}>
						Без коробки
					</button>
					{options.map((box) => (
						<button
							role="option"
							aria-selected={value === box.id}
							key={box.id}
							onClick={() => select(box.id)}>
							{box.name}
						</button>
					))}
					{options.length === 0 && <p>Коробки не найдены.</p>}
				</div>
			)}
		</div>
	)
}

function Inventory({ user }: { user: { username: string; displayName: string } }) {
	const { data: spaces = [], isLoading: spacesLoading, error: spacesError } = useSpacesQuery()
	const [selectedSpaceId, setSelectedSpaceId] = useState('')
	const [query, setQuery] = useState('')
	const [dark, setDark] = useState(true)
	const [adding, setAdding] = useState<{
		kind: 'item' | 'box' | 'location'
		boxId?: string
	} | null>(null)
	const [spaceMenu, setSpaceMenu] = useState(false)
	const [creatingSpace, setCreatingSpace] = useState(false)
	const [view, setView] = useState<'items' | 'boxes' | 'locations'>('items')
	const [statusFilter, setStatusFilter] = useState('all')
	const [boxFilter, setBoxFilter] = useState('all')
	const [photoViewer, setPhotoViewer] = useState<{ src: string; alt: string } | null>(null)
	const [selected, setSelected] = useState<Entity | null>(null)
	const [boxDialog, setBoxDialog] = useState<Box | null>(null)
	const [fullBoxId, setFullBoxId] = useState(
		() => window.location.pathname.match(/^\/boxes\/([^/]+)$/)?.[1] ?? ''
	)
	const [logout] = useLogoutMutation()
	const activeSpace = spaces.find((space) => space.id === selectedSpaceId) ?? spaces[0]
	const searchQuery = query.trim()
	const {
		data: itemList = [],
		isLoading: itemsLoading,
		error: itemsError
	} = useItemsQuery(activeSpace?.id ?? '', {
		skip: !activeSpace || (view === 'items' && Boolean(searchQuery))
	})
	const { data: searchResults = [] } = useSearchQuery(
		{ spaceId: activeSpace?.id ?? '', q: searchQuery },
		{ skip: !activeSpace || !searchQuery }
	)
	const { data: boxList = [] } = useBoxesQuery(activeSpace?.id ?? '', { skip: !activeSpace })
	const { data: locationList = [] } = useLocationsQuery(activeSpace?.id ?? '', {
		skip: !activeSpace
	})
	const items = view === 'items' && searchQuery ? searchResults : itemList
	const fullBox = boxList.find((box) => box.id === fullBoxId)
	useEffect(() => {
		const onPopState = () =>
			setFullBoxId(window.location.pathname.match(/^\/boxes\/([^/]+)$/)?.[1] ?? '')
		window.addEventListener('popstate', onPopState)
		return () => window.removeEventListener('popstate', onPopState)
	}, [])
	function openBoxFull(box: Box) {
		window.history.pushState({}, '', `/boxes/${box.id}`)
		setFullBoxId(box.id)
		setSelected(null)
		setBoxDialog(null)
	}
	function closeBoxFull() {
		window.history.replaceState({}, '', '/')
		setFullBoxId('')
	}
	const unfilteredRecords = view === 'items' ? items : view === 'boxes' ? boxList : locationList
	const availableTags = [
		...new Map(items.flatMap((item) => item.tags ?? []).map((tag) => [tag.id, tag])).values()
	]
	const [tagFilter, setTagFilter] = useState('all')
	const records = unfilteredRecords.filter((record) => {
		if (!(statusFilter === 'all' ? record.state !== 'deleted' : record.state === statusFilter))
			return false
		if (view !== 'items' || boxFilter === 'all') return true
		const item = record as Item
		return boxFilter === 'none' ? !item.boxId : item.boxId === boxFilter
	})
	if (spacesLoading)
		return (
			<main className="auth-page">
				<p role="status">Загружаем пространства…</p>
			</main>
		)
	if (spacesError)
		return (
			<main className="auth-page">
				<p role="alert">{message(spacesError)}</p>
			</main>
		)
	if (!activeSpace)
		return (
			<main className={dark ? 'app dark' : 'app'}>
				<aside>
					<strong>storere</strong>
				</aside>
				<SpaceSetup />
			</main>
		)
	const title = view === 'items' ? 'Мои вещи' : view === 'boxes' ? 'Коробки' : 'Места'
	return (
		<main className={dark ? 'app dark' : 'app'}>
			<aside>
				<strong>storere</strong>
				<nav>
					<button className={view === 'items' ? 'active' : ''} onClick={() => setView('items')}>
						<span className="nav-icon">⌕</span>
						<span>Вещи</span>
					</button>
					<button className={view === 'boxes' ? 'active' : ''} onClick={() => setView('boxes')}>
						<span className="nav-icon">▣</span>
						<span>Коробки</span>
					</button>
					<button
						className={view === 'locations' ? 'active' : ''}
						onClick={() => setView('locations')}>
						<span className="nav-icon">⌖</span>
						<span>Места</span>
					</button>
				</nav>
				<div className="account">
					<div className="space-menu">
						<button
							className="space-trigger"
							aria-expanded={spaceMenu}
							onClick={() => setSpaceMenu(!spaceMenu)}>
							<span>Пространство</span>
							<b>{activeSpace.name}</b>
							<span aria-hidden="true">⌃</span>
						</button>
						{spaceMenu && (
							<div className="space-options" role="menu">
								{spaces.map((space) => (
									<button
										role="menuitem"
										className={space.id === activeSpace.id ? 'active' : ''}
										key={space.id}
										onClick={() => {
											setSelectedSpaceId(space.id)
											setSpaceMenu(false)
										}}>
										{space.name}
									</button>
								))}
								<button
									role="menuitem"
									className="add-space"
									onClick={() => {
										setSpaceMenu(false)
										setCreatingSpace(true)
									}}>
									＋ Добавить пространство
								</button>
							</div>
						)}
					</div>
					<span>
						{user.displayName} · @{user.username}
					</span>
					<button className="link-button" onClick={() => logout()}>
						Выйти
					</button>
				</div>
			</aside>
			<section className="content">
				<header>
					<div>
						<p className="eyebrow">{activeSpace.name.toUpperCase()}</p>
						<h1>{title}</h1>
					</div>
					<div className="actions">
						<button aria-label="Переключить тему" onClick={() => setDark(!dark)}>
							{dark ? '☀' : '☾'}
						</button>
						<button
							className="primary"
							onClick={() =>
								setAdding({
									kind: view === 'items' ? 'item' : view === 'boxes' ? 'box' : 'location'
								})
							}>
							＋ Добавить
						</button>
					</div>
				</header>
				{view === 'items' && (
					<label className="search">
						<span>⌕</span>
						<input
							value={query}
							onChange={(event) => setQuery(event.target.value)}
							placeholder="Найти вещь…"
							autoFocus
						/>
					</label>
				)}
				<div className="filters" aria-label="Фильтры">
					<label>
						Статус
						<select
							aria-label="Фильтр по статусу"
							value={statusFilter}
							onChange={(event) => setStatusFilter(event.target.value)}>
							<option value="all">Все</option>
							<option value="active">Активно</option>
							<option value="temporarily_removed">Временно убрано</option>
							<option value="archived">Архив</option>
							<option value="deleted">Удалённые</option>
						</select>
					</label>
					{view === 'items' && (
						<BoxFilter boxes={boxList} value={boxFilter} onChange={setBoxFilter} />
					)}
					{view === 'items' && (
						<label>
							Флаг
							<select
								aria-label="Фильтр по флагу"
								value={tagFilter}
								onChange={(event) => setTagFilter(event.target.value)}>
								<option value="all">Все</option>
								{availableTags.map((tag) => (
									<option key={tag.id} value={tag.id}>
										{tag.name}
									</option>
								))}
							</select>
						</label>
					)}
				</div>
				{itemsError && (
					<p role="alert" className="form-error">
						{message(itemsError)}
					</p>
				)}
				<div className="summary">
					<span>
						{records.length} {view === 'items' ? 'вещи' : view === 'boxes' ? 'коробки' : 'места'}
					</span>
				</div>
				{itemsLoading && view === 'items' ? (
					<p role="status">Загружаем вещи…</p>
				) : (
					<div className="grid">
						{view === 'items'
							? (records as Item[]).map((item) => (
									<button
										className="entity-card"
										key={item.id}
										onClick={() => setSelected({ kind: 'item', value: item })}>
										<div className="preview">
											{item.media?.[0] ? (
												<>
													<div className="preview-image">
														<img src={item.media[0].url} alt="" />
													</div>
													<span
														className="image-expand"
														role="button"
														tabIndex={0}
														aria-label="Открыть фото на весь экран"
														onClick={(event) => {
															event.stopPropagation()
															setPhotoViewer({ src: item.media![0].url, alt: item.name })
														}}>
														⤢
													</span>
												</>
											) : (
												<span className="card-placeholder">▧</span>
											)}
											<div className="card-icons">
												<span
													className="status-icon"
													title={stateMeta(item.state).label}
													aria-label={stateMeta(item.state).label}>
													{stateMeta(item.state).icon}
												</span>
												{item.tags?.map((tag) => (
													<span
														key={tag.id}
														className="flag-icon"
														title={tag.name}
														aria-label={tag.name}>
														⚑
													</span>
												))}
											</div>
											<span className="preview-photo-count">
												{boxList.find((box) => box.id === item.boxId)?.name || 'Без коробки'}
											</span>
										</div>
										<div className="card">
											<h2>{item.name}</h2>
											<p>{item.description || 'Без описания'}</p>
										</div>
									</button>
								))
							: view === 'boxes'
								? (boxList as Box[]).map((box) => (
										<button className="entity-card" key={box.id} onClick={() => setBoxDialog(box)}>
											<div className="preview box-preview">
												{itemList
													.filter((item) => item.boxId === box.id)
													.slice(0, 6)
													.map((item) =>
														item.media?.[0] ? (
															<img key={item.id} src={item.media[0].url} alt="" />
														) : (
															<span key={item.id} aria-label={item.name}>
																▧
															</span>
														)
													)}
												{!itemList.some((item) => item.boxId === box.id) && <span>□</span>}
											</div>
											<div className="card">
												<h2>{box.name}</h2>
												<p>
													{box.itemCount} вещей · {box.description || 'Без описания'}
												</p>
											</div>
										</button>
									))
								: (locationList as Location[]).map((location) => (
										<button
											className="entity-card"
											key={location.id}
											onClick={() => setSelected({ kind: 'location', value: location })}>
											<div className="preview">⌖</div>
											<div className="card">
												<h2>{location.name}</h2>
												<p>{location.description || location.code || 'Без описания'}</p>
											</div>
										</button>
									))}
					</div>
				)}
			</section>
			<button
				className="fab"
				aria-label="Быстрое создание"
				onClick={() => setAdding({ kind: 'item' })}>
				＋
			</button>
			{adding && (
				<AddDialog
					space={activeSpace}
					initialKind={adding.kind}
					initialBoxId={adding.boxId}
					onClose={() => setAdding(null)}
				/>
			)}
			{boxDialog && !fullBox && (
				<EntityDialog
					entity={{ kind: 'box', value: boxDialog }}
					boxes={boxList}
					locations={locationList}
					items={itemList}
					spaceId={activeSpace.id}
					disableEscape={Boolean(selected || adding)}
					onOpenItem={(item) => setSelected({ kind: 'item', value: item })}
					onOpenFull={() => openBoxFull(boxDialog)}
					onAddItem={() => setAdding({ kind: 'item', boxId: boxDialog.id })}
					onClose={() => setBoxDialog(null)}
				/>
			)}
			{selected && !fullBox && (
				<EntityDialog
					entity={selected}
					boxes={boxList}
					locations={locationList}
					items={itemList}
					spaceId={activeSpace.id}
					onClose={() => setSelected(null)}
				/>
			)}
			{fullBox && (
				<EntityDialog
					entity={{ kind: 'box', value: fullBox }}
					boxes={boxList}
					locations={locationList}
					items={itemList}
					spaceId={activeSpace.id}
					fullPage
					disableEscape={Boolean(selected || adding)}
					onOpenItem={(item) => setSelected({ kind: 'item', value: item })}
					onAddItem={() => setAdding({ kind: 'item', boxId: fullBox.id })}
					onClose={closeBoxFull}
				/>
			)}
			{selected?.kind === 'item' && fullBox && (
				<EntityDialog
					entity={selected}
					boxes={boxList}
					locations={locationList}
					items={itemList}
					spaceId={activeSpace.id}
					onClose={() => setSelected(null)}
				/>
			)}
			{photoViewer && <PhotoViewer {...photoViewer} onClose={() => setPhotoViewer(null)} />}
			{creatingSpace && <CreateSpaceDialog onClose={() => setCreatingSpace(false)} />}
		</main>
	)
}
export function App() {
	const session = useMeQuery()
	if (session.isLoading)
		return (
			<main className="auth-page">
				<p role="status">Проверяем сессию…</p>
			</main>
		)
	return session.data ? <Inventory user={session.data} /> : <AuthPage />
}
